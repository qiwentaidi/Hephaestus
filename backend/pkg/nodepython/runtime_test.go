package nodepython

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveInterpreterOnly(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 required")
	}
	p, v, err := Resolve(context.Background(), Selection{Mode: "manual", Path: python})
	if err != nil || p == "" || v == "" {
		t.Fatalf("probe: %q %q %v", p, v, err)
	}
	// A symlink is a valid executable, even though it is not portable evidence.
	link := filepath.Join(t.TempDir(), "python")
	if err = os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Resolve(context.Background(), Selection{Mode: "manual", Path: link}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Resolve(context.Background(), Selection{Mode: "manual", Path: "python3"}); err == nil {
		t.Fatal("relative manual path accepted")
	}
	if _, _, err = Resolve(context.Background(), Selection{Mode: "manual", Path: "/missing-python"}); err == nil {
		t.Fatal("manual selection fell back")
	}
}

func TestDetectionDoesNotTestVenv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "python")
	// This probe stub has no venv support. Only the version query can succeed.
	body := "#!/bin/sh\ncase \"$*\" in *venv*|*ensurepip*) exit 99;; esac\nprintf '%s\\n' '{\"major\":3,\"version\":\"3.12.2\"}'\n"
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(context.Background(), Selection{Mode: "manual", Path: path}); err != nil {
		t.Fatal(err)
	}
}
