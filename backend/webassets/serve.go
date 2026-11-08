package webassets

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// indexHTML 缓存 index.html 内容
var indexHTML []byte
var subFS fs.FS

func Register(r *gin.Engine) error {
	var err error
	subFS, err = fs.Sub(distFS, "dist")
	if err != nil {
		return err
	}

	// 预加载 index.html
	indexHTML, err = fs.ReadFile(subFS, "index.html")
	if err != nil {
		return err
	}

	// 创建文件服务器
	fileServer := http.FileServer(http.FS(subFS))

	r.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path

		// API 不处理
		if strings.HasPrefix(urlPath, "/api") {
			c.Status(http.StatusNotFound)
			return
		}

		// 清理路径，移除末尾斜杠
		cleanPath := path.Clean(urlPath)
		cleanPath = strings.TrimSuffix(cleanPath, "/")

		if cleanPath == "" || cleanPath == "/" {
			// 首页直接返回 index.html
			serveHTML(c, indexHTML)
			return
		}

		cleanPath = strings.TrimPrefix(cleanPath, "/")

		// 1. 尝试精确匹配文件（用于静态资源 .js, .css, .png 等）
		if isFile(cleanPath) {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// 2. 尝试 path.html（Next.js 静态导出格式）
		htmlPath := cleanPath + ".html"
		if content, err := fs.ReadFile(subFS, htmlPath); err == nil {
			serveHTML(c, content)
			return
		}

		// 3. 尝试 path/index.html
		indexPath := cleanPath + "/index.html"
		if content, err := fs.ReadFile(subFS, indexPath); err == nil {
			serveHTML(c, content)
			return
		}

		// 4. SPA fallback: 返回 index.html
		serveHTML(c, indexHTML)
	})

	return nil
}

// isFile 检查路径是否是一个真实存在的文件（非目录）
func isFile(filePath string) bool {
	f, err := subFS.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// serveHTML 返回 HTML 内容
func serveHTML(c *gin.Context, content []byte) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write(content)
}
