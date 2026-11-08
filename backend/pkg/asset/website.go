package asset

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

// WebsiteOrigin is the shared identity for HTTP(S) websites. URL paths remain
// in the catalog as evidence; the ledger groups them by this origin.
func WebsiteOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Opaque != "" {
		return "", fmt.Errorf("invalid website URL")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid HTTP(S) website URL")
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else if !IsDomainHost(host) {
		return "", fmt.Errorf("invalid website host")
	}
	port := 80
	if scheme == "https" {
		port = 443
	}
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid website port")
		}
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if (scheme == "http" && port != 80) || (scheme == "https" && port != 443) {
		authority = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return scheme + "://" + authority, nil
}

// ArchiveWebsite updates all existing URL members atomically, within the same
// scope as the selected asset. Ordinary asset edits retain their old semantics.
func (s Store) ArchiveWebsite(ctx context.Context, id string, archived bool) (database.UnifiedAsset, error) {
	var selected database.UnifiedAsset
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&selected, "id = ?", id).Error; err != nil {
			return err
		}
		if selected.Type != TypeURL {
			return fmt.Errorf("website archive requires a URL asset")
		}
		origin, err := WebsiteOrigin(selected.CanonicalValue)
		if err != nil {
			return err
		}
		var rows []database.UnifiedAsset
		if err := tx.Where("scope_key = ? AND type = ?", selected.ScopeKey, TypeURL).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			candidate, err := WebsiteOrigin(row.CanonicalValue)
			if err != nil || candidate != origin {
				continue
			}
			updated, err := (Store{DB: tx}).UpdateAsset(ctx, row.ID, AssetUpdate{Archived: &archived})
			if err != nil {
				return err
			}
			if row.ID == id {
				selected = updated
			}
		}
		return nil
	})
	return selected, err
}
