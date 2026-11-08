package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManagedDictFile is a dictionary from either the updateable bundle or the
// user's persistent configuration. A custom file shadows a bundled file with
// the same source and name.
type ManagedDictFile struct {
	Name   string
	Path   string
	Custom bool
}

func GetCustomConfigResourceRoot() string {
	return filepath.Join(GetConfigBaseDir(), "custom-config")
}

func GetCustomDictDir(source string) string {
	return filepath.Join(GetCustomConfigResourceRoot(), source)
}

func managedDictDirs(source string) (bundled, custom string, err error) {
	switch source {
	case "crack", "dirsearch", "jwt":
		return filepath.Join(GetConfigResourceRoot(), source), GetCustomDictDir(source), nil
	default:
		return "", "", fmt.Errorf("unsupported dictionary source: %s", source)
	}
}

func ListManagedDictFiles(source string) ([]ManagedDictFile, error) {
	bundled, custom, err := managedDictDirs(source)
	if err != nil {
		return nil, err
	}

	files := make(map[string]ManagedDictFile)
	for _, dir := range []struct {
		path   string
		custom bool
	}{{bundled, false}, {custom, true}} {
		entries, err := os.ReadDir(dir.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
				continue
			}
			files[entry.Name()] = ManagedDictFile{
				Name: entry.Name(), Path: filepath.Join(dir.path, entry.Name()), Custom: dir.custom,
			}
		}
	}

	result := make([]ManagedDictFile, 0, len(files))
	for _, file := range files {
		result = append(result, file)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func ResolveManagedDictFile(source, name string) (ManagedDictFile, error) {
	if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") ||
		!strings.HasSuffix(strings.ToLower(name), ".txt") {
		return ManagedDictFile{}, fmt.Errorf("invalid dictionary name: %s", name)
	}
	bundled, custom, err := managedDictDirs(source)
	if err != nil {
		return ManagedDictFile{}, err
	}
	for _, dir := range []struct {
		path   string
		custom bool
	}{{custom, true}, {bundled, false}} {
		path := filepath.Join(dir.path, name)
		info, err := os.Lstat(path)
		if err == nil && info.Mode().IsRegular() {
			return ManagedDictFile{Name: name, Path: path, Custom: dir.custom}, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return ManagedDictFile{}, err
		}
	}
	return ManagedDictFile{}, os.ErrNotExist
}
