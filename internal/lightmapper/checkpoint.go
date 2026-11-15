package lightmapper

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
)

const CheckpointSchema = "karty.lightmapper-checkpoint@1"

// CheckpointIdentity freezes every estimator/allocation dependency. Sample
// ranges are meaningful only within the same independent production attempt.
type CheckpointIdentity struct{ Scene, Receivers, Algorithm, Backend, Pilot, Allocation, Production string }

// CommittedRange owns exactly the half-open sample interval [First,First+Count).
type CommittedRange struct {
	ReceiverID, First, Count uint64
	Statistics               Statistics
}

type Checkpoint struct {
	Schema   string
	Identity CheckpointIdentity
	Ranges   []CommittedRange
}

var errCheckpoint = errors.New("lightmapper: incompatible or overlapping checkpoint ranges")

func (checkpoint Checkpoint) Validate(expected CheckpointIdentity) error {
	if checkpoint.Schema != CheckpointSchema || checkpoint.Identity != expected || expected.Scene == "" || expected.Receivers == "" ||
		expected.Algorithm == "" ||
		expected.Backend == "" ||
		expected.Pilot == "" ||
		expected.Allocation == "" ||
		expected.Production == "" {
		return errCheckpoint
	}

	ranges := append([]CommittedRange(nil), checkpoint.Ranges...)
	sort.Slice(ranges, func(index, j int) bool {
		if ranges[index].ReceiverID != ranges[j].ReceiverID {
			return ranges[index].ReceiverID < ranges[j].ReceiverID
		}

		return ranges[index].First < ranges[j].First
	})

	for index, current := range ranges {
		if current.Count == 0 || math.MaxUint64-current.First < current.Count || current.Statistics.Count != current.Count ||
			!current.Statistics.valid() {
			return errCheckpoint
		}

		if index > 0 {
			previous := ranges[index-1]
			if previous.ReceiverID == current.ReceiverID && previous.First+previous.Count > current.First {
				return errCheckpoint
			}
		}
	}

	return nil
}

// Encode validates before serialization. Artifact storage supplies content
// hashing, atomic replacement and path confinement outside this value format.
//
//nolint:musttag // Checkpoint v1 deliberately preserves its canonical exported field names.
func (checkpoint Checkpoint) Encode() ([]byte, error) {
	if err := checkpoint.Validate(checkpoint.Identity); err != nil {
		return nil, err
	}

	return json.Marshal(checkpoint)
}

//nolint:musttag // Decode the same versioned canonical checkpoint record.
func DecodeCheckpoint(data []byte, expected CheckpointIdentity) (Checkpoint, error) {
	var result Checkpoint
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}

	return result, result.Validate(expected)
}
