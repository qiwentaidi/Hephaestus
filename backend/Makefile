.PHONY: build run dev clean deps frontend-build install release-all release-windows release-darwin-arm64 release-darwin-amd64 release-linux

# 应用名称
APP_NAME = hephaestus

# 版本号
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u '+%Y-%m-%d %H:%M:%S')
LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X 'main.BuildTime=$(BUILD_TIME)'"

# 前端构建输出目录（嵌入用）
WEB_DIST = webassets/dist
FRONTEND_DIR = ../frontend
DIST_DIR = ../dist

# 构建项目（当前平台）
build: frontend-build
	go build $(LDFLAGS) -o bin/$(APP_NAME) main.go

# 运行项目
run: build
	./bin/$(APP_NAME)

# 开发模式（需要安装 air: go install github.com/cosmtrek/air@latest）
dev:
	air

# 清理构建文件
clean:
	rm -rf bin/ tmp/ dist/ $(WEB_DIST) $(DIST_DIR)

# 下载依赖
deps:
	go mod download
	go mod tidy

# 构建前端并拷贝到后端嵌入目录
frontend-build:
	cd $(FRONTEND_DIR) && STATIC_EXPORT=true npm run build
	rm -rf $(WEB_DIST)
	mkdir -p $(WEB_DIST)
	cp -R $(FRONTEND_DIR)/out/* $(WEB_DIST)/

# 安装开发工具
install:
	go install github.com/cosmtrek/air@latest

# ============ 多平台发布构建 ============

# 构建所有平台
release-all: frontend-build
	@mkdir -p $(DIST_DIR)
	@echo "构建 Windows AMD64..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe main.go
	@echo "构建 macOS ARM64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 main.go
	@echo "构建 macOS AMD64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 main.go
	@echo "构建 Linux AMD64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 main.go
	@echo "============================================"
	@echo "构建完成！产物位于: $(DIST_DIR)"
	@ls -lh $(DIST_DIR)

# 单独构建各平台
release-windows: frontend-build
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe main.go

release-darwin-arm64: frontend-build
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 main.go

release-darwin-amd64: frontend-build
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 main.go

release-linux: frontend-build
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 main.go

# 仅构建后端（跳过前端，用于前端已构建的情况）
release-backend-only:
	@mkdir -p $(DIST_DIR)
	@echo "构建所有平台（跳过前端构建）..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 main.go
	@ls -lh $(DIST_DIR)
