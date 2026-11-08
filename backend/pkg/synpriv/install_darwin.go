//go:build darwin

package synpriv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Keep the helper's Contents/MacOS and Contents/Frameworks layout intact.
// The desktop binary links libpcap through @executable_path/../Frameworks;
// copying only the executable to PrivilegedHelperTools makes dyld terminate
// it before the helper can answer its self-test.
const darwinHelperPath = "/Library/PrivilegedHelperTools/com.hephaestus.syn-helper/Contents/MacOS/com.hephaestus.syn-helper"

func HelperPath() string {
	return darwinHelperPath
}

func Install(ctx context.Context) (HelperStatus, error) {
	executable, err := os.Executable()
	if err != nil {
		return Status(ctx), fmt.Errorf("resolve current executable: %w", err)
	}

	script := strings.Join([]string{
		"set -e",
		"src=" + shellQuote(executable),
		"dst=" + shellQuote(HelperPath()),
		`helper_dir="$(/usr/bin/dirname "$dst")"`,
		`legacy_path="$(/usr/bin/dirname "$helper_dir")"`,
		`frameworks_dir="$(/usr/bin/dirname "$helper_dir")/Frameworks"`,
		`src_lib="$(/usr/bin/dirname "$src")/../Frameworks/libpcap.A.dylib"`,
		`tmp="${dst}.tmp.$$"`,
		`tmp_lib="${frameworks_dir}/libpcap.A.dylib.tmp.$$"`,
		`if [ -f "$legacy_path" ] && [ ! -d "$legacy_path" ]; then /bin/rm -f "$legacy_path"; fi`,
		`/bin/mkdir -p "$helper_dir" "$frameworks_dir"`,
		`trap '/bin/rm -f "$tmp" "$tmp_lib"' EXIT`,
		`/bin/cp "$src" "$tmp"`,
		`/usr/sbin/chown root:wheel "$tmp"`,
		`/bin/chmod 4755 "$tmp"`,
		`/bin/mv "$tmp" "$dst"`,
		`/usr/sbin/chown root:wheel "$dst"`,
		`/bin/chmod 4755 "$dst"`,
		`if [ -f "$src_lib" ]; then`,
		`  /bin/cp -L "$src_lib" "$tmp_lib"`,
		`  /usr/sbin/chown root:wheel "$tmp_lib"`,
		`  /bin/chmod 0644 "$tmp_lib"`,
		`  /bin/mv "$tmp_lib" "$frameworks_dir/libpcap.A.dylib"`,
		`  /usr/sbin/chown root:wheel "$frameworks_dir/libpcap.A.dylib"`,
		`  /bin/chmod 0644 "$frameworks_dir/libpcap.A.dylib"`,
		`fi`,
	}, "\n")

	appleScript := "do shell script " + strconv.Quote(script) + " with administrator privileges"
	cmd := exec.CommandContext(ctx, "osascript", "-e", appleScript)
	output, err := cmd.CombinedOutput()
	status := Status(ctx)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return status, fmt.Errorf("install SYN helper: %s", message)
	}
	status = Status(ctx)
	if !status.Usable {
		if status.Error != "" {
			return status, fmt.Errorf("installed SYN helper is not usable: %s", status.Error)
		}
		return status, fmt.Errorf("installed SYN helper is not usable")
	}
	return status, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
