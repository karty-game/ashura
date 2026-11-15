package engine

import (
	"math"
	"testing"
)

func TestCloseCallbacksObserveClosedAndCancel(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	called := 0

	cancel, err := app.OnClose(func() { t.Fatal("canceled resource cleanup ran") })
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	cancel()

	_, err = app.OnClose(func() {
		called++

		if !app.Closed() {
			t.Fatal("resource cleanup observed an open App")
		}

		if err := app.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()

	if called != 1 || len(app.closeCallbacks) != 0 {
		t.Fatalf("cleanup calls=%d, retained callbacks=%d", called, len(app.closeCallbacks))
	}

	if _, err := app.OnClose(func() {}); err == nil {
		t.Fatal("closed App accepted new resource cleanup")
	}
}

func TestCloseCallbacksRejectInvalidRegistrationAndWrap(t *testing.T) {
	t.Parallel()

	var absent *App
	if _, err := absent.OnClose(func() {}); err == nil {
		t.Fatal("nil App accepted cleanup")
	}

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	if _, err := app.OnClose(nil); err == nil {
		t.Fatal("accepted nil cleanup")
	}

	app.nextCloseCallback = math.MaxUint64
	if _, err := app.OnClose(func() {}); err == nil {
		t.Fatal("callback counter wrapped and could alias stale cancellation")
	}
}
