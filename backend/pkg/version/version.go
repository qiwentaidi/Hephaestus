package version

// Version Web 服务端、Agent 节点与桌面客户端共用的发布版本号。
//
// 从 v4.0.0 起 Web 与 Desktop 使用同一个版本号：同一次发布（一个 tag）同时
// 产出 Web 与桌面端全部产物，因此两者必须同步递增。桌面端历史版本号是
// 3.x，已安装的客户端用字符串拆段比较版本，所以版本号只能单调递增，不能
// 为了统一而回退到 0.x。
const Version = "v4.0.1"

// DesktopVersion 桌面客户端版本号，必须与 Version 保持一致。
//
// 保留独立常量是因为 desktop-version.mjs 直接从本文件解析字面量来生成
// Info.plist / Windows 版本资源；改成 Version 的别名会破坏该解析。
const DesktopVersion = "v4.0.1"
