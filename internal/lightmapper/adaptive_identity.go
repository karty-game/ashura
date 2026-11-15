//go:build !js

package lightmapper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

//nolint:musttag // The private identity deliberately includes the complete versioned options and policy values, including their exported field names.
func newAdaptiveState(
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	policy AdaptivePolicy,
	backend string,
) (*AdaptiveState, error) {
	hash, err := ReceiverHash(receivers)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(struct {
		Algorithm string         `json:"algorithm"`
		Scene     string         `json:"scene"`
		Receivers string         `json:"receivers"`
		Backend   string         `json:"backend"`
		Options   Options        `json:"options"`
		Policy    AdaptivePolicy `json:"policy"`
	}{Algorithm, scene.Hash(), hash, backend, options, policy})
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256(encoded)
	identity := hex.EncodeToString(digest[:])
	state := &AdaptiveState{
		Schema:       AdaptiveSchema,
		Identity:     identity,
		SceneHash:    scene.Hash(),
		ReceiverHash: hash,
		Backend:      backend,
		Options:      options,
		Policy:       policy,
	}
	state.PilotSeed = adaptiveSeed(identity, "pilot")

	state.ProductionSeed = adaptiveSeed(identity, "production")

	return state, nil
}
