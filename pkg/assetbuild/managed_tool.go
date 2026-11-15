//go:build !js

package assetbuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
)

// ManagedToolSpec pins a complete native build-tool archive managed by mise's
// HTTP backend. StripComponents removes leading archive directories; Executable
// and Components are paths within the resulting installation. Components name
// the real loadable library files, preserving any upstream symlink layout.
type ManagedToolSpec struct {
	Name, Version, URL, SHA256, Format, Executable string
	Components                                     []string
	StripComponents                                int
}

// ManagedTool is a mise-managed build-tool installation, never game data.
type ManagedTool struct {
	Executable string   `json:"executable"`
	Components []string `json:"components"`
}

const (
	managedZip           = "zip"
	managedTarGzip       = "tar.gz"
	managedTarBzip2      = "tar.bz2"
	managedIdentityLimit = 128
	managedPathLimit     = 4096
	managedPathDepth     = 64
	managedSelectedLimit = 64
)

var errManagedTool = errors.New("invalid or unavailable mise-managed asset tool")

// ManagedTool asks required mise >=2026.11.5 to install the pinned HTTP artifact,
// then tracks mise, its installation environment, the executable and components.
// Only build actions may install tools; game runtimes never invoke this API.
func (ctx *Context) ManagedTool(spec ManagedToolSpec) (ManagedTool, error) {
	return ctx.managedInstallation(spec, false)
}

// ManagedLibrary installs an upstream library archive through the same pinned
// mise provider as ManagedTool. Executable names its primary library file;
// components are tracked without requiring executable permission.
func (ctx *Context) ManagedLibrary(spec ManagedToolSpec) (string, error) {
	installed, err := ctx.managedInstallation(spec, true)

	return installed.Executable, err
}

func (ctx *Context) managedInstallation(spec ManagedToolSpec, library bool) (ManagedTool, error) {
	if ctx.expanding {
		return ManagedTool{}, ctx.discoveryError()
	}

	if err := validateManagedTool(spec); err != nil {
		return ManagedTool{}, err
	}

	mise, err := ctx.TrackTool("mise")
	if err != nil {
		return ManagedTool{}, fmt.Errorf("asset preparation requires mise >=2026.11.5 on PATH: %w", err)
	}

	environment := managedMiseEnvironment(ctx)

	installed, err := ctx.runner.managedToolSnapshot(ctx, spec, mise, environment)
	if err != nil {
		return ManagedTool{}, err
	}

	track := ctx.TrackTool
	if library {
		track = ctx.TrackLibrary
	}

	installed.Executable, err = track(installed.Executable)
	if err != nil {
		return ManagedTool{}, err
	}

	for _, component := range installed.Components {
		if _, err = track(component); err != nil {
			return ManagedTool{}, err
		}
	}

	return installed, nil
}

func managedMiseEnvironment(ctx *Context) []string {
	names := strings.Fields("MISE_DATA_DIR MISE_CACHE_DIR MISE_CONFIG_DIR MISE_STATE_DIR " +
		"XDG_DATA_HOME XDG_CACHE_HOME XDG_CONFIG_HOME XDG_STATE_HOME HOME USERPROFILE LOCALAPPDATA APPDATA")
	values := make([]string, 0, len(names))

	for _, name := range names {
		values = append(values, name+"="+ctx.Getenv(name))
	}

	return values
}

func (run *runner) managedToolSnapshot(ctx context.Context, spec ManagedToolSpec, mise string, environment []string) (ManagedTool, error) {
	if run.snapshotMu != nil {
		run.snapshotMu.Lock()
		defer run.snapshotMu.Unlock()
	}

	identity, err := jsonDigest([]any{spec, mise, environment})
	if err != nil {
		return ManagedTool{}, err
	}

	key := "\x00mise-tool:" + identity
	if snapshot, exists := run.files[key]; exists {
		var installed ManagedTool
		if snapshot.err != nil {
			return installed, snapshot.err
		}

		err = json.Unmarshal(snapshot.data, &installed)

		return installed, err
	}

	installed, err := installMiseTool(ctx, run.project, spec, mise)

	data, marshalErr := json.Marshal(installed)
	if err == nil {
		err = marshalErr
	}

	run.files[key] = fileValue{data: data, err: err}

	return installed, err
}

func validateManagedTool(spec ManagedToolSpec) error {
	if !validManagedLocation(spec.URL) ||
		!validManagedIdentity(spec.Name) || !validManagedIdentity(spec.Version) || !validHash(spec.SHA256) ||
		len(spec.Components) > managedSelectedLimit || spec.StripComponents < 0 || spec.StripComponents > managedPathDepth ||
		(spec.Format != managedZip && spec.Format != managedTarGzip && spec.Format != managedTarBzip2) {
		return errManagedTool
	}

	if !validManagedPath(spec.Executable) {
		return errManagedTool
	}

	seen := map[string]bool{strings.ToLower(spec.Executable): true}
	for _, name := range spec.Components {
		if !validManagedPath(name) || seen[strings.ToLower(name)] {
			return errManagedTool
		}

		seen[strings.ToLower(name)] = true
	}

	return nil
}

func validManagedIdentity(value string) bool {
	if value == "" || len(value) > managedIdentityLimit || value[0] == '.' || value[0] == '-' {
		return false
	}

	for _, char := range value {
		letter := (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')

		number := char >= '0' && char <= '9'
		if !letter && !number && !strings.ContainsRune("._+-", char) {
			return false
		}
	}

	return true
}

func validManagedLocation(value string) bool {
	location, err := url.Parse(value)

	return err == nil && location.Scheme == "https" && location.Host != "" && location.User == nil && location.Fragment == "" &&
		len(value) <= managedPathLimit && !strings.ContainsAny(value, "[],\\\"'\x00\r\n ")
}

func validManagedPath(name string) bool {
	return len(name) <= managedPathLimit && strings.Count(name, "/") <= managedPathDepth && fs.ValidPath(name) && name != "." &&
		!strings.ContainsAny(name, "\\:\x00\r\n")
}

func managedMiseArgument(spec ManagedToolSpec) (string, error) {
	identity, err := jsonDigest(spec)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http:karty-asset-%s-%s[url=%s,checksum=sha256:%s,format=%s,strip_components=%d,bin_path=bin]@%s",
		spec.Name, identity, spec.URL, spec.SHA256, spec.Format, spec.StripComponents, spec.Version), nil
}

func managedInstalledPath(directory, name string) (string, error) {
	full, err := confined(directory, name)
	if err != nil {
		return "", fmt.Errorf("mise installation path %s: %w", name, err)
	}

	return filepath.Clean(full), nil
}
