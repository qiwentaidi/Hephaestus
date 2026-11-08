package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	codeInvalidPayload    = "invalid_payload"
	codeInvalidName       = "invalid_name"
	codeInvalidDefinition = "invalid_definition"
	codeDuplicateName     = "duplicate_name"
	codeNotFound          = "not_found"
	codeTooManyThemes     = "too_many_themes"
	codeInternal          = "internal_error"
)

// openThemeDB 允许测试注入数据库连接。
var openThemeDB = database.GetDB

type writeRequest struct {
	Name       string     `json:"name"`
	Definition Definition `json:"definition"`
}

type importRequest struct {
	Themes []writeRequest `json:"themes"`
}

type themePayload struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Definition Definition `json:"definition"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// ListCustomThemesHandler 返回全部自定义主题。
func ListCustomThemesHandler(c *gin.Context) {
	db, err := openThemeDB()
	if err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "获取数据库连接失败")
		return
	}

	var records []database.CustomTheme
	if err := db.Order("updated_at DESC, id DESC").Find(&records).Error; err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "获取主题列表失败")
		return
	}

	payloads := make([]themePayload, 0, len(records))
	for _, record := range records {
		payload, err := payloadFromRecord(record)
		if err != nil {
			log.Printf("skip invalid custom theme %d: %v", record.ID, err)
			continue
		}
		payloads = append(payloads, payload)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payloads})
}

// CreateCustomThemeHandler 新建自定义主题。
func CreateCustomThemeHandler(c *gin.Context) {
	var req writeRequest
	if err := decodeJSON(c, &req); err != nil {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPayload, "无效的请求参数")
		return
	}

	name, nameKey, definition, raw, err := normalizeWriteRequest(req)
	if err != nil {
		writeThemeError(c, http.StatusBadRequest, codeForValidationError(err), err.Error())
		return
	}

	db, err := openThemeDB()
	if err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "获取数据库连接失败")
		return
	}

	var existing int64
	if err := db.Model(&database.CustomTheme{}).Where("name_key = ?", nameKey).Count(&existing).Error; err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "检查主题名称失败")
		return
	}
	if existing > 0 {
		writeDuplicateError(c, []string{name})
		return
	}

	record := database.CustomTheme{Name: name, NameKey: nameKey, Definition: raw}
	if err := db.Create(&record).Error; err != nil {
		if isUniqueViolation(err) {
			writeDuplicateError(c, []string{name})
			return
		}
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "保存主题失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": themePayload{
			ID:         record.ID,
			Name:       record.Name,
			Definition: definition,
			CreatedAt:  record.CreatedAt,
			UpdatedAt:  record.UpdatedAt,
		},
	})
}

// UpdateCustomThemeHandler 更新自定义主题。
func UpdateCustomThemeHandler(c *gin.Context) {
	id, ok := parseThemeID(c)
	if !ok {
		return
	}

	var req writeRequest
	if err := decodeJSON(c, &req); err != nil {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPayload, "无效的请求参数")
		return
	}

	name, nameKey, definition, raw, err := normalizeWriteRequest(req)
	if err != nil {
		writeThemeError(c, http.StatusBadRequest, codeForValidationError(err), err.Error())
		return
	}

	db, err := openThemeDB()
	if err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "获取数据库连接失败")
		return
	}

	var record database.CustomTheme
	if err := db.First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeThemeError(c, http.StatusNotFound, codeNotFound, "主题不存在")
			return
		}
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "读取主题失败")
		return
	}

	var existing int64
	if err := db.Model(&database.CustomTheme{}).
		Where("name_key = ? AND id <> ?", nameKey, id).
		Count(&existing).Error; err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "检查主题名称失败")
		return
	}
	if existing > 0 {
		writeDuplicateError(c, []string{name})
		return
	}

	record.Name = name
	record.NameKey = nameKey
	record.Definition = raw
	if err := db.Save(&record).Error; err != nil {
		if isUniqueViolation(err) {
			writeDuplicateError(c, []string{name})
			return
		}
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "保存主题失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": themePayload{
			ID:         record.ID,
			Name:       record.Name,
			Definition: definition,
			CreatedAt:  record.CreatedAt,
			UpdatedAt:  record.UpdatedAt,
		},
	})
}

// DeleteCustomThemeHandler 删除自定义主题。
func DeleteCustomThemeHandler(c *gin.Context) {
	id, ok := parseThemeID(c)
	if !ok {
		return
	}

	db, err := openThemeDB()
	if err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "获取数据库连接失败")
		return
	}

	result := db.Delete(&database.CustomTheme{}, id)
	if result.Error != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "删除主题失败")
		return
	}
	if result.RowsAffected == 0 {
		writeThemeError(c, http.StatusNotFound, codeNotFound, "主题不存在")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ImportCustomThemesHandler 原子导入一批自定义主题。
// 任意一个名称冲突（包内重复或库内已存在）都会拒绝整个导入。
func ImportCustomThemesHandler(c *gin.Context) {
	var req importRequest
	if err := decodeJSON(c, &req); err != nil {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPayload, "无效的导入文件")
		return
	}
	if len(req.Themes) == 0 {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPayload, "导入文件中没有主题")
		return
	}
	if len(req.Themes) > MaxImportThemes {
		writeThemeError(c, http.StatusBadRequest, codeTooManyThemes,
			fmt.Sprintf("一次最多导入 %d 个主题", MaxImportThemes))
		return
	}

	type normalizedTheme struct {
		name       string
		nameKey    string
		raw        string
		definition Definition
	}

	normalized := make([]normalizedTheme, 0, len(req.Themes))
	seen := make(map[string]string, len(req.Themes))
	conflicts := make([]string, 0)
	for _, item := range req.Themes {
		name, nameKey, definition, raw, err := normalizeWriteRequest(item)
		if err != nil {
			writeThemeError(c, http.StatusBadRequest, codeForValidationError(err), err.Error())
			return
		}
		if previous, exists := seen[nameKey]; exists {
			conflicts = appendUnique(conflicts, previous)
			conflicts = appendUnique(conflicts, name)
			continue
		}
		seen[nameKey] = name
		normalized = append(normalized, normalizedTheme{
			name:       name,
			nameKey:    nameKey,
			raw:        raw,
			definition: definition,
		})
	}
	if len(conflicts) > 0 {
		writeDuplicateError(c, conflicts)
		return
	}

	db, err := openThemeDB()
	if err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "获取数据库连接失败")
		return
	}

	keys := make([]string, 0, len(normalized))
	for _, item := range normalized {
		keys = append(keys, item.nameKey)
	}
	var existing []database.CustomTheme
	if err := db.Where("name_key IN ?", keys).Find(&existing).Error; err != nil {
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "检查主题名称失败")
		return
	}
	if len(existing) > 0 {
		conflicts = conflicts[:0]
		for _, record := range existing {
			conflicts = appendUnique(conflicts, record.Name)
		}
		writeDuplicateError(c, conflicts)
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, item := range normalized {
			record := database.CustomTheme{
				Name:       item.name,
				NameKey:    item.nameKey,
				Definition: item.raw,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeDuplicateError(c, conflictsFromImport(req))
			return
		}
		writeThemeError(c, http.StatusInternalServerError, codeInternal, "导入主题失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"created": len(normalized)},
	})
}

func normalizeWriteRequest(req writeRequest) (name string, nameKey string, definition Definition, raw string, err error) {
	name, nameKey, err = NormalizeName(req.Name)
	if err != nil {
		return "", "", Definition{}, "", err
	}
	definition, err = NormalizeDefinition(req.Definition)
	if err != nil {
		return "", "", Definition{}, "", err
	}
	raw, err = EncodeDefinition(definition)
	if err != nil {
		return "", "", Definition{}, "", err
	}
	return name, nameKey, definition, raw, nil
}

func payloadFromRecord(record database.CustomTheme) (themePayload, error) {
	definition, err := DecodeDefinition(record.Definition)
	if err != nil {
		return themePayload{}, err
	}
	return themePayload{
		ID:         record.ID,
		Name:       record.Name,
		Definition: definition,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}, nil
}

func parseThemeID(c *gin.Context) (uint, bool) {
	value := strings.TrimSpace(c.Param("id"))
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil || parsed == 0 {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPayload, "无效的主题 ID")
		return 0, false
	}
	return uint(parsed), true
}

func decodeJSON(c *gin.Context, dst any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxRequestBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("unexpected trailing JSON data")
	}
	return nil
}

func codeForValidationError(err error) string {
	message := err.Error()
	if strings.Contains(message, "名称") {
		return codeInvalidName
	}
	return codeInvalidDefinition
}

func writeThemeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"code":    code,
		"error":   message,
	})
}

func writeDuplicateError(c *gin.Context, names []string) {
	c.JSON(http.StatusConflict, gin.H{
		"success":   false,
		"code":      codeDuplicateName,
		"error":     "主题名称已存在",
		"conflicts": names,
	})
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") ||
		strings.Contains(message, "duplicate") ||
		strings.Contains(message, "constraint")
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func conflictsFromImport(req importRequest) []string {
	conflicts := make([]string, 0)
	for _, item := range req.Themes {
		name, _, err := NormalizeName(item.Name)
		if err != nil {
			continue
		}
		conflicts = appendUnique(conflicts, name)
	}
	return conflicts
}
