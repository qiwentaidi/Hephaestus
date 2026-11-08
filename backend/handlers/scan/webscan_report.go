package scan

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/report"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mergeReportTaskItem struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type mergeWebScanReportRequest struct {
	Tasks    []mergeReportTaskItem `json:"tasks"`
	Format   string                `json:"format"`
	Formats  []string              `json:"formats"`
	FileName string                `json:"fileName"`
}

type generatedReportFile struct {
	Name        string
	ContentType string
	Bytes       []byte
}

func normalizeReportFormats(format string, formats []string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, max(1, len(formats)))

	appendFormat := func(raw string) error {
		norm := strings.ToLower(strings.TrimSpace(raw))
		if norm == "" {
			return nil
		}
		switch norm {
		case "doc", "docx", "rtf":
			norm = "word"
		case "excel":
			norm = "xlsx"
		}
		if norm != "word" && norm != "html" && norm != "xlsx" {
			return fmt.Errorf("不支持的导出格式: %s", raw)
		}
		if _, ok := seen[norm]; ok {
			return nil
		}
		seen[norm] = struct{}{}
		result = append(result, norm)
		return nil
	}

	if err := appendFormat(format); err != nil {
		return nil, err
	}
	for _, item := range formats {
		if err := appendFormat(item); err != nil {
			return nil, err
		}
	}
	if len(result) == 0 {
		result = append(result, "word")
	}
	return result, nil
}

func packageReportsAsZip(fileName string, files []generatedReportFile) (generatedReportFile, error) {
	base := strings.TrimSpace(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if base == "" {
		base = "reports"
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range files {
		w, err := zw.Create(file.Name)
		if err != nil {
			_ = zw.Close()
			return generatedReportFile{}, err
		}
		if _, err := w.Write(file.Bytes); err != nil {
			_ = zw.Close()
			return generatedReportFile{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return generatedReportFile{}, err
	}

	return generatedReportFile{
		Name:        base + ".zip",
		ContentType: "application/zip",
		Bytes:       buf.Bytes(),
	}, nil
}

// ExportMergedWebScanReportsHandler 合并导出多个扫描任务报告
func ExportMergedWebScanReportsHandler(c *gin.Context) {
	var req mergeWebScanReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数无效: " + err.Error(),
		})
		return
	}

	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请至少选择一个任务",
		})
		return
	}
	if len(req.Tasks) > 30 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "一次最多合并导出 30 个任务",
		})
		return
	}

	refs := make([]report.MergeTaskRef, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		id := strings.TrimSpace(t.ID)
		if id == "" || t.Version <= 0 {
			continue
		}
		refs = append(refs, report.MergeTaskRef{
			ID:      id,
			Version: t.Version,
		})
	}
	if len(refs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务参数无效",
		})
		return
	}

	formats, err := normalizeReportFormats(req.Format, req.Formats)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	buildMergedReport := func(norm string) ([]byte, string, string, error) {
		switch norm {
		case "word":
			b, err := report.GenerateMergedWordReportFromDB(refs, req.FileName)
			return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
		case "html":
			b, err := report.GenerateMergedHTMLReportFromDB(refs, req.FileName)
			return b, "text/html; charset=utf-8", ".html", err
		case "xlsx":
			b, err := report.GenerateMergedXLSXReportFromDB(refs, req.FileName)
			return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
		default:
			return nil, "", "", fmt.Errorf("不支持的导出格式: %s", req.Format)
		}
	}

	sanitizeFilenameBase := func(input string) string {
		base := strings.TrimSpace(input)
		if base == "" {
			return ""
		}
		base = strings.TrimSuffix(base, filepath.Ext(base))
		base = strings.TrimSpace(base)
		if base == "" {
			return ""
		}

		replacer := strings.NewReplacer(
			"/", "_",
			"\\", "_",
			":", "_",
			"*", "_",
			"?", "_",
			"\"", "_",
			"<", "_",
			">", "_",
			"|", "_",
			"\n", "_",
			"\r", "_",
			"\t", "_",
		)
		base = replacer.Replace(base)
		base = strings.Trim(base, " .")

		const maxLen = 120
		r := []rune(base)
		if len(r) > maxLen {
			base = string(r[:maxLen])
		}
		return base
	}

	fileBase := sanitizeFilenameBase(req.FileName)
	if fileBase == "" {
		fileBase = fmt.Sprintf("攻击面评估合并报告_%d项_%s", len(refs), time.Now().Format("20060102"))
	}

	files := make([]generatedReportFile, 0, len(formats))
	for _, norm := range formats {
		reportBytes, contentType, ext, err := buildMergedReport(norm)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "不支持的导出格式") ||
				strings.Contains(err.Error(), "未提供有效任务") ||
				strings.Contains(err.Error(), "尚未完成") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"success": false,
				"error":   "生成合并报告失败: " + err.Error(),
			})
			return
		}
		files = append(files, generatedReportFile{
			Name:        fileBase + ext,
			ContentType: contentType,
			Bytes:       reportBytes,
		})
	}

	output := files[0]
	if len(files) > 1 {
		output, err = packageReportsAsZip(fileBase, files)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "打包合并报告失败: " + err.Error(),
			})
			return
		}
	}

	quoted := strings.ReplaceAll(output.Name, `"`, "_")
	escaped := url.PathEscape(output.Name)
	c.Header("Content-Type", output.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, quoted, escaped))
	c.Data(http.StatusOK, output.ContentType, output.Bytes)
}

// ExportWebScanReportHandler 导出扫描报告
func ExportWebScanReportHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "")))
	var formatsParam []string
	if rawFormats := strings.TrimSpace(c.Query("formats")); rawFormats != "" {
		formatsParam = strings.Split(rawFormats, ",")
	}
	customFileName := strings.TrimSpace(c.DefaultQuery("fileName", ""))

	// 获取版本号（如果指定了 version 参数，使用指定版本；否则使用最新版本）
	versionStr := c.Param("version")
	var version int
	var task database.VulnScanTask

	if versionStr != "" {
		// 从路径参数获取版本号
		if v, err := strconv.Atoi(versionStr); err == nil && v > 0 {
			version = v
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "版本号无效",
			})
			return
		}
	} else {
		// 从查询参数获取版本号
		versionStr = c.DefaultQuery("version", "")
		if versionStr != "" {
			if v, err := strconv.Atoi(versionStr); err == nil && v > 0 {
				version = v
			} else {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "版本号无效",
				})
				return
			}
		} else {
			// 没有指定版本，获取最新版本
			if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{
					"success": false,
					"error":   "任务不存在",
				})
				return
			}
			version = task.Version
		}
	}

	// 获取任务信息
	if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务失败: " + err.Error(),
		})
		return
	}

	// 获取任务名称
	taskName := task.TaskName
	if taskName == "" {
		// 尝试从config中获取
		var config map[string]interface{}
		if task.Config != "" {
			json.Unmarshal([]byte(task.Config), &config)
			if name, ok := config["taskName"].(string); ok {
				taskName = name
			}
		}
		if taskName == "" {
			if len(taskID) >= 8 {
				taskName = fmt.Sprintf("任务 %s", taskID[:8])
			} else {
				taskName = "任务"
			}
		}
	}

	scanType := strings.ToLower(strings.TrimSpace(task.ScanType))
	formats, err := normalizeReportFormats(format, formatsParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	buildSingleReport := func(norm string) ([]byte, string, string, error) {
		switch scanType {
		case "webscan":
			switch norm {
			case "", "word":
				b, err := report.GenerateReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GenerateWebScanHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GenerateXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		case "crack":
			// crack 的结果写入 vulnerability_info；复用 webscan 报告生成器即可
			switch norm {
			case "", "word":
				b, err := report.GenerateReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GenerateWebScanHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GenerateXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		case "portscan":
			switch norm {
			case "", "word":
				b, err := report.GeneratePortScanWordReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GeneratePortScanHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GeneratePortScanXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		case "dirscan":
			switch norm {
			case "", "word":
				b, err := report.GenerateDirScanWordReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GenerateDirScanHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GenerateDirScanXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		case "jsscan":
			switch norm {
			case "", "word":
				b, err := report.GenerateJSScanWordReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GenerateJSScanHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GenerateJSScanXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		case "combination":
			// 组合任务：先做一个最小可用的报告（列出子任务）。后续可扩展为聚合导出。
			switch norm {
			case "", "word":
				b, err := report.GenerateCombinationWordReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GenerateCombinationHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GenerateCombinationXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		default:
			// 未知类型：尽量降级为 webscan 报告
			switch norm {
			case "", "word":
				b, err := report.GenerateReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
			case "html":
				b, err := report.GenerateHTMLReportFromDB(taskID, version, taskName)
				return b, "text/html; charset=utf-8", ".html", err
			case "xlsx":
				b, err := report.GenerateXLSXReportFromDB(taskID, version, taskName)
				return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
			default:
				return nil, "", "", fmt.Errorf("不支持的导出格式: %s", format)
			}
		}
	}

	// 生成文件名：优先使用用户自定义名（不含扩展名）；否则使用默认规则
	sanitizeFilenameBase := func(input string) string {
		base := strings.TrimSpace(input)
		if base == "" {
			return ""
		}
		// 去掉扩展名（避免用户传入 foo.docx 导致重复扩展名）
		base = strings.TrimSuffix(base, filepath.Ext(base))
		base = strings.TrimSpace(base)
		if base == "" {
			return ""
		}

		// Windows/常见文件系统非法字符
		replacer := strings.NewReplacer(
			"/", "_",
			"\\", "_",
			":", "_",
			"*", "_",
			"?", "_",
			"\"", "_",
			"<", "_",
			">", "_",
			"|", "_",
			"\n", "_",
			"\r", "_",
			"\t", "_",
		)
		base = replacer.Replace(base)
		base = strings.Trim(base, " .")

		// 限长（按 rune，避免截断 UTF-8）
		const maxLen = 120
		r := []rune(base)
		if len(r) > maxLen {
			base = string(r[:maxLen])
		}
		return base
	}

	fileBase := sanitizeFilenameBase(customFileName)
	if fileBase == "" {
		fileBase = fmt.Sprintf("漏洞扫描报告_%s_v%d_%s", strings.ReplaceAll(taskName, " ", "_"), version, time.Now().Format("20060102"))
	}

	files := make([]generatedReportFile, 0, len(formats))
	for _, norm := range formats {
		reportBytes, contentType, ext, err := buildSingleReport(norm)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "不支持的导出格式") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"success": false,
				"error":   "生成报告失败: " + err.Error(),
			})
			return
		}
		files = append(files, generatedReportFile{
			Name:        fileBase + ext,
			ContentType: contentType,
			Bytes:       reportBytes,
		})
	}

	output := files[0]
	if len(files) > 1 {
		output, err = packageReportsAsZip(fileBase, files)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "打包报告失败: " + err.Error(),
			})
			return
		}
	}

	// RFC 5987 + filename 双写，兼容浏览器对中文文件名的处理
	quoted := strings.ReplaceAll(output.Name, `"`, "_")
	escaped := url.PathEscape(output.Name)
	c.Header("Content-Type", output.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, quoted, escaped))
	c.Data(http.StatusOK, output.ContentType, output.Bytes)
}
