package engine_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
)

type frameObserver struct{ closes int }

func (*frameObserver) Active() bool              { return false }
func (*frameObserver) Observe(engine.FrameStats) {}
func (observer *frameObserver) Close()           { observer.closes++ }

func TestFrameObserverOwnership(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	first, second := &frameObserver{}, &frameObserver{}
	app.SetFrameObserver(first)
	app.SetFrameObserver(second)

	if first.closes != 1 || second.closes != 0 {
		t.Fatal("replacement did not transfer observer ownership")
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if second.closes != 1 {
		t.Fatal("App shutdown did not close its observer")
	}

	app.SetFrameObserver(first)

	if first.closes != 2 {
		t.Fatal("closed App retained a new observer")
	}
}
