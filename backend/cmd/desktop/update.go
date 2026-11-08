package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func latestDesktopUpdateDMGPath() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("desktop dmg install is only supported on macOS")
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(execPath), ".update_temp", "desktop-update.dmg")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("未找到已下载的 dmg 安装包")
		}
		return "", err
	}
	return path, nil
}

func installDesktopDMGAndRelaunch() error {
	dmgPath, err := latestDesktopUpdateDMGPath()
	if err != nil {
		return err
	}

	volumePath, err := attachDMGVolume(dmgPath)
	if err != nil {
		return err
	}
	defer detachDMGVolume(volumePath)

	appPath, err := locateMountedAppBundle(volumePath)
	if err != nil {
		return err
	}

	targetPath := filepath.Join("/Applications", filepath.Base(appPath))
	if err := copyAppBundle(appPath, targetPath); err != nil {
		return err
	}
	if err := clearAppQuarantine(targetPath); err != nil {
		return err
	}
	if err := adHocSignApp(targetPath); err != nil {
		return err
	}
	registerAppWithGatekeeper(targetPath)
	if err := openInstalledApp(targetPath); err != nil {
		return err
	}
	return nil
}

func attachDMGVolume(dmgPath string) (string, error) {
	cmd := exec.Command("hdiutil", "attach", "-nobrowse", dmgPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("挂载 dmg 失败: %s", strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "/Volumes/"); idx >= 0 {
			return strings.TrimSpace(line[idx:]), nil
		}
	}
	return "", fmt.Errorf("未找到 dmg 挂载卷")
}

func detachDMGVolume(volumePath string) {
	if strings.TrimSpace(volumePath) == "" {
		return
	}
	cmd := exec.Command("hdiutil", "detach", volumePath)
	prepareDesktopBackgroundCommand(cmd)
	_ = cmd.Run()
}

func locateMountedAppBundle(volumePath string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(volumePath, "*.app"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("挂载卷中未找到 .app")
	}
	return matches[0], nil
}

func copyAppBundle(srcPath string, dstPath string) error {
	_ = os.RemoveAll(dstPath)
	cmd := exec.Command("ditto", srcPath, dstPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("复制应用到 /Applications 失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func clearAppQuarantine(appPath string) error {
	cmd := exec.Command("xattr", "-cr", appPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("移除隔离属性失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func adHocSignApp(appPath string) error {
	cmd := exec.Command("codesign", "--force", "--deep", "--sign", "-", appPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ad-hoc 签名失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func registerAppWithGatekeeper(appPath string) {
	cmd := exec.Command("spctl", "--add", "--label", "Hephaestus", appPath)
	prepareDesktopBackgroundCommand(cmd)
	_ = cmd.Run()
}

func openInstalledApp(appPath string) error {
	cmd := exec.Command("open", "-n", appPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("启动新应用失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (d *DesktopService) launchInstalledUpdateAndQuit() {
	go func() {
		if err := installDesktopDMGAndRelaunch(); err != nil {
			return
		}
		time.Sleep(1200 * time.Millisecond)
		if window := d.getWindow(); window != nil {
			window.Close()
		}
	}()
}
