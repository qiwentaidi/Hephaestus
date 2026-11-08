package asset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type ReclassifySummary struct {
	Reclassified int
	Archived     int
	Failed       int
}

// ReclassifyMisclassifiedIPAssets repairs historical catalog rows that were
// written as domains even though their value is an IP. Valid IP values are
// moved (and merged when necessary) into IP assets; truncated IPv6-shaped
// values are archived because they cannot represent a usable asset.
func ReclassifyMisclassifiedIPAssets(ctx context.Context, db *gorm.DB) (ReclassifySummary, error) {
	summary := ReclassifySummary{}
	if db == nil || !db.Migrator().HasTable(&database.UnifiedAsset{}) {
		return summary, nil
	}
	var assets []database.UnifiedAsset
	if err := db.WithContext(ctx).Where("type = ?", TypeDomain).Order("created_at ASC").Find(&assets).Error; err != nil {
		return summary, err
	}
	for _, old := range assets {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		default:
		}
		value := strings.TrimSpace(old.CanonicalValue)
		if value == "" {
			value = strings.TrimSpace(old.DisplayValue)
		}
		ip, valid := ParseIPLike(value)
		if !valid && !LooksLikeIPv6(value) {
			continue
		}
		if !valid {
			if err := archiveMisclassifiedAsset(db.WithContext(ctx), old); err != nil {
				summary.Failed++
				continue
			}
			summary.Archived++
			continue
		}
		if err := mergeIntoIPAsset(db.WithContext(ctx), old, ip); err != nil {
			summary.Failed++
			continue
		}
		summary.Reclassified++
	}
	return summary, nil
}

func archiveMisclassifiedAsset(db *gorm.DB, old database.UnifiedAsset) error {
	now := time.Now().UTC()
	return db.Model(&database.UnifiedAsset{}).Where("id = ?", old.ID).Updates(map[string]any{
		"status":      "retired",
		"archived_at": now,
		"updated_at":  now,
	}).Error
}

func mergeIntoIPAsset(db *gorm.DB, old database.UnifiedAsset, ip string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		targetID := AssetID(old.ScopeKey, TypeIP, ip)
		var target database.UnifiedAsset
		err := tx.Where("id = ?", targetID).First(&target).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			target = old
			target.ID = targetID
			target.Type = TypeIP
			target.CanonicalValue = ip
			target.DisplayValue = ip
			target.ArchivedAt = nil
			target.UpdatedAt = time.Now().UTC()
			if err := tx.Create(&target).Error; err != nil {
				return fmt.Errorf("create IP asset %s: %w", targetID, err)
			}
		case err != nil:
			return err
		default:
			if target.ArchivedAt != nil {
				if err := tx.Model(&database.UnifiedAsset{}).Where("id = ?", target.ID).Updates(map[string]any{
					"archived_at": nil,
					"status":      "active",
					"updated_at":  time.Now().UTC(),
				}).Error; err != nil {
					return err
				}
			}
			if !old.FirstSeenAt.IsZero() && (target.FirstSeenAt.IsZero() || old.FirstSeenAt.Before(target.FirstSeenAt)) {
				target.FirstSeenAt = old.FirstSeenAt
			}
			if old.LastSeenAt.After(target.LastSeenAt) {
				target.LastSeenAt = old.LastSeenAt
			}
			if err := tx.Model(&database.UnifiedAsset{}).Where("id = ?", target.ID).Updates(map[string]any{
				"first_seen_at": target.FirstSeenAt,
				"last_seen_at":  target.LastSeenAt,
				"updated_at":    time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
		}

		if err := moveAssetReferences(tx, old.ID, targetID, ip); err != nil {
			return err
		}
		return tx.Where("id = ?", old.ID).Delete(&database.UnifiedAsset{}).Error
	})
}

func moveAssetReferences(tx *gorm.DB, oldID, targetID, ip string) error {
	if err := tx.Model(&database.UnifiedAssetObservation{}).
		Where("asset_id = ?", oldID).
		Updates(map[string]any{"asset_id": targetID, "type": TypeIP, "canonical_value": ip}).Error; err != nil {
		return fmt.Errorf("move observations: %w", err)
	}
	if tx.Migrator().HasTable(&database.UnifiedAssetChange{}) {
		if err := tx.Model(&database.UnifiedAssetChange{}).Where("asset_id = ?", oldID).Update("asset_id", targetID).Error; err != nil {
			return fmt.Errorf("move changes: %w", err)
		}
	}
	if err := moveAssetRelations(tx, oldID, targetID); err != nil {
		return err
	}
	if tx.Migrator().HasTable(&database.FindingAssetLink{}) {
		if err := moveFindingAssetLinks(tx, oldID, targetID); err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&database.AssetRisk{}) {
		if err := mergeAssetRisk(tx, oldID, targetID); err != nil {
			return err
		}
	}
	return nil
}

func moveAssetRelations(tx *gorm.DB, oldID, targetID string) error {
	var relations []database.UnifiedAssetRelation
	if err := tx.Where("from_asset_id = ? OR to_asset_id = ?", oldID, oldID).Find(&relations).Error; err != nil {
		return fmt.Errorf("load relations: %w", err)
	}
	for _, relation := range relations {
		fromID, toID := relation.FromAssetID, relation.ToAssetID
		if fromID == oldID {
			fromID = targetID
		}
		if toID == oldID {
			toID = targetID
		}
		var existing database.UnifiedAssetRelation
		err := tx.Where("scope_key = ? AND from_asset_id = ? AND to_asset_id = ? AND relation_type = ? AND id <> ?",
			relation.ScopeKey, fromID, toID, relation.RelationType, relation.ID).First(&existing).Error
		if err == nil {
			updates := map[string]any{"updated_at": time.Now().UTC()}
			if relation.LastSeenAt.After(existing.LastSeenAt) {
				updates["last_seen_at"] = relation.LastSeenAt
			}
			if relation.Active {
				updates["active"] = true
			}
			if err := tx.Model(&database.UnifiedAssetRelation{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", relation.ID).Delete(&database.UnifiedAssetRelation{}).Error; err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&database.UnifiedAssetRelation{}).Where("id = ?", relation.ID).Updates(map[string]any{
			"from_asset_id": fromID,
			"to_asset_id":   toID,
			"updated_at":    time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func moveFindingAssetLinks(tx *gorm.DB, oldID, targetID string) error {
	var links []database.FindingAssetLink
	if err := tx.Where("asset_id = ?", oldID).Find(&links).Error; err != nil {
		return fmt.Errorf("load finding links: %w", err)
	}
	for _, link := range links {
		var existing database.FindingAssetLink
		err := tx.Where("finding_id = ? AND asset_id = ? AND role = ? AND id <> ?",
			link.FindingID, targetID, link.Role, link.ID).First(&existing).Error
		if err == nil {
			if err := tx.Where("id = ?", link.ID).Delete(&database.FindingAssetLink{}).Error; err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&database.FindingAssetLink{}).Where("id = ?", link.ID).Update("asset_id", targetID).Error; err != nil {
			return err
		}
	}
	return tx.Model(&database.SecurityFinding{}).Where("primary_asset_id = ?", oldID).Update("primary_asset_id", targetID).Error
}

func mergeAssetRisk(tx *gorm.DB, oldID, targetID string) error {
	var oldRisk database.AssetRisk
	if err := tx.Where("asset_id = ?", oldID).First(&oldRisk).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var target database.AssetRisk
	err := tx.Where("asset_id = ?", targetID).First(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		oldRisk.AssetID = targetID
		oldRisk.UpdatedAt = time.Now().UTC()
		return tx.Create(&oldRisk).Error
	}
	if err != nil {
		return err
	}
	target.DirectCritical += oldRisk.DirectCritical
	target.DirectHigh += oldRisk.DirectHigh
	target.DirectMedium += oldRisk.DirectMedium
	target.InheritedCritical += oldRisk.InheritedCritical
	target.InheritedHigh += oldRisk.InheritedHigh
	target.InheritedMedium += oldRisk.InheritedMedium
	target.OpenFindings += oldRisk.OpenFindings
	target.UpdatedAt = time.Now().UTC()
	if err := tx.Save(&target).Error; err != nil {
		return err
	}
	return tx.Where("asset_id = ?", oldID).Delete(&database.AssetRisk{}).Error
}
