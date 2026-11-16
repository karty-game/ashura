//go:build !js

package framehost

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestFrameHostCommandsAndLifetime(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the pinned mise Node tool executes the JavaScript host contract")
	}

	fixture, err := os.ReadFile("testdata/host_test.cjs")
	if err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The selected Node tool runs only the tracked, embedded host contract fixture.
	command := exec.CommandContext(t.Context(), node, "--input-type=commonjs")

	command.Stdin = strings.NewReader(Source + "\n" + string(fixture))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("frame host contract failed: %v\n%s", err, output)
	}
}

func TestFrameHostIsStaticInlineSource(t *testing.T) {
	t.Parallel()

	for _, forbidden := range []string{"</script", "eval(", "new Function", "...args"} {
		if strings.Contains(Source, forbidden) {
			t.Fatalf("frame host contains an unsafe inline or allocating dynamic construct: %q", forbidden)
		}
	}
}
