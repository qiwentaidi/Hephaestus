package configmigration

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
)

var validScanTemplateTypes = map[string]struct{}{
	"webscan":     {},
	"portscan":    {},
	"dirsearch":   {},
	"jsscan":      {},
	"crack":       {},
	"combination": {},
	"ai_dialogue": {},
}

// Import validates a bundle, decrypts its secret fields and applies the
// selected modules. AI, API key and notification modules replace the current
// configuration; scan templates are matched by name, updated when they already
// exist and created otherwise.
func Import(request ImportRequest) (ImportResult, error) {
	if strings.TrimSpace(request.Bundle.Schema) != BundleSchema {
		return ImportResult{}, invalid("配置文件格式无效，请选择由本系统导出的配置文件")
	}
	if request.Bundle.Version != BundleVersion {
		return ImportResult{}, invalid("配置文件版本 %d 不受支持，当前支持版本 %d", request.Bundle.Version, BundleVersion)
	}

	available := make(map[string]json.RawMessage, len(request.Bundle.Modules))
	for _, module := range request.Bundle.Modules {
		if !isSupportedModule(module.ID) {
			return ImportResult{}, invalid("配置文件包含不支持的模块: %s", module.ID)
		}
		if _, exists := available[module.ID]; exists {
			return ImportResult{}, invalid("配置文件包含重复模块: %s", module.ID)
		}
		if len(module.Data) == 0 || string(module.Data) == "null" {
			return ImportResult{}, invalid("配置文件模块 %s 缺少数据", module.ID)
		}
		available[module.ID] = module.Data
	}

	requested := request.Modules
	if len(requested) == 0 {
		for _, module := range request.Bundle.Modules {
			requested = append(requested, module.ID)
		}
	}
	modules, err := normalizeModules(requested)
	if err != nil {
		return ImportResult{}, err
	}
	for _, id := range modules {
		if _, ok := available[id]; !ok {
			return ImportResult{}, invalid("配置文件中不存在模块: %s", id)
		}
	}

	// Decrypt and validate every selected module before touching live config so
	// a malformed file does not leave the installation half-imported.
	payloads := make(map[string]json.RawMessage, len(modules))
	for _, id := range modules {
		payload, err := decryptModule(available[id])
		if err != nil {
			return ImportResult{}, invalid("解析模块 %s 失败: %v", id, err)
		}
		if id == ModuleScanTemplates {
			if _, err := decodeScanTemplates(payload); err != nil {
				return ImportResult{}, err
			}
		}
		payloads[id] = payload
	}

	result := ImportResult{Modules: make([]ModuleResult, 0, len(modules))}
	for _, id := range moduleOrder {
		payload, ok := payloads[id]
		if !ok {
			continue
		}
		var moduleResult ModuleResult
		switch id {
		case ModuleAI:
			moduleResult, err = applyAI(payload)
		case ModuleAPIKeys:
			moduleResult, err = applyAPIKeys(payload)
		case ModuleNotifications:
			moduleResult, err = applyNotifications(payload)
		case ModuleScanTemplates:
			moduleResult, err = applyScanTemplates(payload)
		default:
			err = fmt.Errorf("不支持的配置模块: %s", id)
		}
		if err != nil {
			return result, fmt.Errorf("导入模块 %s 失败: %w", id, err)
		}
		moduleResult.ID = id
		result.Modules = append(result.Modules, moduleResult)
	}

	return result, nil
}

func decryptModule(data json.RawMessage) (json.RawMessage, error) {
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("配置数据格式无效: %w", err)
	}
	decrypted, err := decryptSecrets(payload)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(decrypted)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func applyAI(payload json.RawMessage) (ModuleResult, error) {
	var aiConfig config.AIConfig
	if err := json.Unmarshal(payload, &aiConfig); err != nil {
		return ModuleResult{}, fmt.Errorf("AI 配置格式无效: %w", err)
	}
	aiConfig = config.NormalizeAIConfig(aiConfig)
	if err := config.UpdateAIConfig(aiConfig); err != nil {
		return ModuleResult{}, err
	}
	return ModuleResult{Applied: len(aiConfig.Providers)}, nil
}

func applyAPIKeys(payload json.RawMessage) (ModuleResult, error) {
	var spaceConfig config.SpaceEngineConfig
	if err := json.Unmarshal(payload, &spaceConfig); err != nil {
		return ModuleResult{}, fmt.Errorf("API 密钥配置格式无效: %w", err)
	}
	if err := config.UpdateSpaceEngineConfig(spaceConfig); err != nil {
		return ModuleResult{}, err
	}
	return ModuleResult{Applied: countConfiguredAPIKeys(spaceConfig)}, nil
}

func applyNotifications(payload json.RawMessage) (ModuleResult, error) {
	var notificationConfig config.NotificationConfig
	if err := json.Unmarshal(payload, &notificationConfig); err != nil {
		return ModuleResult{}, fmt.Errorf("通知配置格式无效: %w", err)
	}
	if err := config.UpdateNotificationConfig(notificationConfig); err != nil {
		return ModuleResult{}, err
	}
	return ModuleResult{Applied: len(notificationConfig.Channels)}, nil
}

func countConfiguredAPIKeys(spaceConfig config.SpaceEngineConfig) int {
	values := []string{
		spaceConfig.FOFA.Key,
		spaceConfig.Hunter.Key,
		spaceConfig.Quake.Key,
		spaceConfig.Censys.Token,
		spaceConfig.Zoomeye.Key,
		spaceConfig.Github.Token,
		spaceConfig.Chaos.Key,
		spaceConfig.Bevigil.Key,
		spaceConfig.Securitytrails.Key,
		spaceConfig.CTRadar.Key,
	}
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func decodeScanTemplates(payload json.RawMessage) ([]exportedScanTemplate, error) {
	var templates []exportedScanTemplate
	if err := json.Unmarshal(payload, &templates); err != nil {
		return nil, invalid("扫描模板配置格式无效: %v", err)
	}
	for index := range templates {
		normalized, err := normalizeScanTemplate(templates[index])
		if err != nil {
			return nil, invalid("第 %d 个扫描模板无效: %v", index+1, err)
		}
		templates[index] = normalized
	}
	return templates, nil
}

func normalizeScanTemplate(template exportedScanTemplate) (exportedScanTemplate, error) {
	name := strings.TrimSpace(template.Name)
	if name == "" {
		return template, errors.New("模板名称不能为空")
	}
	taskType := strings.TrimSpace(template.TaskType)
	if taskType == "" {
		taskType = "webscan"
	}
	if _, ok := validScanTemplateTypes[taskType]; !ok {
		return template, fmt.Errorf("无效的任务类型: %s", taskType)
	}

	configText := strings.TrimSpace(string(template.Config))
	if configText == "null" {
		configText = ""
	}
	if configText != "" && !json.Valid([]byte(configText)) {
		return template, errors.New("模板配置不是有效的 JSON")
	}
	if taskType == "ai_dialogue" {
		if configText == "" {
			return template, errors.New("AI 对话模板配置无效")
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(configText), &values); err != nil {
			return template, errors.New("AI 对话模板配置无效")
		}
		prompt, _ := values["prompt"].(string)
		if strings.TrimSpace(prompt) == "" {
			return template, errors.New("AI 对话模板规则不能为空")
		}
		if !strings.Contains(prompt, "{{.goal}}") {
			return template, errors.New("AI 对话模板规则必须包含 {{.goal}} 占位符")
		}
	}

	template.Name = name
	template.TaskType = taskType
	if configText == "" {
		template.Config = nil
	} else {
		template.Config = json.RawMessage(configText)
	}
	return template, nil
}

func applyScanTemplates(payload json.RawMessage) (ModuleResult, error) {
	templates, err := decodeScanTemplates(payload)
	if err != nil {
		return ModuleResult{}, err
	}
	if len(templates) == 0 {
		return ModuleResult{}, nil
	}

	db, err := database.GetDB()
	if err != nil {
		return ModuleResult{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := ModuleResult{}
	tx := db.Begin()
	if tx.Error != nil {
		return ModuleResult{}, tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	for _, template := range templates {
		configJSON := ""
		if len(template.Config) > 0 {
			configJSON = string(template.Config)
		}

		var existing database.ScanTemplate
		lookupErr := tx.Where("name = ?", template.Name).Order("created_at asc").First(&existing).Error
		now := time.Now()
		switch {
		case errors.Is(lookupErr, gorm.ErrRecordNotFound):
			record := database.ScanTemplate{
				ID:          database.GenerateNanoid(),
				Name:        template.Name,
				Description: template.Description,
				TaskType:    template.TaskType,
				Config:      configJSON,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(&record).Error; err != nil {
				return ModuleResult{}, err
			}
			result.Created++
			result.Applied++
		case lookupErr != nil:
			return ModuleResult{}, lookupErr
		default:
			updates := map[string]any{
				"description": template.Description,
				"task_type":   template.TaskType,
				"config":      configJSON,
				"updated_at":  now,
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return ModuleResult{}, err
			}
			result.Updated++
			result.Applied++
		}
	}

	if err := tx.Commit().Error; err != nil {
		return ModuleResult{}, err
	}
	committed = true
	return result, nil
}
