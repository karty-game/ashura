//go:build !js

package assetbuild_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

type executionContractPlugin struct{ token *assetbuild.ExecutionResource }

func (executionContractPlugin) Name() string { return "public-execution-contract" }
func (plugin executionContractPlugin) Plan(plan *assetbuild.Plan) error {
	var refs []assetbuild.Ref

	for _, identity := range []string{"left", "right"} {
		outputs, err := plan.Add(
			assetbuild.Action{
				ID:        identity,
				Processor: "literal",
				Revision:  "1",
				Parallel:  true,
				Exclusive: plugin.token,
				Outputs:   []assetbuild.Slot{{Name: "data", Schema: "literal@1"}},
				Run: func(*assetbuild.Context) (assetbuild.Result, error) {
					return assetbuild.Result{"data": []byte(identity)}, nil
				},
			},
		)
		if err != nil {
			return err
		}

		refs = append(refs, outputs...)
	}

	outputs, err := plan.Add(
		assetbuild.Action{
			ID:        "publish",
			Processor: "join",
			Revision:  "1",
			Inputs:    refs,
			Outputs:   []assetbuild.Slot{{Name: "files", Schema: "files@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var joined []byte

				for _, ref := range refs {
					data, err := ctx.Input(ref)
					if err != nil {
						return nil, err
					}

					joined = append(joined, data...)
				}

				encoded, err := assetbuild.EncodeBundle(assetbuild.Bundle{"value.txt": joined})

				return assetbuild.Result{"files": encoded}, err
			},
		},
	)
	if err == nil {
		plan.Publish(outputs[0])
	}

	return err
}

func TestPublicBuildExecutionContract(t *testing.T) {
	t.Parallel()

	token := assetbuild.NewExecutionResource()
	if token == assetbuild.NewExecutionResource() {
		t.Fatal("distinct resource identities alias")
	}

	var events []assetbuild.ActionEvent

	project := t.TempDir()

	report, err := assetbuild.Build(
		t.Context(),
		assetbuild.Config{Output: "generated", Plugins: []assetbuild.Plugin{executionContractPlugin{token}}},
		assetbuild.Options{
			Project:  project,
			Identity: "execution-contract",
			Jobs:     2,
			Trace:    func(event assetbuild.ActionEvent) { events = append(events, event) },
		},
	)
	if err != nil || report.Executed != 3 {
		t.Fatal(report, err)
	}

	//nolint:gosec // G304: fixed fixture path below this test's private TempDir.
	data, err := os.ReadFile(filepath.Join(project, "generated/value.txt"))
	if err != nil || string(data) != "leftright" {
		t.Fatal(string(data), err)
	}

	const eventCount = 6
	if len(events) != eventCount {
		t.Fatal("missing lifecycle events", events)
	}

	identities := make([]string, 0, len(events))

	for index, event := range events {
		if event.Time.IsZero() || event.Err != nil || event.Processor == "" {
			t.Fatal("incomplete event", event)
		}

		phase := assetbuild.ActionStarted
		if index%2 != 0 {
			phase = assetbuild.ActionFinished
		}

		if event.Phase != phase {
			t.Fatal("resource/serial barrier events overlapped", events)
		}

		identities = append(identities, event.Action)
	}

	if !slices.Equal(identities, []string{"left", "left", "right", "right", "publish", "publish"}) {
		t.Fatal("exclusive token or serial publication order changed", identities)
	}
}
