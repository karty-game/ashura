package engine

import (
	"errors"

	"github.com/karty-game/ashura/internal/uihost"
	"github.com/karty-game/ashura/pkg/ui"
)

// Frame tests exercise UI without creating a window or importing the optional
// uiapp integration back into its own engine dependency.
type testUIAttachment struct{ *ui.View }

var errFrameOnlyUI = errors.New("frame-only UI fixture cannot open a platform backend")

func (*testUIAttachment) AttachUI(any) (uihost.Bridge, error) {
	return nil, errFrameOnlyUI
}
