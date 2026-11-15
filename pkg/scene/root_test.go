package scene

import (
	"errors"
	"reflect"
	"testing"
)

func TestRootStateSurvivesPresentationAndEndsExactlyOnce(t *testing.T) {
	t.Parallel()

	root, err := NewRoot("world")
	if err != nil {
		t.Fatal(err)
	}

	state, err := Bool(root, nil, "switch", false, false)
	if err != nil {
		t.Fatal(err)
	}

	var cleanup []int

	for _, id := range []int{1, 2, 3} {
		if err := root.Own(func() { cleanup = append(cleanup, id) }); err != nil {
			t.Fatal(err)
		}
	}

	if err := state.Set(true); err != nil {
		t.Fatal(err)
	}

	if !state.Get() || root.Closed() {
		t.Fatal("logical state changed without root teardown")
	}

	root.Close()
	root.Close()

	if !reflect.DeepEqual(cleanup, []int{3, 2, 1}) {
		t.Fatal(cleanup)
	}

	if err := state.Set(false); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}

	if err := root.Own(func() {}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}

	if _, err := Bool(root, nil, "another", true, false); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}

	fresh, _ := NewRoot("world")

	freshState, err := Bool(fresh, nil, "switch", false, false)
	if err != nil || freshState.Get() {
		t.Fatalf("fresh scene did not receive defaults: %v", err)
	}

	fresh.Close()
}

func TestSignalDeliveryMutationAndFeedback(t *testing.T) {
	t.Parallel()

	value := NewValue(false)

	var (
		calls  []string
		second func()
	)

	first := value.Subscribe(func(bool) {
		calls = append(calls, "first")

		second()
		value.Subscribe(func(bool) { calls = append(calls, "late") })

		if err := value.Set(false); !errors.Is(err, ErrReentrant) {
			t.Errorf("feedback: %v", err)
		}
	})

	second = value.Subscribe(func(bool) { calls = append(calls, "second") })
	if err := value.Set(true); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(calls, []string{"first"}) || !value.Get() {
		t.Fatalf("callbacks=%v state=%v", calls, value.Get())
	}

	if err := value.Set(true); err != nil || len(calls) != 1 {
		t.Fatal("equal writes emitted changes")
	}

	first()
	first()

	if err := value.Set(false); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(calls, []string{"first", "late"}) {
		t.Fatal(calls)
	}
}

//nolint:paralleltest // Go 1.27 prohibits AllocsPerRun inside parallel tests.
func TestWarmPropertyUpdateAllocatesNothing(t *testing.T) {
	value := NewValue(false)
	calls := 0

	value.Subscribe(func(bool) { calls++ })

	allocations := testing.AllocsPerRun(100, func() {
		if err := value.Set(!value.Get()); err != nil {
			panic(err)
		}
	})
	if allocations != 0 || calls == 0 {
		t.Fatalf("allocations=%v calls=%d", allocations, calls)
	}
}
