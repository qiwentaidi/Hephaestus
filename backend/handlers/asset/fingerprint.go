package asset

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"hephaestus-backend/pkg/webscan"
)

type Fingerprint struct {
	Name        string   `yaml:"name" json:"Name"`
	HighRisk    bool     `yaml:"high_risk,omitempty" json:"HighRisk"`
	Description string   `yaml:"description,omitempty" json:"Description"`
	Path        []string `yaml:"path,omitempty" json:"Path"`
	Rule        []string `yaml:"rule" json:"Rule"`
}

// GetFingerprintListHandler 获取指纹列表
func GetFingerprintListHandler(c *gin.Context) {
	// 只返回指纹名称，避免将匹配规则对外暴露
	fingers, err := webscan.LoadBuiltinFingerprints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载指纹库失败"})
		return
	}

	nameSet := make(map[string]struct{})
	for _, item := range fingers {
		name := strings.TrimSpace(item.ProductName)
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	// 维持 map 结构，前端可沿用分组遍历逻辑
	c.JSON(http.StatusOK, gin.H{
		"builtin": names,
	})
}

// AddFingerprintGroupHandler 添加分组
func AddFingerprintGroupHandler(c *gin.Context) {
	type req struct {
		Group string `json:"group" binding:"required"`
	}
	var r req
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	fingerprints, err := readFingerprints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	if _, exists := fingerprints[r.Group]; exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组已存在"})
		return
	}

	fingerprints[r.Group] = []Fingerprint{}
	if err := writeFingerprints(fingerprints); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "分组添加成功"})
}

// DeleteFingerprintGroupHandler 删除分组
func DeleteFingerprintGroupHandler(c *gin.Context) {
	type req struct {
		Group string `json:"group" binding:"required"`
	}
	var r req
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	fingerprints, err := readFingerprints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	if _, exists := fingerprints[r.Group]; !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组不存在"})
		return
	}

	delete(fingerprints, r.Group)
	if err := writeFingerprints(fingerprints); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "分组删除成功"})
}

// AddFingerprintHandler 添加指纹
func AddFingerprintHandler(c *gin.Context) {
	type req struct {
		Group       string      `json:"group" binding:"required"`
		Fingerprint Fingerprint `json:"fingerprint" binding:"required"`
	}
	var r req
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	fingerprints, err := readFingerprints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	if _, exists := fingerprints[r.Group]; !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组不存在"})
		return
	}

	fingerprints[r.Group] = append(fingerprints[r.Group], r.Fingerprint)
	if err := writeFingerprints(fingerprints); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "指纹添加成功"})
}

// DeleteFingerprintHandler 删除指纹
func DeleteFingerprintHandler(c *gin.Context) {
	type req struct {
		Group string `json:"group" binding:"required"`
		Name  string `json:"name" binding:"required"`
	}
	var r req
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	fingerprints, err := readFingerprints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	arr, exists := fingerprints[r.Group]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组不存在"})
		return
	}

	// 根据名称查找
	foundIndex := -1
	for i, fp := range arr {
		if fp.Name == r.Name {
			foundIndex = i
			break
		}
	}

	if foundIndex == -1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "指纹不存在"})
		return
	}

	// 删除
	fingerprints[r.Group] = append(arr[:foundIndex], arr[foundIndex+1:]...)
	if err := writeFingerprints(fingerprints); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "指纹删除成功"})
}

// UpdateFingerprintHandler 更新指纹
func UpdateFingerprintHandler(c *gin.Context) {
	type req struct {
		Group       string      `json:"group" binding:"required"`
		Index       int         `json:"index" binding:"required"`
		Fingerprint Fingerprint `json:"fingerprint" binding:"required"`
	}
	var r req
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	fingerprints, err := readFingerprints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	arr, exists := fingerprints[r.Group]
	if !exists || r.Index < 0 || r.Index >= len(arr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组或指纹不存在"})
		return
	}

	fingerprints[r.Group][r.Index] = r.Fingerprint
	if err := writeFingerprints(fingerprints); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "指纹更新成功"})
}

// readFingerprints 读取指纹文件
func readFingerprints() (map[string][]Fingerprint, error) {
	configPath := filepath.Join("config", "finger.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var fingerprints map[string][]Fingerprint
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &fingerprints); err != nil {
			return nil, err
		}
	} else {
		fingerprints = make(map[string][]Fingerprint)
	}

	return fingerprints, nil
}

// writeFingerprints 写入指纹文件
func writeFingerprints(fingerprints map[string][]Fingerprint) error {
	configPath := filepath.Join("config", "finger.yaml")

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	if err := encoder.Encode(fingerprints); err != nil {
		return err
	}
	encoder.Close()

	return nil
}
