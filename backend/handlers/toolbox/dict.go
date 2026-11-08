package toolbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/fsutil"
)

func crackDictDir() string {
	return filepath.Join(config.GetConfigResourceRoot(), "crack")
}

func dirsearchDictDir() string {
	return filepath.Join(config.GetConfigResourceRoot(), "dirsearch")
}

func jwtDictDir() string {
	return filepath.Join(config.GetConfigResourceRoot(), "jwt")
}

func customCrackDictDir() string     { return config.GetCustomDictDir("crack") }
func customDirsearchDictDir() string { return config.GetCustomDictDir("dirsearch") }
func customJWTDictDir() string       { return config.GetCustomDictDir("jwt") }

const dictMetaFileName = "_meta.json"

type managedDictSource struct {
	Key          string
	Dir          func() string
	CustomDir    func() string
	Ensure       func() error
	DefaultGroup string
	DefaultProto string
}

func ensureDirsearchDictDir() error {
	return os.MkdirAll(customDirsearchDictDir(), 0o755)
}

func ensureJWTDictDir() error {
	return os.MkdirAll(customJWTDictDir(), 0o755)
}

func getManagedDictSource(source string) (managedDictSource, error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "crack":
		return managedDictSource{
			Key:          "crack",
			Dir:          crackDictDir,
			CustomDir:    customCrackDictDir,
			Ensure:       ensureCrackDictDir,
			DefaultGroup: "crack",
			DefaultProto: "",
		}, nil
	case "dirsearch":
		return managedDictSource{
			Key:          "dirsearch",
			Dir:          dirsearchDictDir,
			CustomDir:    customDirsearchDictDir,
			Ensure:       ensureDirsearchDictDir,
			DefaultGroup: "dirsearch",
			DefaultProto: "Dirsearch",
		}, nil
	case "jwt":
		return managedDictSource{
			Key:          "jwt",
			Dir:          jwtDictDir,
			CustomDir:    customJWTDictDir,
			Ensure:       ensureJWTDictDir,
			DefaultGroup: "jwt",
			DefaultProto: "JWT",
		}, nil
	default:
		return managedDictSource{}, fmt.Errorf("不支持的字典来源: %s", source)
	}
}

// ensureCrackDictDir 只创建用户字典目录，不自动导入其他程序的字典。
func ensureCrackDictDir() error {
	if err := os.MkdirAll(customCrackDictDir(), 0o755); err != nil {
		return fmt.Errorf("创建字典目录失败: %w", err)
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := fsutil.CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// DictionaryInfo 前端使用的字典信息结构
type DictionaryInfo struct {
	Name         string `json:"name"`
	Source       string `json:"source"`
	Protocol     string `json:"protocol"`
	Group        string `json:"group"`
	Entries      int    `json:"entries"`
	Size         string `json:"size"`
	LastModified string `json:"lastModified"`
	Description  string `json:"description,omitempty"`
	Origin       string `json:"origin"`
}

type DictionaryMeta struct {
	Protocol    string `json:"protocol,omitempty"`
	Group       string `json:"group,omitempty"`
	Description string `json:"description,omitempty"`
}

// protocolKeyToLabel 将协议文件夹名（小写key）转换为显示标签
func protocolKeyToLabel(protocolKey string) string {
	switch protocolKey {
	case "ftp":
		return "FTP"
	case "mysql":
		return "MySQL"
	case "mssql":
		return "MSSQL"
	case "dameng":
		return "Dameng"
	case "smb":
		return "SMB"
	case "rdp":
		return "RDP"
	case "postgresql", "postgres":
		return "PostgreSQL"
	case "ssh":
		return "SSH"
	case "smtp":
		return "SMTP"
	case "mongodb":
		return "MongoDB"
	case "oracle":
		return "Oracle"
	case "ldap":
		return "LDAP"
	case "socks5":
		return "Socks5"
	case "mqtt":
		return "MQTT"
	case "rabbitmq":
		return "RabbitMQ"
	case "vnc":
		return "VNC"
	case "telnet":
		return "Telnet"
	case "activemq":
		return "ActiveMQ"
	case "kafka":
		return "Kafka"
	case "clickhouse":
		return "ClickHouse"
	case "rsync":
		return "Rsync"
	case "java-rmi":
		return "Java-RMI"
	case "jdwp":
		return "JDWP"
	case "adb":
		return "ADB"
	case "memcached":
		return "Memcached"
	case "zookeeper":
		return "ZooKeeper"
	default:
		// 首字母大写
		if len(protocolKey) > 0 {
			return strings.ToUpper(protocolKey[:1]) + protocolKey[1:]
		}
		return protocolKey
	}
}

// inferProtocolAndType 根据文件名简单推断协议与字典类型（用于旧格式兼容）
func inferProtocolAndType(filename string) (protocol string, dictType string) {
	name := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))

	// 类型推断
	dictType = "password"
	if strings.Contains(name, "user") || strings.Contains(name, "username") {
		dictType = "username"
	}

	// 协议推断：取第一个分隔符之前的部分作为协议 key
	protocolKey := name
	for _, sep := range []string{"_", "-", "."} {
		if idx := strings.Index(protocolKey, sep); idx > 0 {
			protocolKey = protocolKey[:idx]
			break
		}
	}

	return protocolKeyToLabel(protocolKey), dictType
}

func inferDictionaryGroup(filename string, protocol string, source string) string {
	if strings.EqualFold(source, "dirsearch") {
		return "dirsearch"
	}
	name := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))
	if strings.Contains(name, "jwt") || strings.EqualFold(protocol, "JWT") {
		return "jwt"
	}
	return "crack"
}

func normalizeDictionaryGroup(group string, filename string, protocol string, source string) string {
	if trimmed := strings.TrimSpace(group); trimmed != "" {
		return trimmed
	}
	return inferDictionaryGroup(filename, protocol, source)
}

func dictMetaPath(dir string) string {
	return filepath.Join(dir, dictMetaFileName)
}

func loadDictMeta(dir string) (map[string]DictionaryMeta, error) {
	metaPath := dictMetaPath(dir)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]DictionaryMeta{}, nil
		}
		return nil, err
	}

	var meta map[string]DictionaryMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta == nil {
		meta = map[string]DictionaryMeta{}
	}
	return meta, nil
}

func saveDictMeta(source managedDictSource, meta map[string]DictionaryMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dictMetaPath(source.CustomDir()), data, 0o644)
}

func inferDictionaryProtocol(filename string, source managedDictSource, meta DictionaryMeta) string {
	if protocol := strings.TrimSpace(meta.Protocol); protocol != "" {
		return protocol
	}
	if source.DefaultProto != "" {
		return source.DefaultProto
	}
	protocol, _ := inferProtocolAndType(filename)
	return protocol
}

func resolveManagedDictPath(source managedDictSource, name string) (string, string, error) {
	if err := ValidateDictFileName(name); err != nil {
		return "", "", err
	}

	dictDir := source.CustomDir()
	path := filepath.Clean(filepath.Join(dictDir, name))
	absDictDir, err := filepath.Abs(dictDir)
	if err != nil {
		return "", "", fmt.Errorf("获取字典目录绝对路径失败: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("获取文件绝对路径失败: %w", err)
	}
	if !strings.HasPrefix(absPath, absDictDir) {
		return "", "", fmt.Errorf("非法的字典名称：路径不在允许的目录内")
	}
	return path, absDictDir, nil
}

func listManagedDicts(source managedDictSource, groupFilter string, protocolFilter string, searchQuery string) ([]DictionaryInfo, error) {
	if err := source.Ensure(); err != nil {
		return nil, err
	}

	bundledMeta, err := loadDictMeta(source.Dir())
	if err != nil {
		return nil, err
	}
	customMeta, err := loadDictMeta(source.CustomDir())
	if err != nil {
		return nil, err
	}
	entries, err := config.ListManagedDictFiles(source.Key)
	if err != nil {
		return nil, err
	}

	dicts := make([]DictionaryInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := os.Stat(entry.Path)
		if err != nil {
			continue
		}

		meta := bundledMeta[entry.Name]
		origin := "bundled"
		if entry.Custom {
			meta = customMeta[entry.Name]
			origin = "custom"
		}
		protocolLabel := inferDictionaryProtocol(entry.Name, source, meta)
		group := normalizeDictionaryGroup(meta.Group, entry.Name, protocolLabel, source.Key)
		description := strings.TrimSpace(meta.Description)

		if groupFilter != "" && !strings.EqualFold(group, groupFilter) {
			continue
		}
		if protocolFilter != "" && !strings.EqualFold(protocolLabel, protocolFilter) {
			continue
		}
		if searchQuery != "" {
			searchTarget := strings.ToLower(strings.Join([]string{
				entry.Name,
				source.Key,
				protocolLabel,
				group,
				description,
			}, " "))
			if !strings.Contains(searchTarget, searchQuery) {
				continue
			}
		}

		dicts = append(dicts, DictionaryInfo{
			Name:         entry.Name,
			Source:       source.Key,
			Protocol:     protocolLabel,
			Group:        group,
			Entries:      countFileLines(entry.Path),
			Size:         humanReadableSize(info.Size()),
			LastModified: info.ModTime().Format("2006-01-02"),
			Description:  description,
			Origin:       origin,
		})
	}

	return dicts, nil
}

// humanReadableSize 将字节数转换为人类可读格式
func humanReadableSize(size int64) string {
	const kb = 1024
	const mb = kb * 1024
	const gb = mb * 1024

	switch {
	case size >= gb:
		return fmt.Sprintf("%.1f GB", float64(size)/float64(gb))
	case size >= mb:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(mb))
	case size >= kb:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(kb))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// countFileLines 统计文件行数
func countFileLines(filePath string) int {
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	var count int
	buf := make([]byte, 32*1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			count += strings.Count(string(buf[:n]), "\n")
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}
	}
	return count
}

// ListCrackDictHandler 列出爆破字典文件信息
// 支持键值对格式：每个协议一个 .txt 文件，内容为 username:password 格式
func ListCrackDictHandler(c *gin.Context) {
	sourceFilter := strings.TrimSpace(c.Query("source"))
	groupFilter := strings.TrimSpace(c.Query("group"))
	protocolFilter := strings.TrimSpace(c.Query("protocol"))
	searchQuery := strings.ToLower(strings.TrimSpace(c.Query("q")))

	sources := []string{"crack", "dirsearch", "jwt"}
	if sourceFilter != "" && !strings.EqualFold(sourceFilter, "all") {
		sources = []string{sourceFilter}
	}

	var dicts []DictionaryInfo
	for _, sourceKey := range sources {
		source, err := getManagedDictSource(sourceKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		items, err := listManagedDicts(source, groupFilter, protocolFilter, searchQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "读取字典目录失败: " + err.Error(),
			})
			return
		}
		dicts = append(dicts, items...)
	}

	sort.Slice(dicts, func(i, j int) bool {
		if dicts[i].Source != dicts[j].Source {
			return dicts[i].Source < dicts[j].Source
		}
		if dicts[i].Group != dicts[j].Group {
			return dicts[i].Group < dicts[j].Group
		}
		return dicts[i].Name < dicts[j].Name
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"dicts": dicts,
		},
	})
}

// ValidateDictFileName 验证字典文件名是否安全（防止路径遍历）
func ValidateDictFileName(name string) error {
	// 禁止路径遍历符号和路径分隔符
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("非法的字典名称：包含路径分隔符或路径遍历符号")
	}
	// 禁止空文件名
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("字典名称不能为空")
	}
	// 必须是以 .txt 结尾的文件
	if !strings.HasSuffix(strings.ToLower(name), ".txt") {
		return fmt.Errorf("字典文件必须是 .txt 格式")
	}
	return nil
}

// GetCrackDictContentHandler 获取指定字典文件内容
// 格式：name=protocol.txt（如 rdp.txt），内容为键值对格式 username:password
func GetCrackDictContentHandler(c *gin.Context) {
	source, err := getManagedDictSource(c.DefaultQuery("source", "crack"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少字典名称参数 name",
		})
		return
	}

	// 安全检查：防止路径遍历攻击
	if err := ValidateDictFileName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := source.Ensure(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化字典目录失败: " + err.Error(),
		})
		return
	}

	file, err := config.ResolveManagedDictFile(source.Key, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "字典文件不存在",
		})
		return
	}

	content, err := os.ReadFile(file.Path)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "字典文件不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取字典文件失败: " + err.Error(),
		})
		return
	}

	metaDir := source.Dir()
	if file.Custom {
		metaDir = source.CustomDir()
	}
	metaMap, err := loadDictMeta(metaDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取字典元数据失败: " + err.Error(),
		})
		return
	}
	meta := metaMap[name]
	protocol := inferDictionaryProtocol(name, source, meta)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"name":        name,
			"source":      source.Key,
			"content":     string(content),
			"group":       normalizeDictionaryGroup(meta.Group, name, protocol, source.Key),
			"protocol":    protocol,
			"description": meta.Description,
			"origin":      map[bool]string{true: "custom", false: "bundled"}[file.Custom],
			"updatedAt":   time.Now().Format(time.RFC3339),
		},
	})
}

// SaveCrackDictContentRequest 保存字典内容请求体
type SaveCrackDictContentRequest struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Content     string `json:"content" binding:"required"`
	Protocol    string `json:"protocol"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// SaveCrackDictContentHandler 保存（新建/覆盖）字典文件内容
// 键值对格式：根据协议保存到 protocol.txt（如 rdp.txt），内容为 username:password 格式
func SaveCrackDictContentHandler(c *gin.Context) {
	var req SaveCrackDictContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	protocol := strings.TrimSpace(req.Protocol)
	name := strings.TrimSpace(req.Name)
	source, err := getManagedDictSource(req.Source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := source.Ensure(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化字典目录失败: " + err.Error(),
		})
		return
	}

	var path string

	if name != "" {
		// 如果提供了 name，直接使用（编辑场景）
		if !strings.HasSuffix(strings.ToLower(name), ".txt") {
			name = name + ".txt"
		}
		// 安全检查：防止路径遍历攻击
		if err := ValidateDictFileName(name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		path, _, err = resolveManagedDictPath(source, name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	} else if protocol != "" {
		if source.Key != "crack" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "当前字典来源必须显式提供字典名称",
			})
			return
		}
		// 新建场景：根据协议生成文件名
		protocolKey := strings.ToLower(protocol)
		// 映射协议显示名到文件名
		switch protocolKey {
		case "mssql":
			protocolKey = "mssql"
		case "postgresql":
			protocolKey = "postgresql"
		case "socks5":
			protocolKey = "socks5"
		case "activemq":
			protocolKey = "activemq"
		case "java-rmi":
			protocolKey = "java-rmi"
		}
		name = protocolKey + ".txt"
		path, _, err = resolveManagedDictPath(source, name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "需要提供协议或字典名称",
		})
		return
	}
	create := c.Query("create") == "true"
	if create {
		if source.Key == "crack" && protocol == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "爆破字典需要填写协议"})
			return
		}
		if _, err := config.ResolveManagedDictFile(source.Key, name); err == nil {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "字典文件已存在: " + name})
			return
		} else if !os.IsNotExist(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
	}

	if create {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "字典文件已存在: " + name})
			return
		}
		if err == nil {
			_, err = file.WriteString(req.Content)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		}
	} else {
		err = os.WriteFile(path, []byte(req.Content), 0o644)
	}
	if err != nil {
		if create {
			_ = os.Remove(path)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "写入字典文件失败: " + err.Error(),
		})
		return
	}

	metaMap, err := loadDictMeta(source.CustomDir())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取字典元数据失败: " + err.Error(),
		})
		return
	}

	existingMeta := metaMap[name]
	if existingMeta == (DictionaryMeta{}) {
		if bundledMeta, metaErr := loadDictMeta(source.Dir()); metaErr == nil {
			existingMeta = bundledMeta[name]
		}
	}
	protocolLabel := strings.TrimSpace(req.Protocol)
	if protocolLabel == "" {
		protocolLabel = inferDictionaryProtocol(name, source, existingMeta)
	}

	metaMap[name] = DictionaryMeta{
		Protocol:    protocolLabel,
		Group:       normalizeDictionaryGroup(req.Group, name, protocolLabel, source.Key),
		Description: strings.TrimSpace(req.Description),
	}

	if err := saveDictMeta(source, metaMap); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存字典元数据失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "字典已保存",
	})
}

// DeleteCrackDictContentHandler 删除字典文件
func DeleteCrackDictContentHandler(c *gin.Context) {
	source, err := getManagedDictSource(c.DefaultQuery("source", "crack"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少字典名称参数 name",
		})
		return
	}

	// 安全检查：防止路径遍历攻击
	if err := ValidateDictFileName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := source.Ensure(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化字典目录失败: " + err.Error(),
		})
		return
	}

	file, err := config.ResolveManagedDictFile(source.Key, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "字典文件不存在",
		})
		return
	}
	if !file.Custom {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "内置字典不可删除"})
		return
	}

	if err := os.Remove(file.Path); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "字典文件不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除字典文件失败: " + err.Error(),
		})
		return
	}

	metaMap, err := loadDictMeta(source.CustomDir())
	if err == nil {
		delete(metaMap, name)
		_ = saveDictMeta(source, metaMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "字典已删除",
	})
}

// GetCrackDictByProtocol 根据协议获取可用的字典文件路径
// 用于暴破功能中匹配字典
func GetCrackDictByProtocol(protocol string) (string, error) {
	protocolKey := strings.ToLower(protocol)

	// 确保目录存在
	if err := ensureCrackDictDir(); err != nil {
		return "", err
	}

	entries, err := config.ListManagedDictFiles("crack")
	if err != nil {
		return "", err
	}

	// 查找文件名包含协议名的字典文件
	for _, entry := range entries {
		// 检查文件名是否包含协议名
		fileName := strings.ToLower(strings.TrimSuffix(entry.Name, ".txt"))
		if strings.Contains(fileName, protocolKey) || strings.Contains(protocolKey, fileName) {
			return entry.Path, nil
		}
	}

	return "", fmt.Errorf("未找到协议 %s 的字典", protocol)
}
