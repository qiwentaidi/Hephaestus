package webscan

import (
	"sync"

	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/runtimeenv"
)

var snapChromeScreenshotNoticeOnce sync.Once

func warnIfLinuxSnapChromeScreenshot() {
	chromePath, ok := runtimeenv.DetectSnapChromeExecutable()
	if !ok {
		return
	}

	snapChromeScreenshotNoticeOnce.Do(func() {
		logger.Warning("检测到当前 Linux 环境的 Chrome/Chromium 可能来自 snap 安装: %s。snap 沙箱可能导致 chromedp 截图长期运行后在 /tmp/snap-private-tmp/snap.chromium 残留大量临时文件，建议配置定时清理该目录，或改用非 snap 方式安装 Chrome/Chromium。", chromePath)
	})
}
