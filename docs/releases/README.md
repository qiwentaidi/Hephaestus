# 版本更新日志

发布时 `v*` tag 会触发 `.github/workflows/release.yml`，其中 `publish` 作业按以下顺序取更新日志，写入官网 `client` 与 `desktop` 两条发布记录：

1. 附注 tag 的消息：`git tag -a v4.0.1 -F notes.md`
2. 本目录下的 `<version>.md`，例如 `docs/releases/v4.0.1.md`
3. 刚发布的 GitHub Release 正文（`generate_release_notes` 自动生成的变更列表）
4. 都没有时留空——更新已有记录时后端保留原日志，不会清空

## 建议格式

官网按 Markdown 渲染，沿用历史版本的三个小标题，用户一眼能看出改了什么：

```markdown
### 新增

- 一句话描述新增能力

### 调整

- 行为或默认值的变化

### 修复

- 修复的问题，必要时附 issue 链接
```

## 注意

- 文件名必须与 tag 完全一致（含 `v` 前缀），否则不会被读取。
- 只在需要脱离 git 历史撰写时用本目录；日常直接在附注 tag 里写更省事。
