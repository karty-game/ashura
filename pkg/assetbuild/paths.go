//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const maxArtifactBytes = 256 << 20

var errPath = errors.New("asset path escapes its owner, overlaps generated state, or follows a symlink")

func projectRoot(directory string) (string, error) {
	if directory == "" {
		directory = "."
	}

	root, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("asset project: %w", err)
	}

	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("asset project: %w", err)
	}

	if canonical != root {
		return "", fmt.Errorf("project %q: %w", root, errPath)
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("project %q must be a directory: %w", root, errPath)
	}

	return root, nil
}

func confined(root, name string) (string, error) {
	full := name
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, filepath.FromSlash(name))
	}

	relative, err := filepath.Rel(root, full)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("path %q: %w", name, errPath)
	}

	current := root
	for part := range strings.SplitSeq(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)

		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}

		if statErr != nil {
			return "", fmt.Errorf("inspect asset path: %w", statErr)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %q: %w", name, errPath)
		}
	}

	return filepath.Join(root, relative), nil
}

func readProjectFile(root, name string) ([]byte, error) {
	full, err := confined(root, name)
	if err != nil {
		return nil, err
	}

	return readBounded(full)
}

func readBounded(full string) ([]byte, error) {
	//nolint:gosec // The caller confines source/cache paths; executable fingerprinting deliberately selects os.Executable.
	file, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("read asset %s: %w", full, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect asset: %w", err)
	}

	if !info.Mode().IsRegular() || info.Size() > maxArtifactBytes {
		return nil, fmt.Errorf("asset must be a regular file no larger than %d bytes: %w", maxArtifactBytes, errPath)
	}

	return readSizedBounded(file, info.Size())
}

// readSizedBounded uses the admitted file size to avoid geometric ReadAll
// growth and repeated full-payload copies. A spare byte permits EOF detection
// without another allocation. Files that change length during reading retain
// the original bounded-reader behavior, including short and growing payloads.
//
//nolint:makezero,prealloc // The initial buffer contains bytes already read; only a concurrently growing file needs a tail append.
func readSizedBounded(reader io.Reader, expected int64) ([]byte, error) {
	if expected < 0 || expected > maxArtifactBytes {
		return nil, fmt.Errorf("asset exceeds byte limit: %w", errPath)
	}

	data := make([]byte, int(expected)+1)

	count := 0
	for count < len(data) {
		n, readErr := reader.Read(data[count:])
		count += n

		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fmt.Errorf("read asset: %w", readErr)
		}

		if count > maxArtifactBytes {
			return nil, fmt.Errorf("asset exceeds byte limit: %w", errPath)
		}

		if errors.Is(readErr, io.EOF) {
			return data[:count], nil
		}
	}

	remaining, err := io.ReadAll(io.LimitReader(reader, int64(maxArtifactBytes-count)+1))
	if err != nil {
		return nil, fmt.Errorf("read asset: %w", err)
	}

	data = append(data, remaining...)
	if len(data) > maxArtifactBytes {
		return nil, fmt.Errorf("asset exceeds byte limit: %w", errPath)
	}

	return data, nil
}

func globFiles(root, pattern string) ([]string, error) {
	prefix, err := globPrefix(pattern)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(pattern, "/")

	base := root
	if prefix != "" {
		base, err = confined(root, prefix)
		if err != nil {
			return nil, err
		}
	}

	var names []string

	err = filepath.WalkDir(base, func(full string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) && full == base {
			return nil
		}

		if walkErr != nil {
			return fmt.Errorf("discover asset: %w", walkErr)
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source %s: %w", full, errPath)
		}

		if entry.IsDir() {
			return nil
		}

		relative, relErr := filepath.Rel(root, full)
		if relErr != nil {
			return fmt.Errorf("asset relative path: %w", relErr)
		}

		name := filepath.ToSlash(relative)
		if matchParts(parts, strings.Split(name, "/")) {
			names = append(names, name)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Sort(names)

	return names, nil
}

func globPrefix(pattern string) (string, error) {
	if !fs.ValidPath(pattern) || strings.ContainsAny(pattern, "\\:") {
		return "", fmt.Errorf("glob %q: %w", pattern, errPath)
	}

	parts := strings.Split(pattern, "/")
	prefix := make([]string, 0, len(parts))
	wildcard := false

	for _, part := range parts {
		if part != "**" {
			if _, err := path.Match(part, ""); err != nil {
				return "", fmt.Errorf("invalid glob %q: %w", pattern, err)
			}
		}

		wildcard = wildcard || strings.ContainsAny(part, "*?[")
		if !wildcard {
			prefix = append(prefix, part)
		}
	}

	return strings.Join(prefix, "/"), nil
}

func matchParts(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}

	if pattern[0] == "**" {
		return matchParts(pattern[1:], name) || len(name) > 0 && matchParts(pattern, name[1:])
	}

	if len(name) == 0 {
		return false
	}

	matched, err := path.Match(pattern[0], name[0])

	return err == nil && matched && matchParts(pattern[1:], name[1:])
}

func overlaps(left, right string) bool {
	relative, err := filepath.Rel(left, right)
	if err == nil && filepath.IsLocal(relative) {
		return true
	}

	relative, err = filepath.Rel(right, left)

	return err == nil && filepath.IsLocal(relative)
}
