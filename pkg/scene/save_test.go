package scene

import (
	"bytes"
	"errors"
	"testing"
)

var errInvalidState = errors.New("invalid state")

func TestPersistentAndTemporaryStateAcrossRootAndApplication(t *testing.T) {
	t.Parallel()

	saves := NewSaves()
	root, _ := NewRoot("world-A")

	permanent, err := Bool(root, saves, "light", false, true)
	if err != nil {
		t.Fatal(err)
	}

	temporary, err := Bool(root, saves, "marker", false, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := permanent.Set(true); err != nil {
		t.Fatal(err)
	}

	if err := temporary.Set(true); err != nil {
		t.Fatal(err)
	}

	liveSnapshot, err := saves.Export()
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(liveSnapshot, []byte("marker")) {
		t.Fatal("temporary state was saved")
	}

	root.Close()

	if len(saves.active) != 0 || len(saves.owners) != 0 {
		t.Fatal("closed root retained live callbacks")
	}

	retained, err := saves.Export()
	if err != nil || !bytes.Equal(liveSnapshot, retained) {
		t.Fatalf("retention changed data: %v", err)
	}

	freshService := NewSaves()
	if err := freshService.Import(retained); err != nil {
		t.Fatal(err)
	}

	restored, _ := NewRoot("world-A")

	light, err := Bool(restored, freshService, "light", false, true)
	if err != nil || !light.Get() {
		t.Fatalf("persistent state not restored: %v", err)
	}

	marker, err := Bool(restored, freshService, "marker", false, false)
	if err != nil || marker.Get() {
		t.Fatalf("temporary state not reset: %v", err)
	}

	other, _ := NewRoot("world-B")

	otherLight, err := Bool(other, freshService, "light", false, true)
	if err != nil || otherLight.Get() {
		t.Fatalf("state leaked between roots: %v", err)
	}

	if err := freshService.Import(retained); err == nil {
		t.Fatal("import replaced live state")
	}

	restored.Close()
	other.Close()

	if err := freshService.Reset(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveRejectsVersionTypeDuplicateOwnerAndFailedCapture(t *testing.T) {
	t.Parallel()

	saves := NewSaves()

	root, _ := NewRoot("level")
	if _, err := Bool(root, saves, "light", false, true); err != nil {
		t.Fatal(err)
	}

	duplicate, _ := NewRoot("level")
	if _, err := Bool(duplicate, saves, "different", false, true); err == nil {
		t.Fatal("duplicate live root identity accepted")
	}

	root.Close()
	duplicate.Close()

	before, _ := saves.Export()
	for _, input := range []string{`{}`, `{"version":2,"roots":{}}`, `{"version":1,"roots":{}} {}`} {
		if err := saves.Import([]byte(input)); err == nil {
			t.Fatal(input)
		}

		after, _ := saves.Export()
		if !bytes.Equal(before, after) {
			t.Fatal("invalid import changed retained data")
		}
	}

	bad := NewSaves()
	if err := bad.Import([]byte(`{"version":1,"roots":{"level":{"light":{"version":1,"data":"true"}}}}`)); err != nil {
		t.Fatal(err)
	}

	invalid, _ := NewRoot("level")
	if _, err := Bool(invalid, bad, "light", false, true); err == nil {
		t.Fatal("string restored into boolean")
	}

	invalid.Close()

	failed := NewSaves()
	failingRoot, _ := NewRoot("failed")
	errCapture := errInvalidState
	invalidCapture := false

	if err := Persist(failingRoot, failed, "component", 1, func() bool { return false },
		func(bool) error {
			if invalidCapture {
				return errCapture
			}

			return nil
		}, func(bool) error { return nil }); err != nil {
		t.Fatal(err)
	}

	invalidCapture = true

	failingRoot.Close()

	if _, err := failed.Export(); !errors.Is(err, errCapture) {
		t.Fatal(err)
	}

	if len(failed.active) != 0 || len(failed.owners) != 0 {
		t.Fatal("failed capture retained a component callback")
	}
}

func TestAbortedConstructionDoesNotCommitSaveData(t *testing.T) {
	t.Parallel()

	saves := NewSaves()
	root, _ := NewRoot("world")

	value, err := Bool(root, saves, "light", true, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := value.Set(false); err != nil {
		t.Fatal(err)
	}

	root.Abort()

	data, err := saves.Export()
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(data, []byte("light")) || len(saves.owners) != 0 || len(saves.active) != 0 {
		t.Fatal("construction rollback retained component state", string(data))
	}
}
