package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"
)

var (
	settingMu sync.RWMutex
)

const (
	SettingKeySpaceEngines          = "space_engines"
	SettingKeyApiBaseURL            = "api_base_url"
	SettingKeyDingTalk              = "dingtalk"
	SettingKeyWeChat                = "wechat"
	SettingKeyFeishu                = "feishu"
	SettingKeyNotifications         = "notifications"
	SettingKeyAI                    = "ai"
	SettingKeyToolGuard             = "tool_guard"
	SettingKeyUpdateServer          = "update_server"
	SettingKeySavedQueryPresets     = "saved_query_presets"
	SettingKeyScanRuntime           = "scan_runtime"
	SettingKeyPOCTemplateFolders    = "poc_template_folders"
	SettingKeyAgentConfig           = "agent-config"
	SettingKeyDesktopConfig         = "desktop_config"
	SettingKeyDesktopJWTSecret      = "desktop_jwt_secret"
	SettingKeyMiniProgramDecompile  = "mini_program_decompile"
	SettingKeySubdomainDNSServers   = "subdomain_dns_servers"
	SettingKeyAssetLifecycleLastRun = "asset_lifecycle_last_run"
)

type AgentServerSetting struct {
	Server struct {
		Address string `json:"address"`
	} `json:"server"`
}

// GetSetting 从数据库获取配置项
func GetSetting(key string) (interface{}, error) {
	settingMu.RLock()
	defer settingMu.RUnlock()

	db, err := GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var setting Setting
	if err := db.Where("key = ?", key).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 配置不存在，返回 nil
		}
		return nil, fmt.Errorf("查询配置失败: %w", err)
	}

	// 解析 JSON
	var value interface{}
	if err := json.Unmarshal([]byte(setting.Value), &value); err != nil {
		return nil, fmt.Errorf("解析配置JSON失败: %w", err)
	}

	return value, nil
}

// SetSetting 保存配置项到数据库
func SetSetting(key string, value interface{}) error {
	settingMu.Lock()
	defer settingMu.Unlock()

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 将值转换为 JSON
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化配置JSON失败: %w", err)
	}

	// 使用 GORM 的 Clauses 实现 UPSERT 操作
	setting := Setting{
		Key:   key,
		Value: string(valueJSON),
	}

	if err := db.Save(&setting).Error; err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	return nil
}

// DeleteSetting 删除配置项
func DeleteSetting(key string) error {
	settingMu.Lock()
	defer settingMu.Unlock()

	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Where("key = ?", key).Delete(&Setting{})
	if result.Error != nil {
		return fmt.Errorf("删除配置失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("配置不存在: %s", key)
	}

	return nil
}

// GetAllSettings 获取所有配置项
func GetAllSettings() (map[string]interface{}, error) {
	settingMu.RLock()
	defer settingMu.RUnlock()

	db, err := GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var settingsList []Setting
	if err := db.Order("key").Find(&settingsList).Error; err != nil {
		return nil, fmt.Errorf("查询配置列表失败: %w", err)
	}

	settings := make(map[string]interface{})
	for _, setting := range settingsList {
		// 解析 JSON
		var value interface{}
		if err := json.Unmarshal([]byte(setting.Value), &value); err != nil {
			return nil, fmt.Errorf("解析配置JSON失败: %w", err)
		}

		settings[setting.Key] = value
	}

	return settings, nil
}

func LoadAgentServerSetting() (AgentServerSetting, error) {
	var cfg AgentServerSetting

	raw, err := GetSetting(SettingKeyAgentConfig)
	if err != nil || raw == nil {
		return cfg, err
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return cfg, fmt.Errorf("序列化 Agent 配置失败: %w", err)
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 Agent 配置失败: %w", err)
	}

	cfg.Server.Address = strings.TrimSpace(cfg.Server.Address)
	return cfg, nil
}
