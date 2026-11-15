package resource

import (
	"path/filepath"
	"runtime"
)

// ProjectBase selects the standard pipeline publication root for an application.
// Native project paths are relative to the process working directory (or absolute).
// Browser exports place prepared public resources beside the application page,
// including when that page is served below an authenticated proxy prefix.
// This selects a path once; it does not probe files, fetch or transform assets.
func ProjectBase(project string) string {
	if runtime.GOOS == "js" {
		return "./"
	}

	return filepath.Join(project, "generated", "public")
}
