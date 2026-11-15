package scene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
)

const (
	saveVersion = 1
	maxSaveSize = 4 << 20
	maxRecords  = 16384
)

var errSave = errors.New("invalid scene save snapshot")

type savedRecord struct {
	Version uint32          `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type snapshot struct {
	Version int                               `json:"version"`
	Roots   map[string]map[string]savedRecord `json:"roots"`
}

// Saves retains one application's selected component data. Live registrations
// reference their roots until close; retained records contain data only.
// Snapshot bytes can use any storage backend.
type Saves struct {
	roots    map[string]map[string]savedRecord
	active   map[string]map[string]func() (savedRecord, error)
	owners   map[string]*Root
	failures map[string]error
}

// NewSaves creates an independent application's persistence service.
func NewSaves() *Saves {
	return &Saves{roots: make(map[string]map[string]savedRecord), active: make(map[string]map[string]func() (savedRecord, error)),
		owners: make(map[string]*Root), failures: make(map[string]error)}
}

// Persist registers a typed saveable component and restores it before hooks run.
// capture must return owned data; validate must reject invalid saved values.
// On root close the latest record is retained before component teardown. Capture
// errors remain observable through Export, so close never silently discards them.
func Persist[T any](root *Root, saves *Saves, identity string, version uint32,
	capture func() T, validate func(T) error, restore func(T) error,
) error {
	if root.Closed() {
		return ErrClosed
	}

	if saves == nil || identity == "" || version == 0 || capture == nil || validate == nil || restore == nil {
		return errSave
	}

	if owner := saves.owners[root.ID()]; owner != nil && owner != root {
		return fmt.Errorf("save root %s is already live: %w", root.ID(), errSave)
	}

	if saves.active[root.ID()][identity] != nil {
		return fmt.Errorf("duplicate save identity %s/%s: %w", root.ID(), identity, errSave)
	}

	if record, found := saves.roots[root.ID()][identity]; found {
		var state T

		if record.Version != version {
			return fmt.Errorf("save component %s version %d, expected %d: %w", identity, record.Version, version, errSave)
		}

		if err := strictJSON(record.Data, &state); err != nil {
			return fmt.Errorf("save component %s: %w", identity, err)
		}

		if err := validate(state); err != nil {
			return err
		}

		if err := restore(state); err != nil {
			return err
		}
	}

	if saves.active[root.ID()] == nil {
		saves.active[root.ID()] = make(map[string]func() (savedRecord, error))
	}

	saves.active[root.ID()][identity] = func() (savedRecord, error) {
		state := capture()
		if err := validate(state); err != nil {
			return savedRecord{}, err
		}

		data, err := json.Marshal(state)

		return savedRecord{Version: version, Data: data}, err
	}
	saves.owners[root.ID()] = root

	return root.Own(func() { saves.retain(root.ID(), identity, !root.aborted) })
}

// Export captures live and retained component state at the game-loop boundary.
func (saves *Saves) Export() ([]byte, error) {
	for identity, err := range saves.failures {
		return nil, fmt.Errorf("retaining %s: %w", identity, err)
	}

	result := snapshot{Version: saveVersion, Roots: make(map[string]map[string]savedRecord)}
	for root, records := range saves.roots {
		result.Roots[root] = make(map[string]savedRecord)
		maps.Copy(result.Roots[root], records)
	}

	for root, records := range saves.active {
		if result.Roots[root] == nil {
			result.Roots[root] = make(map[string]savedRecord)
		}

		for identity, capture := range records {
			record, err := capture()
			if err != nil {
				return nil, fmt.Errorf("save %s/%s: %w", root, identity, err)
			}

			result.Roots[root][identity] = record
		}
	}

	data, err := json.Marshal(result)
	if err == nil {
		err = validateSnapshot(data, result)
	}

	return data, err
}

// Import replaces retained records atomically while no saveable roots are live.
// Load before constructing roots; component types and versions validate on restore.
func (saves *Saves) Import(data []byte) error {
	if len(saves.active) != 0 || len(data) > maxSaveSize {
		return errSave
	}

	var result snapshot
	if err := strictJSON(data, &result); err != nil {
		return err
	}

	if err := validateSnapshot(data, result); err != nil {
		return err
	}

	saves.roots = result.Roots
	saves.failures = make(map[string]error)

	return nil
}

// Reset discards retained save data only when no saveable roots are active.
func (saves *Saves) Reset() error {
	if len(saves.owners) != 0 {
		return errSave
	}

	saves.roots = make(map[string]map[string]savedRecord)
	saves.failures = make(map[string]error)

	return nil
}

func (saves *Saves) retain(root, identity string, commit bool) {
	capture := saves.active[root][identity]
	if capture == nil {
		return
	}

	if commit {
		record, err := capture()
		if err != nil {
			saves.failures[root+"/"+identity] = err
		} else {
			if saves.roots[root] == nil {
				saves.roots[root] = make(map[string]savedRecord)
			}

			saves.roots[root][identity] = record
		}
	}

	delete(saves.active[root], identity)

	if len(saves.active[root]) == 0 {
		delete(saves.active, root)
		delete(saves.owners, root)
	}
}

func validateSnapshot(data []byte, result snapshot) error {
	if len(data) > maxSaveSize || result.Version != saveVersion || result.Roots == nil {
		return errSave
	}

	count := 0

	for root, records := range result.Roots {
		if strings.TrimSpace(root) == "" {
			return errSave
		}

		for identity, record := range records {
			count++
			if identity == "" || record.Version == 0 || !json.Valid(record.Data) || count > maxRecords {
				return errSave
			}
		}
	}

	return nil
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("save decode: %w", err)
	}

	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errSave
	}

	return nil
}
