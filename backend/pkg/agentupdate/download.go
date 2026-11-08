package agentupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var releaseVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$`)

const maxPackageBytes int64 = 512 << 20

func ServerTarget(ctx context.Context, server string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var text strings.Builder
	if err := fetch(ctx, strings.TrimRight(server, "/")+"/api/v1/agent/release?format=text", &text, 1024); err != nil {
		return "", err
	}
	v := strings.TrimSpace(text.String())
	if !releaseVersion.MatchString(v) {
		return "", fmt.Errorf("invalid server compatibility version")
	}
	return v, nil
}

func fetch(ctx context.Context, url string, writer io.Writer, max int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	n, err := io.Copy(writer, io.LimitReader(response.Body, max+1))
	if n > max {
		return fmt.Errorf("download exceeds size limit")
	}
	return err
}

// Stage extracts only the expected root executable and touches no configuration.
// Its directory lives beside the installed binary for same-volume atomic moves.
func Stage(ctx context.Context, binary, base, target, goos, arch string, progress func(int)) (string, error) {
	if !releaseVersion.MatchString(target) {
		return "", fmt.Errorf("invalid target version")
	}
	name := "hephaestus-agent-" + goos + "-" + arch + ".zip"
	app := "hephaestus-scan-agent"
	if goos == "windows" {
		app += ".exe"
	}
	dir, err := os.MkdirTemp(filepath.Dir(binary), ".agent-update-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	archive := filepath.Join(dir, "package.zip")
	f, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	count := &downloadProgress{callback: progress}
	err = fetch(ctx, strings.TrimRight(base, "/")+"/"+target+"/"+name, io.MultiWriter(f, hash, count), maxPackageBytes)
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	var checks strings.Builder
	if err = fetch(ctx, strings.TrimRight(base, "/")+"/"+target+"/SHA256SUMS.txt", &checks, 64<<10); err != nil {
		return "", err
	}
	var expected string
	matches := 0
	for _, line := range strings.Split(checks.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			expected = strings.ToLower(fields[0])
			matches++
		}
	}
	if matches != 1 || len(expected) != 64 || expected != hex.EncodeToString(hash.Sum(nil)) {
		return "", fmt.Errorf("package SHA-256 mismatch or missing checksum")
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer z.Close()
	var entry *zip.File
	for _, item := range z.File {
		if item.Name == app {
			if entry != nil {
				return "", fmt.Errorf("duplicate executable")
			}
			entry = item
		}
	}
	if entry == nil || !entry.Mode().IsRegular() || entry.UncompressedSize64 > uint64(maxPackageBytes) {
		return "", fmt.Errorf("invalid or missing executable")
	}
	r, err := entry.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	staged := filepath.Join(dir, app)
	out, err := os.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(out, io.LimitReader(r, maxPackageBytes+1))
	closeErr = out.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > maxPackageBytes {
		return "", fmt.Errorf("executable too large")
	}
	if err = VerifyBinary(ctx, staged, target); err != nil {
		return "", err
	}
	ok = true
	return staged, nil
}

type downloadProgress struct {
	bytes    int64
	callback func(int)
	last     time.Time
}

func (p *downloadProgress) Write(b []byte) (int, error) {
	p.bytes += int64(len(b))
	// Report downloaded MiB; the UI presents this as a byte count, not a guessed percentage.
	if p.callback != nil && time.Since(p.last) > time.Second {
		p.callback(int(p.bytes >> 20))
		p.last = time.Now()
	}
	return len(b), nil
}

func VerifyBinary(ctx context.Context, binary, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "Hephaestus scan-agent "+target+" (cli mode)" {
		return fmt.Errorf("executable version or platform does not match %s", target)
	}
	out, err = exec.CommandContext(ctx, binary, "help").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "supervise") {
		return fmt.Errorf("release does not support managed updates")
	}
	return nil
}

func Replace(binary, staged string) error {
	backup := binary + ".previous"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(binary, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, binary); err != nil {
		os.Rename(backup, binary)
		return err
	}
	return nil
}

func Restore(binary string) error {
	// The caller has already waited for the new child to fully exit.
	if _, err := os.Stat(binary + ".previous"); err != nil {
		return err
	}
	if err := os.Remove(binary); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(binary+".previous", binary)
}
