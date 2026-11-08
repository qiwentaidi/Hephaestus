package scan

import (
	"net/http"
	"sort"
	"strings"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"

	"github.com/gin-gonic/gin"
)

type CustomPortGroup = scantask.CustomPortGroup
type DefaultPortGroup = scantask.DefaultPortGroup

type saveCustomPortGroupRequest struct {
	ID    string `json:"id"`
	Name  string `json:"name" binding:"required"`
	Ports string `json:"ports" binding:"required"`
}

type saveDefaultPortGroupRequest struct {
	ID      string `json:"id" binding:"required"`
	Name    string `json:"name" binding:"required"`
	Ports   string `json:"ports" binding:"required"`
	BuiltIn bool   `json:"builtIn"`
}

func ListCustomPortGroupsHandler(c *gin.Context) {
	groups, err := scantask.LoadCustomPortGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取自定义端口组失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    groups,
	})
}

func GetDefaultPortGroupHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	group, err := scantask.LoadDefaultPortGroupWithDB(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取默认端口组失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": group})
}

func SaveDefaultPortGroupHandler(c *gin.Context) {
	var req saveDefaultPortGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	group := DefaultPortGroup{
		ID:      strings.TrimSpace(req.ID),
		Name:    strings.TrimSpace(req.Name),
		Ports:   scantask.NormalizePorts(req.Ports),
		BuiltIn: req.BuiltIn,
	}
	if group.ID == "" || group.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "端口组 ID 和名称不能为空"})
		return
	}

	if canonical, ok := scantask.CanonicalBuiltInPortGroup(group.ID); ok {
		group = canonical
	} else if group.BuiltIn {
		if !strings.HasPrefix(group.ID, "builtin-") {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "内置端口组 ID 无效"})
			return
		}
	} else {
		groups, loadErr := scantask.LoadCustomPortGroupsWithDB(db)
		if loadErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取自定义端口组失败: " + loadErr.Error()})
			return
		}
		matched := false
		for _, item := range groups {
			if item.ID == group.ID {
				group.Name = item.Name
				group.Ports = item.Ports
				matched = true
				break
			}
		}
		if !matched {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "端口组不存在"})
			return
		}
	}

	if err := scantask.ValidatePortGroupPorts(group.Ports); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := scantask.SaveDefaultPortGroupWithDB(db, group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存默认端口组失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": group})
}

func SaveCustomPortGroupHandler(c *gin.Context) {
	var req saveCustomPortGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	name := strings.TrimSpace(req.Name)
	ports := scantask.NormalizePorts(req.Ports)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "端口组名称不能为空"})
		return
	}
	if err := scantask.ValidatePortGroupPorts(ports); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	groups, err := scantask.LoadCustomPortGroupsWithDB(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取自定义端口组失败: " + err.Error()})
		return
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = database.GenerateNanoid()
	}

	for _, group := range groups {
		if group.ID != id && strings.EqualFold(strings.TrimSpace(group.Name), name) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "端口组名称已存在"})
			return
		}
	}

	saved := CustomPortGroup{
		ID:    id,
		Name:  name,
		Ports: ports,
	}

	updated := false
	for i := range groups {
		if groups[i].ID == id {
			groups[i] = saved
			updated = true
			break
		}
	}
	if !updated {
		groups = append(groups, saved)
	}

	sort.Slice(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})

	if err := database.SetSetting(scantask.SettingKeyCustomPortGroups, groups); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存自定义端口组失败: " + err.Error()})
		return
	}
	if currentDefault, loadErr := scantask.LoadDefaultPortGroupWithDB(db); loadErr == nil && currentDefault.ID == saved.ID {
		_ = scantask.SaveDefaultPortGroupWithDB(db, DefaultPortGroup{ID: saved.ID, Name: saved.Name, Ports: saved.Ports})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    saved,
	})
}

func DeleteCustomPortGroupHandler(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "端口组 ID 不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	groups, err := scantask.LoadCustomPortGroupsWithDB(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取自定义端口组失败: " + err.Error()})
		return
	}

	next := make([]CustomPortGroup, 0, len(groups))
	removed := false
	for _, group := range groups {
		if group.ID == id {
			removed = true
			continue
		}
		next = append(next, group)
	}
	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "端口组不存在"})
		return
	}

	if err := database.SetSetting(scantask.SettingKeyCustomPortGroups, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "删除自定义端口组失败: " + err.Error()})
		return
	}
	if currentDefault, loadErr := scantask.LoadDefaultPortGroupWithDB(db); loadErr == nil && currentDefault.ID == id {
		if saveErr := scantask.SaveDefaultPortGroupWithDB(db, scantask.FallbackDefaultPortGroup()); saveErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "恢复默认端口组失败: " + saveErr.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

type portGroupValidationError struct {
	message string
}

func (e *portGroupValidationError) Error() string {
	return e.message
}
