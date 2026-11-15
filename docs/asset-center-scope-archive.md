# 资产中心作用域归档

## 背景

资产台账由 `unified_assets` / `unified_asset_observations` 驱动，与扫描任务解耦。删除来源任务不会删除资产记录，因此归档成为唯一的清理出口。部分台账行（根域、端口）由观察数据推导而来，没有独立资产行，旧实现无法归档它们。

## 模型

新增 `asset_scope_archives` 表（`backend/pkg/database/unified_asset.go`）：

| 字段 | 说明 |
| --- | --- |
| `scope_key` | 当前固定为 `global`，与统一资产目录一致 |
| `category` | `root` 或 `port` |
| `value` | 根域为根域名；端口为 `host:port/transport` |
| `archived_at` | 归档时间；恢复时删除该行 |

作用域归档只影响视图，不修改 `unified_assets` / `unified_asset_observations`，因此恢复无损，删除来源任务也不会清除归档状态。

## 语义

- 根域作用域：隐藏该根域、其子域、其端口和其网站。IP 资产保留，因为一个 IP 可能被多个根域共享。
- 端口作用域：只隐藏对应端口行，主机和其他端口保留。
- 归档期间新观察到的同根域资产保持隐藏，直到恢复根域。
- 恢复根域不会恢复此前单独归档的子域/网站；它们保留自己的归档状态。
- 根域作用域同时作用于台账、分类计数、导出、盘点/拓扑/地图/仪表盘投影。
- 端口作用域当前作用于台账、计数和导出；拓扑/地图中的端口仍作为主机属性展示。
- `显示已归档` 展示作用域归档行（合成行），作为恢复入口；不展开被隐藏的子树。

## 接口

沿用 `POST /api/v1/assets/ledger/entities/batch-archive`：

```json
{
  "action": "archive",
  "items": [
    { "id": "root:example.com", "category": "root", "value": "example.com", "scope": true },
    { "id": "port:192.0.2.10:443:tcp", "category": "port", "value": "192.0.2.10:443", "host": "192.0.2.10", "port": 443, "transport": "tcp", "scope": true }
  ]
}
```

`scope: true` 表示作用域归档；未带 `scope` 的 root/port 选择保持旧的资产级归档行为，兼容旧客户端。资产级归档仍使用 `assetId`。

## 前端

- 根域行操作：`归档根域` / `恢复根域`；归档时弹出确认框，显示将被隐藏的子域、端口、网站数量，并说明 IP 资产不受影响。
- 端口行操作：`归档端口` / `恢复端口`，即时执行，仅隐藏该端口。
- 批量归档包含根域时先弹确认框；否则沿用原有批量行为。
- 作用域归档行使用虚线边框的中性徽标（`根域归档` / `端口归档`），可恢复。
- 勾选“显示已归档”时显示说明文案。

## 关键实现

- `backend/handlers/asset/asset_scope_archive.go`：作用域索引、规范化、归档/恢复、台账过滤与合成行、聚合视图过滤。
- `backend/handlers/asset/asset_ledger_entities.go`：计算 `archiveSubdomainCount` / `archivePortCount` / `archiveWebsiteCount`，供确认框展示实际会被隐藏的数量。
- `backend/handlers/asset/asset_ledger_batch.go`：批量接口按 `scope` 分流作用域归档与资产归档。
- `backend/handlers/asset/asset_center_unified.go`、`asset_center_collect.go`：投影视图过滤。
- `frontend/app/(main)/asset-center/components/AssetLedgerView.tsx`：行操作、确认框、徽标、提示。
- `frontend/app/(main)/asset-center/components/AssetDetailSheet.tsx`：根域资产提示使用台账行归档整个根域。

## 测试

`backend/handlers/asset/asset_scope_archive_test.go` 覆盖：

- 根域作用域隐藏子树并保留 IP；
- 恢复后子树重新出现；
- 无独立资产行的根域合成归档行；
- 端口作用域只隐藏端口、不归档主机；
- 作用域接口选择与恢复；
- 聚合投影过滤；
- 归档期间新增观察仍隐藏，删除任务后归档保持。

## 根域概览

纯聚合根域（由子域、端口或网站推导、自身没有独立资产记录）以及作用域归档的根域行，点击后打开右侧“根域详情”面板：展示归档/状态徽标、子域/IP/端口/网站数量、来源、关联 IP、首次/最近发现，并提供“查看子域名”和“归档根域/恢复根域”。有独立资产记录的根域仍打开原有资产详情。
