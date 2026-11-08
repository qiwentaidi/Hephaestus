package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

const desktopUpdateHelperFlag = "desktop-update-helper"

func buildDesktopUpdateHelperPath(execPath string) string {
	execDir := filepath.Dir(execPath)
	baseName := filepath.Base(execPath)
	ext := filepath.Ext(baseName)
	name := strings.TrimSuffix(baseName, ext)
	if ext == "" {
		return filepath.Join(execDir, "."+name+".update-helper")
	}
	return filepath.Join(execDir, "."+name+".update-helper"+ext)
}

func maybeRunDesktopUpdateHelper() bool {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	helperMode := fs.Bool(desktopUpdateHelperFlag, false, "commit prepared desktop update and relaunch")
	targetPath := fs.String("target", "", "desktop executable target path")
	cleanupPath := fs.String("cleanup", "", "helper executable path to delete after relaunch")
	waitMs := fs.Int("wait-ms", 1500, "delay before committing prepared update")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return false
	}
	if !*helperMode {
		return false
	}

	if err := runDesktopUpdateHelper(*targetPath, *cleanupPath, *waitMs); err != nil {
		fmt.Fprintf(os.Stderr, "desktop update helper failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
	return true
}

func runDesktopUpdateHelper(targetPath string, cleanupPath string, waitMs int) error {
	if targetPath == "" {
		return fmt.Errorf("missing update target path")
	}

	time.Sleep(time.Duration(waitMs) * time.Millisecond)

	if err := selfupdate.CommitBinary(selfupdate.Options{TargetPath: targetPath}); err != nil {
		if rollbackErr := selfupdate.RollbackError(err); rollbackErr != nil {
			return fmt.Errorf("commit failed: %w (rollback failed: %v)", err, rollbackErr)
		}
		return err
	}

	cmd := exec.Command("cmd", "/C", "start", "", targetPath)
	cmd.Dir = filepath.Dir(targetPath)
	prepareDesktopBackgroundCommand(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}

	scheduleDesktopHelperCleanup(targetPath, cleanupPath)
	return nil
}

func scheduleDesktopHelperCleanup(targetPath string, cleanupPath string) {
	oldBinaryPath := filepath.Join(
		filepath.Dir(targetPath),
		fmt.Sprintf(".%s.old", filepath.Base(targetPath)),
	)
	tempDir := filepath.Join(filepath.Dir(targetPath), ".update_temp")

	cmd := exec.Command(
		"cmd",
		"/C",
		fmt.Sprintf(
			`for /L %%%%I in (1,1,20) do (del /f /q "%s" >nul 2>&1 & if not exist "%s" goto helper_deleted & ping 127.0.0.1 -n 2 >nul) & :helper_deleted & for /L %%%%I in (1,1,20) do (del /f /q "%s" >nul 2>&1 & if not exist "%s" goto old_deleted & ping 127.0.0.1 -n 2 >nul) & :old_deleted & for /L %%%%I in (1,1,10) do (rmdir /s /q "%s" >nul 2>&1 & if not exist "%s" goto cleanup_done & ping 127.0.0.1 -n 2 >nul) & :cleanup_done`,
			cleanupPath,
			cleanupPath,
			oldBinaryPath,
			oldBinaryPath,
			tempDir,
			tempDir,
		),
	)
	prepareDesktopBackgroundCommand(cmd)
	_ = cmd.Start()
}

func cleanupDesktopUpdateArtifacts(execPath string) {
	if execPath == "" {
		return
	}

	helperPath := buildDesktopUpdateHelperPath(execPath)
	oldBinaryPath := filepath.Join(
		filepath.Dir(execPath),
		fmt.Sprintf(".%s.old", filepath.Base(execPath)),
	)
	tempDir := filepath.Join(filepath.Dir(execPath), ".update_temp")

	tryRemovePath(helperPath)
	tryRemovePath(oldBinaryPath)
	_ = os.RemoveAll(tempDir)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func tryRemovePath(path string) {
	for i := 0; i < 20; i++ {
		if err := os.Remove(path); err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
