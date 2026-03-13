# 说在前面

目前工具仍处于内测阶段，通过License进行用户认证，需要可以联系本人，联系方式在最下面

## 快速开始

### 1️⃣ 使用 Docker 部署 PostgreSQL

系统依赖 PostgreSQL，如果没有请先通过 Docker 启动数据库服务：

```
docker run -d \
  --name my-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=Hephaestus@123. \
  -e POSTGRES_DB=hephaestus \
  -p 5432:5432 \
  -v pgdata:/var/lib/postgresql/data \
  postgres:15
```

说明：

- 数据库名称：`hephaestus`
- 用户名：`postgres`
- 密码：`Hephaestus@123.`
- 默认端口：`5432`
- 数据会持久化到 Docker volume：`pgdata`

> ⚠️ 请确保本地 5432 端口未被占用，或自行修改映射端口。

------

### 2️⃣ 启动程序

根据你的操作系统与 CPU 架构，运行对应的可执行文件即可：

```
./hephaestus-xx-xx
```

程序启动后：

- 会在**命令行输出当前运行状态**
- 首次启动或配置变更时，相关配置项也会在终端中提示

------

### 3️⃣ 修改服务端口（可选）

程序默认监听端口为 **18181**。
 如需修改端口，请编辑用户家目录下的配置文件：

```
~/.config/hephaestus/config.yaml
```

示例：

```
server:
  port: 18181
```

⚠️ 注意事项：

- **配置文件与可执行程序需位于同一用户环境**
- 修改端口后需重启程序生效
- 请确保端口未被占用

------

### 4️⃣ Linux 截图功能说明

在 Linux 环境下，如发现**截图功能不可用**，请自行确认系统是否已安装 Chromium 相关依赖。

常见原因包括：

- 未安装 `chromium` / `chromium-browser`
- 缺少必要的字体或图形依赖
- 运行环境为精简版 Server / 容器环境

> 建议优先在具备完整桌面依赖的 Linux 环境中使用截图功能。

## 联系方式

一定要备注好来意

<img src="assets/image-20231006124944803.png" alt="image-20231006124944803" style="width: 200px;" />
