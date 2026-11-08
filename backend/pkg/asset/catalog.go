package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"golang.org/x/net/idna"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TypeDomain      = "domain"
	TypeIP          = "ip"
	TypeURL         = "url"
	TypeService     = "service"
	TypeCertificate = "certificate"

	DefaultScope = "global"
)

var validTypes = map[string]bool{
	TypeDomain:      true,
	TypeIP:          true,
	TypeURL:         true,
	TypeService:     true,
	TypeCertificate: true,
}

// Observation is the input contract shared by scanners, imports and future
// connectors. SourceID and SourceVersion are optional but should be supplied
// when the source has a task/version identity.
type Observation struct {
	ScopeKey      string
	Type          string
	Value         string
	DisplayValue  string
	Source        string
	SourceID      string
	SourceVersion int
	TaskID        string
	TaskVersion   int
	ImportBatchID string
	Origin        string
	GroupKey      string
	Confidence    string
	ObservedAt    time.Time
	ExpiresAt     *time.Time
	EvidenceRef   string
	Attributes    map[string]any
}

type Normalized struct {
	ScopeKey       string
	Type           string
	CanonicalValue string
	DisplayValue   string
}

type Store struct{ DB *gorm.DB }

type RelationInput struct {
	ScopeKey      string
	FromAssetID   string
	ToAssetID     string
	RelationType  string
	Source        string
	SourceID      string
	SourceVersion int
	ObservedAt    time.Time
	Metadata      map[string]any
}

func Normalize(scopeKey, assetType, rawValue string) (Normalized, error) {
	scopeKey = strings.TrimSpace(scopeKey)
	if scopeKey == "" {
		scopeKey = DefaultScope
	}
	if len([]rune(scopeKey)) > 255 {
		return Normalized{}, errors.New("scopeKey must be at most 255 characters")
	}

	assetType = strings.ToLower(strings.TrimSpace(assetType))
	value := strings.TrimSpace(rawValue)
	if assetType == "host" || assetType == "hostname" {
		assetType = TypeDomain
	}
	if assetType == "" {
		assetType = inferType(value)
	}
	if !validTypes[assetType] {
		return Normalized{}, fmt.Errorf("unsupported asset type %q", assetType)
	}
	if value == "" {
		return Normalized{}, errors.New("asset value is required")
	}

	canonical, err := normalizeValue(assetType, value)
	if err != nil {
		return Normalized{}, err
	}
	return Normalized{ScopeKey: scopeKey, Type: assetType, CanonicalValue: canonical, DisplayValue: value}, nil
}

// ParseIPLike normalizes IPv4/IPv6 values that scanners commonly emit with
// brackets, a zone suffix, a port, or a trailing colon from truncated parsing.
func ParseIPLike(value string) (string, bool) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", false
	}
	candidates := make([]string, 0, 3)
	if host, _, err := net.SplitHostPort(raw); err == nil {
		candidates = append(candidates, host)
	}
	candidates = append(candidates, raw, strings.Trim(raw, "[]"))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if zone := strings.LastIndex(candidate, "%"); zone > 0 {
			candidate = candidate[:zone]
		}
		candidate = strings.Trim(candidate, "[]")
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String(), true
		}
		if strings.HasSuffix(candidate, ":") {
			if ip := net.ParseIP(strings.TrimSuffix(candidate, ":")); ip != nil {
				return ip.String(), true
			}
		}
	}
	return "", false
}

// LooksLikeIPv6 covers truncated or malformed scanner values that are still
// clearly IPv6-shaped. They cannot be canonicalized, but must not be treated as
// domains in asset listings and topology.
func LooksLikeIPv6(value string) bool {
	raw := strings.TrimSpace(value)
	raw = strings.Trim(raw, "[]")
	if zone := strings.LastIndex(raw, "%"); zone > 0 {
		raw = raw[:zone]
	}
	raw = strings.TrimSuffix(raw, ":")
	if strings.Count(raw, ":") < 3 {
		return false
	}
	groups := strings.Split(raw, ":")
	nonEmpty := 0
	for _, group := range groups {
		if group == "" {
			continue
		}
		nonEmpty++
		if len(group) > 4 {
			return false
		}
		for _, char := range strings.ToLower(group) {
			if !strings.ContainsRune("0123456789abcdef", char) {
				return false
			}
		}
	}
	if nonEmpty == 6 && !strings.Contains(raw, "::") {
		allShort := true
		for _, group := range groups {
			if len(group) != 2 {
				allShort = false
				break
			}
		}
		if allShort {
			return false // MAC address, not IPv6
		}
	}
	return nonEmpty > 0
}

// ClassifyHost is the single host classification used by asset ingestion and
// projections. It is intentionally the only place that decides whether a
// scanner value is an IP or a domain.
func ClassifyHost(value string) string {
	if _, ok := ParseIPLike(value); ok {
		return TypeIP
	}
	if LooksLikeIPv6(value) {
		return TypeIP
	}
	return TypeDomain
}

var (
	domainHostIDNA  = idna.New(idna.MapForLookup(), idna.StrictDomainName(false), idna.CheckHyphens(false))
	domainHostLabel = regexp.MustCompile(`^[a-zA-Z0-9_](?:[a-zA-Z0-9_-]{0,61}[a-zA-Z0-9_])?$`)
)

// IsDomainHost validates a bare host after any wildcard and trailing dot
// have been removed. Single-label internal names and DNS record names are valid.
func IsDomainHost(host string) bool {
	ascii, err := domainHostIDNA.ToASCII(host)
	if err != nil || len(ascii) > 253 || net.ParseIP(ascii) != nil {
		return false
	}
	for label := range strings.SplitSeq(ascii, ".") {
		if !domainHostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func inferType(value string) string {
	trimmed := strings.TrimSpace(value)
	if _, ok := ParseIPLike(trimmed); ok {
		return TypeIP
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Hostname() != "" && parsed.Scheme != "" {
		return TypeURL
	}
	return TypeDomain
}

func normalizeValue(assetType, value string) (string, error) {
	switch assetType {
	case TypeIP:
		ip, ok := ParseIPLike(value)
		if !ok {
			return "", fmt.Errorf("invalid IP asset %q", value)
		}
		return ip, nil
	case TypeDomain:
		host := value
		if strings.Contains(value, "://") {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Hostname() == "" {
				return "", fmt.Errorf("invalid domain asset %q", value)
			}
			host = parsed.Hostname()
		} else if parsed, err := url.Parse("//" + value); err == nil && parsed.User == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
		}
		host = strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "*."), ".")
		if !IsDomainHost(host) {
			return "", fmt.Errorf("invalid domain asset %q", value)
		}
		return host, nil
	case TypeURL:
		parsed, err := url.Parse(value)
		if err != nil || parsed.Hostname() == "" || parsed.Scheme == "" {
			return "", fmt.Errorf("invalid URL asset %q", value)
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		if (parsed.Scheme == "http" && parsed.Port() == "80") || (parsed.Scheme == "https" && parsed.Port() == "443") {
			parsed.Host = parsed.Hostname()
			if strings.Contains(parsed.Host, ":") {
				parsed.Host = "[" + parsed.Host + "]"
			}
		}
		parsed.Fragment = ""
		if parsed.Path == "/" {
			parsed.Path = ""
		}
		return parsed.String(), nil
	case TypeService:
		service := strings.Join(strings.Fields(strings.ToLower(value)), " ")
		if service == "" {
			return "", errors.New("service value is required")
		}
		return service, nil
	case TypeCertificate:
		fingerprint := strings.ToLower(strings.TrimSpace(value))
		fingerprint = strings.TrimPrefix(fingerprint, "sha256:")
		fingerprint = strings.ReplaceAll(fingerprint, ":", "")
		fingerprint = strings.ReplaceAll(fingerprint, " ", "")
		if len(fingerprint) != 64 {
			return "", errors.New("certificate value must be a SHA-256 fingerprint")
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			return "", errors.New("certificate value must be a SHA-256 fingerprint")
		}
		return fingerprint, nil
	default:
		return "", fmt.Errorf("unsupported asset type %q", assetType)
	}
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:])[:48]
}

func AssetID(scopeKey, assetType, canonicalValue string) string {
	return stableID("asset", strings.Join([]string{scopeKey, assetType, canonicalValue}, "\x00"))
}

func observationKey(input Observation, normalized Normalized) string {
	seed := strings.Join([]string{normalized.ScopeKey, normalized.Type, normalized.CanonicalValue, strings.TrimSpace(input.Source), strings.TrimSpace(input.SourceID), fmt.Sprint(input.SourceVersion), strings.TrimSpace(input.EvidenceRef)}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func confidenceRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "confirmed", "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func normalizeConfidence(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if confidenceRank(value) == 0 {
		return "medium"
	}
	return value
}

func normalizeOrigin(origin, source string) string {
	origin = strings.ToLower(strings.TrimSpace(origin))
	switch origin {
	case "manual", "import", "scan", "external":
		return origin
	case "":
	default:
		return origin
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "manual":
		return "manual"
	case "import", "csv", "excel":
		return "import"
	default:
		return "scan"
	}
}

func verificationForOrigin(origin string) string {
	switch strings.ToLower(strings.TrimSpace(origin)) {
	case "manual", "import":
		return "declared"
	default:
		return "observed"
	}
}

func verificationRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "verified":
		return 3
	case "observed":
		return 2
	case "declared":
		return 1
	default:
		return 0
	}
}

func upgradeVerification(current, incoming string) string {
	if verificationRank(incoming) > verificationRank(current) {
		return strings.ToLower(strings.TrimSpace(incoming))
	}
	return strings.ToLower(strings.TrimSpace(current))
}

func (s Store) Upsert(ctx context.Context, input Observation) (database.UnifiedAsset, database.UnifiedAssetObservation, error) {
	if s.DB == nil {
		return database.UnifiedAsset{}, database.UnifiedAssetObservation{}, errors.New("asset store database is nil")
	}
	normalized, err := Normalize(input.ScopeKey, input.Type, input.Value)
	if err != nil {
		return database.UnifiedAsset{}, database.UnifiedAssetObservation{}, err
	}
	if display := strings.TrimSpace(input.DisplayValue); display != "" {
		normalized.DisplayValue = display
	}
	if strings.TrimSpace(input.Source) == "" {
		return database.UnifiedAsset{}, database.UnifiedAssetObservation{}, errors.New("observation source is required")
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	input.Confidence = normalizeConfidence(input.Confidence)
	input.Origin = normalizeOrigin(input.Origin, input.Source)
	input.GroupKey = strings.TrimSpace(input.GroupKey)
	taskID, taskVersion := taskFromAttributes(input.TaskID, input.TaskVersion, input.Attributes)
	assetID := AssetID(normalized.ScopeKey, normalized.Type, normalized.CanonicalValue)
	obsKey := observationKey(input, normalized)
	obsID := stableID("obs", obsKey)
	var assetRow database.UnifiedAsset
	var observationRow database.UnifiedAssetObservation
	assetExisted := false
	var before map[string]any
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", assetID).First(&assetRow).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			candidate := database.UnifiedAsset{
				ID:             assetID,
				ScopeKey:       normalized.ScopeKey,
				Type:           normalized.Type,
				CanonicalValue: normalized.CanonicalValue,
				DisplayValue:   normalized.DisplayValue,
				GroupKey:       input.GroupKey,
				Verification:   verificationForOrigin(input.Origin),
				Status:         "active",
				Confidence:     input.Confidence,
				Tags:           []string{},
				FirstSeenAt:    input.ObservedAt,
				LastSeenAt:     input.ObservedAt,
				Metadata:       map[string]any{},
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", assetID).First(&assetRow).Error; err != nil {
				return err
			}
		} else {
			assetExisted = true
			before = assetState(assetRow)
		}
		updates := map[string]any{"display_value": normalized.DisplayValue, "updated_at": time.Now().UTC()}
		if assetRow.ArchivedAt == nil {
			updates["status"] = "active"
			assetRow.Status = "active"
		}
		if next := upgradeVerification(assetRow.Verification, verificationForOrigin(input.Origin)); next != assetRow.Verification {
			updates["verification"] = next
			assetRow.Verification = next
		}
		if assetRow.GroupKey == "" && input.GroupKey != "" {
			updates["group_key"] = input.GroupKey
			assetRow.GroupKey = input.GroupKey
		}
		if input.ObservedAt.Before(assetRow.FirstSeenAt) {
			updates["first_seen_at"] = input.ObservedAt
			assetRow.FirstSeenAt = input.ObservedAt
		}
		if input.ObservedAt.After(assetRow.LastSeenAt) {
			updates["last_seen_at"] = input.ObservedAt
			assetRow.LastSeenAt = input.ObservedAt
		}
		if confidenceRank(input.Confidence) > confidenceRank(assetRow.Confidence) {
			updates["confidence"] = input.Confidence
			assetRow.Confidence = input.Confidence
		}
		assetRow.DisplayValue = normalized.DisplayValue
		if err := tx.Model(&database.UnifiedAsset{}).Where("id = ?", assetID).Updates(updates).Error; err != nil {
			return err
		}

		observationRow = database.UnifiedAssetObservation{
			ID: obsID, AssetID: assetID, ScopeKey: normalized.ScopeKey, Type: normalized.Type,
			CanonicalValue: normalized.CanonicalValue, Source: strings.TrimSpace(input.Source),
			SourceID: strings.TrimSpace(input.SourceID), SourceVersion: input.SourceVersion,
			TaskID: taskID, TaskVersion: taskVersion, ImportBatchID: strings.TrimSpace(input.ImportBatchID),
			Origin: input.Origin, ObservationKey: obsKey, Confidence: input.Confidence,
			ObservedAt: input.ObservedAt, ExpiresAt: input.ExpiresAt,
			EvidenceRef: strings.TrimSpace(input.EvidenceRef), Attributes: input.Attributes,
		}
		if err := tx.Where("observation_key = ?", obsKey).First(&observationRow).Error; err == nil {
			// Older catalog rows predate the promoted provenance columns. Fill
			// them in on an idempotent replay instead of leaving the source
			// task invisible to the ledger.
			updates := map[string]any{}
			if observationRow.TaskID == "" && taskID != "" {
				updates["task_id"] = taskID
			}
			if observationRow.TaskVersion == 0 && taskVersion > 0 {
				updates["task_version"] = taskVersion
			}
			if observationRow.Origin == "" && input.Origin != "" {
				updates["origin"] = input.Origin
			}
			if observationRow.ImportBatchID == "" && input.ImportBatchID != "" {
				updates["import_batch_id"] = input.ImportBatchID
			}
			if len(updates) > 0 {
				if err := tx.Model(&database.UnifiedAssetObservation{}).Where("id = ?", observationRow.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&observationRow).Error; err != nil {
			return err
		}
		changeType := "observed"
		if !assetExisted {
			changeType = "added"
		} else if before["status"] == "stale" || before["status"] == "retired" {
			changeType = "reactivated"
		} else if before["displayValue"] != assetRow.DisplayValue || before["confidence"] != assetRow.Confidence || before["verification"] != assetRow.Verification {
			changeType = "updated"
		}
		if err := recordAssetChange(tx, assetRow, changeType, observationRow.Source, observationRow.SourceID, observationRow.SourceVersion, observationRow.ObservedAt, before, assetState(assetRow)); err != nil {
			return err
		}
		return tx.Where("observation_key = ?", obsKey).First(&observationRow).Error
	})
	return assetRow, observationRow, err
}

func taskFromAttributes(taskID string, taskVersion int, attrs map[string]any) (string, int) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" && attrs != nil {
		if value, ok := attrs["taskId"]; ok {
			taskID = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	if taskVersion <= 0 && attrs != nil {
		if value, ok := attrs["version"]; ok {
			switch typed := value.(type) {
			case int:
				taskVersion = typed
			case int64:
				taskVersion = int(typed)
			case float64:
				taskVersion = int(typed)
			default:
				if parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value))); err == nil {
					taskVersion = parsed
				}
			}
		}
	}
	return taskID, taskVersion
}

func assetState(assetRow database.UnifiedAsset) map[string]any {
	return map[string]any{
		"status":       assetRow.Status,
		"confidence":   assetRow.Confidence,
		"displayValue": assetRow.DisplayValue,
		"name":         assetRow.Name,
		"notes":        assetRow.Notes,
		"tags":         assetRow.Tags,
		"verification": assetRow.Verification,
		"groupKey":     assetRow.GroupKey,
		"archivedAt":   assetRow.ArchivedAt,
		"firstSeenAt":  assetRow.FirstSeenAt,
		"lastSeenAt":   assetRow.LastSeenAt,
		"updatedAt":    assetRow.UpdatedAt,
	}
}

func buildAssetChange(assetRow database.UnifiedAsset, changeType, source, sourceID string, sourceVersion int, detectedAt time.Time, before, after map[string]any) database.UnifiedAssetChange {
	if detectedAt.IsZero() {
		detectedAt = time.Now().UTC()
	}
	seed := strings.Join([]string{assetRow.ID, changeType, source, sourceID, fmt.Sprint(sourceVersion), detectedAt.UTC().Format(time.RFC3339Nano)}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	key := hex.EncodeToString(sum[:])
	return database.UnifiedAssetChange{ID: stableID("change", key), ChangeKey: key, ScopeKey: assetRow.ScopeKey, AssetID: assetRow.ID, AssetType: assetRow.Type, CanonicalValue: assetRow.CanonicalValue, ChangeType: changeType, Source: source, SourceID: sourceID, SourceVersion: sourceVersion, BeforeJSON: before, AfterJSON: after, DetectedAt: detectedAt}
}

func recordAssetChange(tx *gorm.DB, assetRow database.UnifiedAsset, changeType, source, sourceID string, sourceVersion int, detectedAt time.Time, before, after map[string]any) error {
	change := buildAssetChange(assetRow, changeType, source, sourceID, sourceVersion, detectedAt, before, after)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&change).Error
}

func (s Store) UpsertRelation(ctx context.Context, input RelationInput) (database.UnifiedAssetRelation, error) {
	if s.DB == nil {
		return database.UnifiedAssetRelation{}, errors.New("asset store database is nil")
	}
	if strings.TrimSpace(input.FromAssetID) == "" || strings.TrimSpace(input.ToAssetID) == "" || strings.TrimSpace(input.RelationType) == "" || strings.TrimSpace(input.Source) == "" {
		return database.UnifiedAssetRelation{}, errors.New("relation requires from, to, relationType and source")
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	seed := strings.Join([]string{input.ScopeKey, input.FromAssetID, input.ToAssetID, input.RelationType}, "\x00")
	relation := database.UnifiedAssetRelation{ID: stableID("relation", seed), ScopeKey: input.ScopeKey, FromAssetID: input.FromAssetID, ToAssetID: input.ToAssetID, RelationType: strings.TrimSpace(input.RelationType), Source: strings.TrimSpace(input.Source), SourceID: strings.TrimSpace(input.SourceID), SourceVersion: input.SourceVersion, Active: true, FirstSeenAt: input.ObservedAt, LastSeenAt: input.ObservedAt, Metadata: input.Metadata}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.UnifiedAssetRelation
		if err := tx.Where("id = ?", relation.ID).First(&existing).Error; err == nil {
			relation = existing
			updates := map[string]any{"active": true, "last_seen_at": input.ObservedAt, "source": strings.TrimSpace(input.Source), "source_id": strings.TrimSpace(input.SourceID), "source_version": input.SourceVersion, "updated_at": time.Now().UTC()}
			if input.ObservedAt.Before(existing.FirstSeenAt) {
				updates["first_seen_at"] = input.ObservedAt
				relation.FirstSeenAt = input.ObservedAt
			}
			if input.ObservedAt.After(existing.LastSeenAt) {
				relation.LastSeenAt = input.ObservedAt
			}
			relation.Active = true
			return tx.Model(&database.UnifiedAssetRelation{}).Where("id = ?", relation.ID).Updates(updates).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&relation).Error
	})
	return relation, err
}

func (s Store) ListRelations(ctx context.Context, assetID string, activeOnly bool) ([]database.UnifiedAssetRelation, error) {
	if s.DB == nil {
		return nil, errors.New("asset store database is nil")
	}
	query := s.DB.WithContext(ctx).Where("from_asset_id = ? OR to_asset_id = ?", assetID, assetID)
	if activeOnly {
		query = query.Where("active = ?", true)
	}
	var relations []database.UnifiedAssetRelation
	if err := query.Order("last_seen_at DESC").Find(&relations).Error; err != nil {
		return nil, err
	}
	return relations, nil
}

func (s Store) ReconcileLifecycle(ctx context.Context, now time.Time, staleAfter, retireAfter time.Duration) (int, error) {
	if s.DB == nil {
		return 0, errors.New("asset store database is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if staleAfter <= 0 {
		staleAfter = 7 * 24 * time.Hour
	}
	if retireAfter <= staleAfter {
		retireAfter = 30 * 24 * time.Hour
	}
	threshold := now.Add(-staleAfter)
	const batchSize = 500
	changed := 0
	lastID := ""
	for {
		var assets []database.UnifiedAsset
		query := s.DB.WithContext(ctx).
			Where("(verification IS NULL OR verification <> ?)", "declared").
			Where("archived_at IS NULL").
			Where("last_seen_at < ?", threshold)
		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Order("id ASC").Limit(batchSize).Find(&assets).Error; err != nil {
			return changed, err
		}
		if len(assets) == 0 {
			break
		}
		lastID = assets[len(assets)-1].ID
		batchChanged := 0
		err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			changes := make([]database.UnifiedAssetChange, 0, len(assets))
			for _, assetRow := range assets {
				age := now.Sub(assetRow.LastSeenAt)
				next := "active"
				if age >= retireAfter {
					next = "retired"
				} else if age >= staleAfter {
					next = "stale"
				}
				if next == assetRow.Status {
					continue
				}
				before := assetState(assetRow)
				assetRow.Status = next
				assetRow.UpdatedAt = now
				if err := tx.Model(&database.UnifiedAsset{}).Where("id = ?", assetRow.ID).Updates(map[string]any{"status": next, "updated_at": now}).Error; err != nil {
					return err
				}
				changes = append(changes, buildAssetChange(assetRow, next, "lifecycle", "", 0, now, before, assetState(assetRow)))
				batchChanged++
			}
			if len(changes) > 0 {
				// CreateInBatches opens its own transaction when the slice is
				// larger than the batch size; inside this transaction that
				// nested path loses the destination model. Chunk explicitly.
				const changeBatchSize = 100
				for start := 0; start < len(changes); start += changeBatchSize {
					end := start + changeBatchSize
					if end > len(changes) {
						end = len(changes)
					}
					chunk := changes[start:end]
					if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return changed, err
		}
		changed += batchChanged
	}
	if err := s.DB.WithContext(ctx).Model(&database.UnifiedAssetRelation{}).
		Where("active = ? AND last_seen_at < ?", true, now.Add(-retireAfter)).
		Updates(map[string]any{"active": false, "updated_at": now}).Error; err != nil {
		return changed, err
	}
	return changed, nil
}

func (s Store) UpsertMany(ctx context.Context, inputs []Observation) ([]database.UnifiedAsset, []database.UnifiedAssetObservation, error) {
	if s.DB == nil {
		return nil, nil, errors.New("asset store database is nil")
	}
	if len(inputs) == 0 {
		return nil, nil, errors.New("at least one observation is required")
	}
	assets := make([]database.UnifiedAsset, 0, len(inputs))
	observations := make([]database.UnifiedAssetObservation, 0, len(inputs))
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inner := Store{DB: tx}
		for _, input := range inputs {
			assetRow, observationRow, err := inner.Upsert(ctx, input)
			if err != nil {
				return err
			}
			assets = append(assets, assetRow)
			observations = append(observations, observationRow)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return assets, observations, nil
}

type ListFilter struct {
	ScopeKey string
	Type     string
	Keyword  string
	Status   string
	Limit    int
	Offset   int
}

func (s Store) List(ctx context.Context, filter ListFilter) ([]database.UnifiedAsset, int64, error) {
	if s.DB == nil {
		return nil, 0, errors.New("asset store database is nil")
	}
	query := s.DB.WithContext(ctx).Model(&database.UnifiedAsset{})
	if strings.TrimSpace(filter.ScopeKey) != "" {
		query = query.Where("scope_key = ?", strings.TrimSpace(filter.ScopeKey))
	}
	if strings.TrimSpace(filter.Type) != "" {
		query = query.Where("type = ?", strings.ToLower(strings.TrimSpace(filter.Type)))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query = query.Where("status = ?", strings.ToLower(strings.TrimSpace(filter.Status)))
	}
	if strings.TrimSpace(filter.Keyword) != "" {
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		query = query.Where("canonical_value LIKE ? OR display_value LIKE ?", keyword, keyword)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	var assets []database.UnifiedAsset
	if err := query.Order("last_seen_at DESC, canonical_value ASC").Limit(limit).Offset(filter.Offset).Find(&assets).Error; err != nil {
		return nil, 0, err
	}
	return assets, total, nil
}

func (s Store) Get(ctx context.Context, id string) (database.UnifiedAsset, []database.UnifiedAssetObservation, error) {
	if s.DB == nil {
		return database.UnifiedAsset{}, nil, errors.New("asset store database is nil")
	}
	var assetRow database.UnifiedAsset
	if err := s.DB.WithContext(ctx).Where("id = ?", strings.TrimSpace(id)).First(&assetRow).Error; err != nil {
		return assetRow, nil, err
	}
	var observations []database.UnifiedAssetObservation
	if err := s.DB.WithContext(ctx).Where("asset_id = ?", assetRow.ID).Order("observed_at DESC, created_at DESC").Find(&observations).Error; err != nil {
		return assetRow, nil, err
	}
	return assetRow, observations, nil
}
