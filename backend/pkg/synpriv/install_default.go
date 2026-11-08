//go:build !darwin

package synpriv

import (
	"context"
	"errors"
	"path/filepath"

	"hephaestus-backend/pkg/config"
)

func HelperPath() string {
	return filepath.Join(config.GetConfigBaseDir(), "syn-helper")
}

func Install(ctx context.Context) (HelperStatus, error) {
	return Status(ctx), errors.New("SYN helper installation is only implemented on macOS")
}
