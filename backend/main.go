//go:build !dev
// +build !dev

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appcore "hephaestus-backend/internal/app"
	mcpserver "hephaestus-backend/mcp"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/synpriv"
)

func main() {
	if synpriv.MaybeRunHelper() {
		return
	}
	if synpriv.DenyRegularHelperStartup() {
		log.Println("SYN helper can only be used through the discovery helper interface")
		return
	}

	config.SetRuntimeMode(config.RuntimeModeServer)

	app := appcore.New()
	if err := app.Bootstrap(); err != nil {
		log.Fatal(appcore.FormatBootstrapError(err))
	}

	port := config.GetWebPort()
	if port == 0 {
		port = 18181
	}

	router := app.BuildRouter(true)
	// 恢复持久化的 MCP 专用监听配置，失败仅告警不影响主服务启动
	_ = mcpserver.RestoreMCPListener(router, port)
	srv := app.NewHTTPServer(fmt.Sprintf(":%d", port), router)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("初始化完成，正在启动 HTTP 服务（端口: %d）", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server:", err)
		}
	}()

	<-quit
	log.Println("收到退出信号，正在优雅关闭服务器...")

	go func() {
		time.Sleep(10 * time.Second)
		log.Println("⚠️  优雅关闭超时，强制退出进程")
		os.Exit(1)
	}()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()

	if err := app.Shutdown(shutdownCtx); err != nil && err != context.DeadlineExceeded {
		logger.Warning("应用关闭出现异常: %v", err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warning("服务器关闭超时，强制退出: %v", err)
		os.Exit(1)
	}
}
