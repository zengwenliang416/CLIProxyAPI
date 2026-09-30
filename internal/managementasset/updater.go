package managementasset

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
)

const ManagementFileName = "management.html"

// SetCurrentConfig is retained for SDK compatibility; the panel is a local
// build artifact and does not depend on runtime configuration.
func SetCurrentConfig(_ *config.Config) {}

// StartAutoUpdater never starts a network updater in this distribution.
func StartAutoUpdater(_ context.Context, _ string) {}

func StaticDir(configFilePath string) string {
	if override := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); override != "" {
		cleaned := filepath.Clean(override)
		if strings.EqualFold(filepath.Base(cleaned), ManagementFileName) {
			return filepath.Dir(cleaned)
		}
		return cleaned
	}
	if writable := util.WritablePath(); writable != "" {
		return filepath.Join(writable, "static")
	}
	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return ""
	}
	base := filepath.Dir(configFilePath)
	if info, err := os.Stat(configFilePath); err == nil && info.IsDir() {
		base = configFilePath
	}
	return filepath.Join(base, "static")
}

func FilePath(configFilePath string) string {
	dir := StaticDir(configFilePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, ManagementFileName)
}

// EnsureLatestManagementHTML only reports a local file's presence. Missing
// assets fail closed instead of downloading an unpinned replacement.
func EnsureLatestManagementHTML(_ context.Context, staticDir, _, _ string) bool {
	if strings.TrimSpace(staticDir) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(staticDir, ManagementFileName))
	return err == nil && info.Mode().IsRegular()
}
