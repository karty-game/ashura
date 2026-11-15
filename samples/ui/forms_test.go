package main

import (
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func TestFormStatusRequiresEveryFieldToBeValid(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	status := ""
	check := func(err error) {
		if err != nil {
			t.Error(err)
		}
	}

	view, err := ui.New(formShowcase(func(text string) { status = text }, check), font)
	if err != nil {
		t.Fatal(err)
	}

	if err := view.Resize(800, 800); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(time.Millisecond); err != nil {
		t.Fatal(err)
	}

	find := func(label string) *ui.Node {
		for _, semantic := range view.Semantics() {
			if semantic.Name == label && (semantic.Role == "textbox" || label == "Validate form") {
				return view.Lookup(semantic.ID)
			}
		}

		t.Fatalf("missing control %q", label)

		return nil
	}

	name, email, validate := find("Full name"), find("Email address"), find("Validate form")
	for _, test := range []struct {
		name, email string
		valid       bool
	}{{"", "ada@example.com", false}, {"Ada", "ada@", false}, {"", "ada@", false}, {"Ada", "ada@example.com", true}} {
		check(name.SetText(test.name))
		check(email.SetText(test.email))

		result := view.Dispatch(ui.Event{Kind: ui.Activate, Target: validate.ID()})
		check(result.Err)

		if strings.Contains(status, "ready") != test.valid {
			t.Fatalf("name=%q email=%q status=%q", test.name, test.email, status)
		}
	}
}
