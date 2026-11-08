package scantask

import (
	"encoding/json"
	"sort"
	"strings"

	portutil "github.com/qiwentaidi/utils/port"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/portpresets"
)

const (
	SettingKeyCustomPortGroups = "custom_port_groups"
	SettingKeyDefaultPortGroup = "default_port_group"
	defaultPortGroupID         = "builtin-top1000"
	defaultPortGroupName       = "top1000"
	customPortGroupID          = "custom"
)

type CustomPortGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Ports string `json:"ports"`
}

type DefaultPortGroup struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Ports   string `json:"ports"`
	BuiltIn bool   `json:"builtIn"`
}

func LoadCustomPortGroups() ([]CustomPortGroup, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return LoadCustomPortGroupsWithDB(db)
}

func LoadCustomPortGroupsWithDB(db *gorm.DB) ([]CustomPortGroup, error) {
	var setting database.Setting
	if err := db.Where("key = ?", SettingKeyCustomPortGroups).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return []CustomPortGroup{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(setting.Value) == "" {
		return []CustomPortGroup{}, nil
	}

	var groups []CustomPortGroup
	if err := json.Unmarshal([]byte(setting.Value), &groups); err != nil {
		return nil, err
	}

	for i := range groups {
		groups[i].Name = strings.TrimSpace(groups[i].Name)
		groups[i].Ports = NormalizePorts(groups[i].Ports)
	}

	sort.Slice(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})

	return groups, nil
}

func FallbackDefaultPortGroup() DefaultPortGroup {
	return DefaultPortGroup{
		ID:      defaultPortGroupID,
		Name:    defaultPortGroupName,
		Ports:   portpresets.WorkbenchPortRangeTop1000,
		BuiltIn: true,
	}
}

func CanonicalBuiltInPortGroup(id string) (DefaultPortGroup, bool) {
	switch strings.TrimSpace(id) {
	case defaultPortGroupID:
		return FallbackDefaultPortGroup(), true
	case "builtin-all":
		return DefaultPortGroup{ID: "builtin-all", Name: "全端口", Ports: portpresets.WorkbenchPortRangeFull, BuiltIn: true}, true
	case "builtin-database":
		return DefaultPortGroup{ID: "builtin-database", Name: "数据库端口", Ports: portpresets.WorkbenchPortRangeDatabase, BuiltIn: true}, true
	default:
		return DefaultPortGroup{}, false
	}
}

func LoadDefaultPortGroupWithDB(db *gorm.DB) (DefaultPortGroup, error) {
	if db == nil {
		return DefaultPortGroup{}, NewPortGroupError("数据库连接不能为空")
	}

	var setting database.Setting
	if err := db.Where("key = ?", SettingKeyDefaultPortGroup).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return FallbackDefaultPortGroup(), nil
		}
		return DefaultPortGroup{}, err
	}

	var group DefaultPortGroup
	if err := json.Unmarshal([]byte(setting.Value), &group); err != nil {
		return FallbackDefaultPortGroup(), nil
	}
	group.ID = strings.TrimSpace(group.ID)
	group.Name = strings.TrimSpace(group.Name)
	group.Ports = NormalizePorts(group.Ports)
	if group.ID == "" || group.Name == "" || ValidatePortGroupPorts(group.Ports) != nil {
		return FallbackDefaultPortGroup(), nil
	}
	return group, nil
}

func SaveDefaultPortGroupWithDB(db *gorm.DB, group DefaultPortGroup) error {
	payload, err := json.Marshal(group)
	if err != nil {
		return err
	}
	return db.Save(&database.Setting{Key: SettingKeyDefaultPortGroup, Value: string(payload)}).Error
}

func ResolvePortGroupByID(db *gorm.DB, id string) (DefaultPortGroup, error) {
	id = strings.TrimSpace(id)
	if canonical, ok := CanonicalBuiltInPortGroup(id); ok {
		return canonical, nil
	}
	if currentDefault, err := LoadDefaultPortGroupWithDB(db); err != nil {
		return DefaultPortGroup{}, err
	} else if currentDefault.ID == id {
		return currentDefault, nil
	}

	groups, err := LoadCustomPortGroupsWithDB(db)
	if err != nil {
		return DefaultPortGroup{}, err
	}
	for _, group := range groups {
		if group.ID == id {
			return DefaultPortGroup{ID: group.ID, Name: group.Name, Ports: group.Ports}, nil
		}
	}
	return DefaultPortGroup{}, NewPortGroupError("端口组不存在: " + id)
}

func ApplyPortScanPortGroupDefaultsWithDB(db *gorm.DB, config map[string]interface{}) error {
	if config == nil {
		return NewPortGroupError("端口扫描配置不能为空")
	}

	if rawRange, ok := config["portRange"].(string); ok && strings.TrimSpace(rawRange) != "" {
		portRange := NormalizePorts(rawRange)
		if err := ValidatePortGroupPorts(portRange); err != nil {
			return err
		}
		config["portRange"] = portRange
		if strings.TrimSpace(StringConfigValue(config, "portGroupId")) == "" {
			config["portGroupId"] = customPortGroupID
		}
		if strings.TrimSpace(StringConfigValue(config, "portGroupName")) == "" {
			name := strings.TrimSpace(StringConfigValue(config, "portGroup"))
			if name == "" {
				name = "自定义端口范围"
			}
			config["portGroupName"] = name
		}
		return nil
	}

	var (
		group DefaultPortGroup
		err   error
	)
	if groupID := strings.TrimSpace(StringConfigValue(config, "portGroupId")); groupID != "" {
		group, err = ResolvePortGroupByID(db, groupID)
	} else {
		group, err = LoadDefaultPortGroupWithDB(db)
	}
	if err != nil {
		return err
	}

	config["portRange"] = group.Ports
	config["portGroupId"] = group.ID
	config["portGroupName"] = group.Name
	config["portGroup"] = group.Name
	return nil
}

func StringConfigValue(config map[string]interface{}, key string) string {
	if value, ok := config[key].(string); ok {
		return value
	}
	return ""
}

func NormalizePorts(raw string) string {
	parts := strings.Split(raw, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized = append(normalized, part)
	}
	return strings.Join(normalized, ",")
}

func ValidatePortGroupPorts(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return NewPortGroupError("端口不能为空")
	}

	ports, err := portutil.ParsePortDefinitions(raw)
	if err != nil {
		return NewPortGroupError("端口格式不正确")
	}
	if len(ports) == 0 {
		return NewPortGroupError("端口不能为空")
	}

	return nil
}

func NewPortGroupError(message string) error {
	return &portGroupValidationError{message: message}
}

type portGroupValidationError struct {
	message string
}

func (e *portGroupValidationError) Error() string {
	return e.message
}
