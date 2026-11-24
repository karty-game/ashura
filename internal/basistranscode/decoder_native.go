//go:build !js

package basistranscode

import (
	"context"

	"github.com/karty-game/ashura/internal/texturedata"
)

type unsupported struct{}

func New() Decoder         { return unsupported{} }
func (unsupported) Close() {}
func (unsupported) Transcode(context.Context, []byte, texturedata.Format) (texturedata.Data, error) {
	return texturedata.Data{}, ErrUnsupported
}
