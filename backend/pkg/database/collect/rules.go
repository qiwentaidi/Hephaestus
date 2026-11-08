package collect

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/types"

	"gorm.io/gorm"
)

const (
	maxCustomCollectRules  = 100
	maxCustomRuleLabelLen  = 100
	maxCustomRulePatternLen = 2000
)

// ListCustomRules 获取数据库信息采集的自定义规则
func ListCustomRules() ([]types.CollectCustomRule, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var records []database.DBCollectCustomRule
	if err := db.Order("created_at ASC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("查询自定义规则失败: %w", err)
	}

	rules := make([]types.CollectCustomRule, len(records))
	for i, record := range records {
		rules[i] = types.CollectCustomRule{
			ID:      record.ID,
			Label:   record.Label,
			Pattern: record.Pattern,
		}
	}

	return rules, nil
}

// ReplaceCustomRules 全量替换数据库信息采集自定义规则
func ReplaceCustomRules(rules []types.CollectCustomRule) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if len(rules) > maxCustomCollectRules {
		return fmt.Errorf("自定义规则数量不能超过 %d 条", maxCustomCollectRules)
	}

	records := make([]database.DBCollectCustomRule, 0, len(rules))
	now := time.Now()
	for i, rule := range rules {
		label := strings.TrimSpace(rule.Label)
		pattern := strings.TrimSpace(rule.Pattern)

		if label == "" {
			return fmt.Errorf("第 %d 条规则名称不能为空", i+1)
		}
		if pattern == "" {
			return fmt.Errorf("第 %d 条规则正则不能为空", i+1)
		}
		if len(label) > maxCustomRuleLabelLen {
			return fmt.Errorf("第 %d 条规则名称长度不能超过 %d", i+1, maxCustomRuleLabelLen)
		}
		if len(pattern) > maxCustomRulePatternLen {
			return fmt.Errorf("第 %d 条规则正则长度不能超过 %d", i+1, maxCustomRulePatternLen)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("第 %d 条规则正则无效: %w", i+1, err)
		}

		records = append(records, database.DBCollectCustomRule{
			ID:        database.GenerateNanoid(),
			Label:     label,
			Pattern:   pattern,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&database.DBCollectCustomRule{}).Error; err != nil {
			return fmt.Errorf("清空自定义规则失败: %w", err)
		}

		if len(records) == 0 {
			return nil
		}

		if err := tx.Create(&records).Error; err != nil {
			return fmt.Errorf("保存自定义规则失败: %w", err)
		}

		return nil
	})
}
