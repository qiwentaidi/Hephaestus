# hephaestus-migrate

离线数据库迁移工具，用于修复桌面客户端启动时报：

```
自动迁移表结构失败: 旧子域名任务 <id> 与统一任务类型 combination 冲突
```

这类失败是旧版组合任务遗留的同 ID 子域名任务记录导致的。工具执行的迁移逻辑与
客户端启动时完全一致（`database.AutoMigrate`），但包含针对该历史数据的兼容处理，
因此可以在**不发布新客户端版本**的情况下修复本地数据库。

## 行为

1. 默认读取桌面端配置 `%USERPROFILE%\.config\hephaestus\config-desktop.yaml` 中的 SQLite 路径；
   也可以使用 `-db` 指定数据库文件。
2. 校验目标确实是 Hephaestus 数据库（存在 `vul_scan_task`）。
3. 统计旧子域名任务数量以及会导致启动失败的冲突数量。
4. 迁移前自动备份（`<数据库>.before-migrate-<时间戳>`）。
5. 执行迁移并校验旧表已清理。
6. **不会删除** `subdomain_scan_result` 中的历史结果。

## 使用

```bat
hephaestus-migrate.exe                 :: 自动定位数据库并迁移
hephaestus-migrate.exe -dry-run        :: 只检查，不改数据
hephaestus-migrate.exe -db D:\data\hephaestus.db
hephaestus-migrate.exe -yes -no-backup :: 无人值守（不建议跳过备份）
```

运行前必须完全退出 Hephaestus 客户端（包括托盘图标和后台进程）。

## 构建

```bash
# 本机
go build -o bin/hephaestus-migrate ./cmd/hephaestus-migrate

# Windows AMD64（需要 mingw-w64；SQLite 驱动依赖 CGO）
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -trimpath -ldflags "-s -w -extldflags=-static" \
  -o ../dist/hephaestus-migrate-windows-amd64.exe ./cmd/hephaestus-migrate
```

## 测试

```bash
go test ./cmd/hephaestus-migrate/
```
