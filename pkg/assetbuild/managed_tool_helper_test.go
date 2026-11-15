//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

var errManagedMiseFixture = errors.New("invalid fake mise invocation")

func managedMiseHelper() int {
	if err := managedMiseHelperRun(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 1
	}

	return 0
}

func managedMiseHelperRun() error {
	if len(os.Args) != 6 || !slices.Equal(os.Args[1:4], []string{"--no-config", "--no-env", "--no-hooks"}) {
		return errManagedMiseFixture
	}

	//nolint:gosec // The parent test selects its private invocation log.
	log, err := os.OpenFile(os.Getenv("KARTY_MISE_CALLS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	err = json.NewEncoder(log).Encode(os.Args[1:])

	closeErr := log.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}

	if os.Getenv("KARTY_MISE_MODE") == "wait" {
		time.Sleep(time.Minute)
	}

	if os.Args[4] == "install" {
		if os.Getenv("KARTY_MISE_MODE") == "fail" {
			_, _ = fmt.Fprint(os.Stdout, strings.Repeat("stdout", managedMiseLogLimit))
			_, _ = fmt.Fprint(os.Stderr, strings.Repeat("stderr", managedMiseLogLimit))

			return errManagedMiseFixture
		}

		return nil
	}

	if os.Args[4] != "where" {
		return errManagedMiseFixture
	}

	if os.Getenv("KARTY_MISE_MODE") == "relative" {
		_, err = fmt.Fprintln(os.Stdout, "relative/installation")
	} else {
		_, err = fmt.Fprintln(os.Stdout, os.Getenv("KARTY_MISE_INSTALL_ROOT"))
	}

	return err
}

type managedMiseFixture struct {
	spec        ManagedToolSpec
	root        string
	mise        string
	invocations string
	binary      []byte
}

func newManagedMiseFixture(t *testing.T) *managedMiseFixture {
	t.Helper()

	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // Copy the current test executable to dispatch the private fake mise process.
	binary, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}

	base := t.TempDir()
	fixture := &managedMiseFixture{
		spec: ManagedToolSpec{Name: "fixture", Version: "1.0.0", URL: "https://tools.invalid/fixture.zip",
			SHA256: strings.Repeat("b", 64), Format: managedZip, Executable: "bin/tool" + suffix,
			Components: []string{"lib/component.so"}},
		root: filepath.Join(base, "installation"), mise: filepath.Join(base, "bin", "mise"+suffix),
		invocations: filepath.Join(base, "calls.jsonl"), binary: binary,
	}
	fixture.write(t, fixture.mise, binary)
	fixture.write(t, filepath.Join(fixture.root, filepath.FromSlash(fixture.spec.Executable)), binary)
	fixture.write(t, filepath.Join(fixture.root, "lib", "component.so"), []byte("component bytes"))
	fixture.write(t, filepath.Join(fixture.root, "notices", "Materialize-LICENSE"), []byte("fixture notice"))
	t.Setenv("PATH", filepath.Dir(fixture.mise)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KARTY_MISE_HELPER", "1")
	t.Setenv("KARTY_MISE_MODE", "")
	t.Setenv("KARTY_MISE_CALLS", fixture.invocations)
	t.Setenv("KARTY_MISE_INSTALL_ROOT", fixture.root)

	return fixture
}

func (fixture *managedMiseFixture) write(t *testing.T, name string, data []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // Private test executables and component fixtures need executable permission on POSIX hosts.
	if err := os.WriteFile(name, data, 0o700); err != nil {
		t.Fatal(err)
	}
}

func (fixture *managedMiseFixture) calls(t *testing.T) [][]string {
	t.Helper()

	data, err := os.ReadFile(fixture.invocations)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		t.Fatal(err)
	}

	var calls [][]string

	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var arguments []string
		if err := json.Unmarshal([]byte(line), &arguments); err != nil {
			t.Fatal(err)
		}

		calls = append(calls, arguments)
	}

	return calls
}
