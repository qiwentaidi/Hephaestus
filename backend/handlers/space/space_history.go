package space

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/space/engine"
)

type spaceSearchHistoryDetail struct {
	ID              string                         `json:"id"`
	SessionID       string                         `json:"sessionId"`
	Query           string                         `json:"query"`
	QueryType       string                         `json:"queryType"`
	GroupID         string                         `json:"groupId"`
	GroupName       string                         `json:"groupName"`
	Engines         []string                       `json:"engines"`
	Results         []engine.UnifiedResult         `json:"results"`
	EngineStatus    map[string]engine.EngineStatus `json:"engineStatus"`
	TotalCount      int                            `json:"totalCount"`
	HasMore         bool                           `json:"hasMore"`
	ReachedMaxLimit bool                           `json:"reachedMaxLimit"`
	MaxResults      int                            `json:"maxResults"`
	CreatedBy       string                         `json:"createdBy"`
	CreatedAt       string                         `json:"createdAt"`
	UpdatedAt       string                         `json:"updatedAt"`
}

type spaceSearchHistoryListItem struct {
	ID              string   `json:"id"`
	SessionID       string   `json:"sessionId"`
	Query           string   `json:"query"`
	QueryType       string   `json:"queryType"`
	GroupID         string   `json:"groupId"`
	GroupName       string   `json:"groupName"`
	Engines         []string `json:"engines"`
	TotalCount      int      `json:"totalCount"`
	HasMore         bool     `json:"hasMore"`
	ReachedMaxLimit bool     `json:"reachedMaxLimit"`
	MaxResults      int      `json:"maxResults"`
	CreatedBy       string   `json:"createdBy"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type assignSpaceSearchHistoryGroupRequest struct {
	HistoryIDs []string `json:"historyIds"`
	GroupID    string   `json:"groupId"`
	GroupName  string   `json:"groupName"`
	ClearGroup bool     `json:"clearGroup"`
}

type renameSpaceSearchHistoryGroupRequest struct {
	Name string `json:"name"`
}

func persistSpaceSearchHistory(session *engine.SearchSession, username string) error {
	if session.HistoryID == "" {
		session.HistoryID = database.GenerateNanoid()
	}
	return upsertSpaceSearchHistoryRecord(
		session.HistoryID,
		session.ID,
		session.Query,
		string(session.QueryType),
		session.EnginePages,
		session.Results,
		session.EngineStatus,
		session.HasMore,
		session.ReachedMaxLimit,
		session.MaxResults,
		username,
	)
}

func upsertSpaceSearchHistoryRecord(
	historyID string,
	sessionID string,
	query string,
	queryType string,
	enginePages map[string]*engine.EnginePageState,
	results []engine.UnifiedResult,
	engineStatus map[string]engine.EngineStatus,
	hasMore bool,
	reachedMaxLimit bool,
	maxResults int,
	username string,
) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return err
	}

	engineStatusJSON, err := json.Marshal(engineStatus)
	if err != nil {
		return err
	}

	engineNames := make([]string, 0, len(enginePages))
	for name := range enginePages {
		engineNames = append(engineNames, name)
	}
	enginesJSON, err := json.Marshal(engineNames)
	if err != nil {
		return err
	}

	if historyID == "" {
		historyID = database.GenerateNanoid()
	}

	record := &database.SpaceSearchHistory{
		ID:              historyID,
		SessionID:       sessionID,
		Query:           query,
		QueryType:       queryType,
		Engines:         string(enginesJSON),
		Results:         string(resultsJSON),
		EngineStatus:    string(engineStatusJSON),
		TotalCount:      len(results),
		HasMore:         hasMore,
		ReachedMaxLimit: reachedMaxLimit,
		MaxResults:      maxResults,
		CreatedBy:       username,
		UpdatedAt:       time.Now(),
	}

	var existing database.SpaceSearchHistory
	err = db.Where("session_id = ?", sessionID).First(&existing).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(record).Error
		}
		return err
	}

	existing.Query = record.Query
	existing.QueryType = record.QueryType
	existing.Engines = record.Engines
	existing.Results = record.Results
	existing.EngineStatus = record.EngineStatus
	existing.TotalCount = record.TotalCount
	existing.HasMore = record.HasMore
	existing.ReachedMaxLimit = record.ReachedMaxLimit
	existing.MaxResults = record.MaxResults
	existing.CreatedBy = record.CreatedBy
	existing.UpdatedAt = record.UpdatedAt

	return db.Save(&existing).Error
}

func decodeSpaceSearchHistory(record *database.SpaceSearchHistory) (*spaceSearchHistoryDetail, error) {
	var (
		engines      []string
		results      []engine.UnifiedResult
		engineStatus map[string]engine.EngineStatus
	)

	if strings.TrimSpace(record.Engines) != "" {
		if err := json.Unmarshal([]byte(record.Engines), &engines); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(record.Results) != "" {
		if err := json.Unmarshal([]byte(record.Results), &results); err != nil {
			return nil, err
		}
		// Older records persisted duplicate cross-engine results; dedupe on
		// read so reopening a history tab matches the live search table.
		results = engine.DeduplicateResults(results)
	}
	if strings.TrimSpace(record.EngineStatus) != "" {
		if err := json.Unmarshal([]byte(record.EngineStatus), &engineStatus); err != nil {
			return nil, err
		}
	}
	if engineStatus == nil {
		engineStatus = make(map[string]engine.EngineStatus)
	}

	return &spaceSearchHistoryDetail{
		ID:              record.ID,
		SessionID:       record.SessionID,
		Query:           record.Query,
		QueryType:       record.QueryType,
		GroupID:         record.GroupID,
		GroupName:       record.GroupName,
		Engines:         engines,
		Results:         results,
		EngineStatus:    engineStatus,
		TotalCount:      len(results),
		HasMore:         record.HasMore,
		ReachedMaxLimit: record.ReachedMaxLimit,
		MaxResults:      record.MaxResults,
		CreatedBy:       record.CreatedBy,
		CreatedAt:       record.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:       record.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

func ListSpaceSearchHistoryHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	keyword := strings.TrimSpace(c.Query("keyword"))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	query := db.Model(&database.SpaceSearchHistory{}).Where("created_by = ?", username)
	if keyword != "" {
		query = query.Where("query LIKE ?", "%"+keyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询搜索历史失败: " + err.Error()})
		return
	}

	var records []database.SpaceSearchHistory
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取搜索历史失败: " + err.Error()})
		return
	}

	list := make([]spaceSearchHistoryListItem, 0, len(records))
	for _, record := range records {
		item := spaceSearchHistoryListItem{
			ID:              record.ID,
			SessionID:       record.SessionID,
			Query:           record.Query,
			QueryType:       record.QueryType,
			GroupID:         record.GroupID,
			GroupName:       record.GroupName,
			TotalCount:      record.TotalCount,
			HasMore:         record.HasMore,
			ReachedMaxLimit: record.ReachedMaxLimit,
			MaxResults:      record.MaxResults,
			CreatedBy:       record.CreatedBy,
			CreatedAt:       record.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:       record.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		if strings.TrimSpace(record.Engines) != "" {
			_ = json.Unmarshal([]byte(record.Engines), &item.Engines)
		}
		list = append(list, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"list":     list,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

func GetSpaceSearchHistoryHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	id := c.Param("id")

	var record database.SpaceSearchHistory
	if err := db.Where("id = ? AND created_by = ?", id, username).First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "搜索历史不存在"})
		return
	}

	detail, err := decodeSpaceSearchHistory(&record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "解析搜索历史失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    detail,
	})
}

func DeleteSpaceSearchHistoryHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	id := c.Param("id")

	if err := db.Delete(&database.SpaceSearchHistory{}, "id = ? AND created_by = ?", id, username).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "删除搜索历史失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "搜索历史已删除",
	})
}

func AssignSpaceSearchHistoryGroupHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	var req assignSpaceSearchHistoryGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	historyIDs := make([]string, 0, len(req.HistoryIDs))
	for _, id := range req.HistoryIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			historyIDs = append(historyIDs, id)
		}
	}
	if len(historyIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请选择至少一条历史记录"})
		return
	}

	groupID := strings.TrimSpace(req.GroupID)
	groupName := strings.TrimSpace(req.GroupName)

	if req.ClearGroup {
		groupID = ""
		groupName = ""
	} else {
		if groupID == "" {
			if groupName == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请输入分组名称"})
				return
			}
			groupID = database.GenerateNanoid()
		} else if groupName == "" {
			var existing database.SpaceSearchHistory
			if err := db.Select("group_name").
				Where("group_id = ? AND created_by = ?", groupID, username).
				First(&existing).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "目标分组不存在"})
				return
			}
			groupName = existing.GroupName
		}
	}

	result := db.Model(&database.SpaceSearchHistory{}).
		Where("created_by = ? AND id IN ?", username, historyIDs).
		Updates(map[string]any{
			"group_id":   groupID,
			"group_name": groupName,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "更新分组失败: " + result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "未找到可更新的历史记录"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"groupId":   groupID,
			"groupName": groupName,
		},
		"message": "历史记录分组已更新",
	})
}

func RenameSpaceSearchHistoryGroupHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	groupID := strings.TrimSpace(c.Param("groupId"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "分组 ID 不能为空"})
		return
	}

	var req renameSpaceSearchHistoryGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "分组名称不能为空"})
		return
	}

	result := db.Model(&database.SpaceSearchHistory{}).
		Where("group_id = ? AND created_by = ?", groupID, username).
		Updates(map[string]any{
			"group_name": name,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "重命名分组失败: " + result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "分组不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"groupId":   groupID,
			"groupName": name,
		},
		"message": "分组已重命名",
	})
}
