package scan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/pkg/database"
)

type jsScanInterfaceParameter struct {
	Name string `json:"name"`
	In   string `json:"in"`
}

type jsScanInterface struct {
	Method     string                     `json:"method"`
	URL        string                     `json:"url"`
	TargetURL  string                     `json:"targetUrl"`
	Parameters []jsScanInterfaceParameter `json:"parameters"`
}

var openAPIMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true,
	"delete": true, "head": true, "options": true, "trace": true,
}

func openAPIObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func openAPIString(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func openAPIParameterList(value any) []jsScanInterfaceParameter {
	items, _ := value.([]any)
	parameters := make([]jsScanInterfaceParameter, 0, len(items))
	for _, item := range items {
		parameter := openAPIObject(item)
		name, location := openAPIString(parameter["name"]), openAPIString(parameter["in"])
		if name != "" && location != "" {
			parameters = append(parameters, jsScanInterfaceParameter{Name: name, In: location})
		}
	}
	return parameters
}

func openAPIInterfaceParameters(pathParameters []jsScanInterfaceParameter, operation map[string]any) []jsScanInterfaceParameter {
	parameters := make([]jsScanInterfaceParameter, 0, len(pathParameters)+4)
	seen := make(map[string]bool)
	add := func(parameter jsScanInterfaceParameter) {
		key := parameter.In + "\x00" + parameter.Name
		if !seen[key] {
			seen[key] = true
			parameters = append(parameters, parameter)
		}
	}
	for _, parameter := range pathParameters {
		add(parameter)
	}
	for _, parameter := range openAPIParameterList(operation["parameters"]) {
		add(parameter)
	}
	content := openAPIObject(openAPIObject(operation["requestBody"])["content"])
	contentTypes := make([]string, 0, len(content))
	for contentType := range content {
		contentTypes = append(contentTypes, contentType)
	}
	sort.Strings(contentTypes)
	for _, contentType := range contentTypes {
		schema := openAPIObject(openAPIObject(content[contentType])["schema"])
		properties := openAPIObject(schema["properties"])
		propertyNames := make([]string, 0, len(properties))
		for name := range properties {
			propertyNames = append(propertyNames, name)
		}
		sort.Strings(propertyNames)
		for _, name := range propertyNames {
			add(jsScanInterfaceParameter{Name: name, In: "body"})
		}
	}
	return parameters
}

func openAPIBaseURL(document map[string]any, target string) string {
	parsedTarget, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsedTarget.Scheme == "" || parsedTarget.Host == "" {
		return strings.TrimRight(target, "/")
	}
	origin := parsedTarget.Scheme + "://" + parsedTarget.Host
	servers, _ := document["servers"].([]any)
	if len(servers) == 0 {
		return origin
	}
	server := openAPIString(openAPIObject(servers[0])["url"])
	if server == "" || strings.Contains(server, "{") {
		return origin
	}
	parsedServer, err := url.Parse(server)
	if err != nil {
		return origin
	}
	if parsedServer.Scheme != "" && parsedServer.Host != "" {
		return strings.TrimRight(server, "/")
	}
	return strings.TrimRight(origin, "/") + "/" + strings.TrimLeft(server, "/")
}

func interfacesFromOpenAPI(row database.VulnScanJSOpenAPIDocument) ([]jsScanInterface, error) {
	var document map[string]any
	if err := json.Unmarshal([]byte(row.Document), &document); err != nil {
		return nil, fmt.Errorf("解析 OpenAPI 文档 %s: %w", row.TargetURL, err)
	}
	paths := openAPIObject(document["paths"])
	baseURL := openAPIBaseURL(document, row.TargetURL)
	result := make([]jsScanInterface, 0)
	for path, pathValue := range paths {
		pathItem := openAPIObject(pathValue)
		pathParameters := openAPIParameterList(pathItem["parameters"])
		for method, operationValue := range pathItem {
			method = strings.ToLower(method)
			if !openAPIMethods[method] {
				continue
			}
			fullURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
			if parsedPath, err := url.Parse(path); err == nil && parsedPath.Scheme != "" && parsedPath.Host != "" {
				fullURL = path
			}
			result = append(result, jsScanInterface{
				Method: strings.ToUpper(method), URL: fullURL, TargetURL: row.TargetURL,
				Parameters: openAPIInterfaceParameters(pathParameters, openAPIObject(operationValue)),
			})
		}
	}
	return result, nil
}

// GetJSScanInterfacesHandler returns a paged interface view of Trailblazer's
// versioned OpenAPI snapshots, without exposing the full documents to the UI.
func GetJSScanInterfacesHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	taskID := c.Param("id")
	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}
	query := db.Model(&database.VulnScanJSOpenAPIDocument{}).Where("task_id = ?", taskID)
	if version := strings.TrimSpace(c.Query("version")); version != "" {
		query = query.Where("version = ?", version)
	}
	var documents []database.VulnScanJSOpenAPIDocument
	if err := query.Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取 OpenAPI 文档失败: " + err.Error()})
		return
	}

	interfaces := make([]jsScanInterface, 0)
	targetSet := make(map[string]bool)
	for _, document := range documents {
		targetSet[document.TargetURL] = true
		items, err := interfacesFromOpenAPI(document)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		interfaces = append(interfaces, items...)
	}
	totalAll := len(interfaces)
	targets := make([]string, 0, len(targetSet))
	for target := range targetSet {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	method := strings.ToUpper(strings.TrimSpace(c.Query("method")))
	target := strings.TrimSpace(c.Query("target"))
	filtered := make([]jsScanInterface, 0, len(interfaces))
	for _, item := range interfaces {
		if method != "" && method != "ALL" && item.Method != method {
			continue
		}
		if target != "" && item.TargetURL != target {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(item.URL), keyword) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].URL == filtered[j].URL {
			return filtered[i].Method < filtered[j].Method
		}
		return filtered[i].URL < filtered[j].URL
	})
	total := len(filtered)
	page := parseQueryInt(c.Query("page"), 1)
	pageSize := parseQueryInt(c.Query("pageSize"), 20)
	if pageSize > 200 {
		pageSize = 200
	}
	start := total
	if page-1 <= total/pageSize {
		start = (page - 1) * pageSize
		if start > total {
			start = total
		}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"interfaces": filtered[start:end], "total": total, "totalAll": totalAll,
		"targets": targets, "page": page, "pageSize": pageSize,
	}})
}
