//go:build js && wasm

package basistranscode

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"syscall/js"

	"github.com/karty-game/ashura/internal/texturedata"
)

//go:embed third_party/basis_transcoder.js
var basisJS string

//go:embed third_party/basis_transcoder.wasm
var basisWASM []byte

//go:embed worker.js
var workerJS string

var errWorker = errors.New("basis worker failed")

type session struct {
	done             <-chan struct{}
	cancel           context.CancelFunc
	mu               sync.Mutex
	worker           js.Value
	message, failure js.Func
	events           chan js.Value
	url              string
	callbacks        bool
}

// New allocates no worker, decoder or GPU resources until a compressed read.
func New() Decoder {
	ctx, cancel := context.WithCancel(context.Background())

	return &session{done: ctx.Done(), cancel: cancel, worker: js.Undefined()}
}

func (s *session) Close() {
	s.cancel()
	go func() { s.mu.Lock(); defer s.mu.Unlock(); s.shutdown() }()
}

func (s *session) Transcode(ctx context.Context, data []byte, format texturedata.Format) (output texturedata.Data, err error) {
	if ctx == nil {
		return output, ErrProfile
	}

	info, err := Inspect(data)
	if err != nil {
		return output, err
	}

	if format != texturedata.BC1 && format != texturedata.ETC2RGB {
		return output, ErrUnsupported
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("%w: %v", errWorker, failure)
		}

		if err != nil {
			s.shutdown()
		}
	}()

	if err = s.start(ctx); err != nil {
		return output, err
	}

	if err = ctx.Err(); err != nil {
		return output, err
	}

	if err = s.closed(); err != nil {
		return output, err
	}

	input := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(input, data)

	message := js.Global().Get("Object").New()
	message.Set("input", input)

	target := 0
	if format == texturedata.BC1 {
		target = 2
	}

	message.Set("format", target)

	transfer := js.Global().Get("Array").New()
	transfer.Call("push", input.Get("buffer"))
	s.worker.Call("postMessage", message, transfer)

	event, err := s.wait(ctx)
	if err != nil {
		return output, err
	}

	return copyTranscodedBlocks(event, info, format)
}

func (s *session) shutdown() {
	if !s.worker.IsUndefined() {
		s.worker.Set("onmessage", js.Null())
		s.worker.Set("onerror", js.Null())
		s.worker.Call("terminate")
		s.worker = js.Undefined()
	}

	if s.callbacks {
		s.message.Release()
		s.failure.Release()
		s.callbacks = false
	}

	if s.url != "" {
		js.Global().Get("URL").Call("revokeObjectURL", s.url)
		s.url = ""
	}
}

func (s *session) wait(ctx context.Context) (js.Value, error) {
	select {
	case <-ctx.Done():
		return js.Undefined(), ctx.Err()
	case <-s.done:
		return js.Undefined(), context.Canceled
	case event := <-s.events:
		if failure := event.Get("error"); !failure.IsUndefined() {
			return js.Undefined(), fmt.Errorf("%w: %s", errWorker, failure.String())
		}

		return event, nil
	}
}

func (s *session) start(ctx context.Context) error {
	if !s.worker.IsUndefined() {
		return nil
	}

	if err := s.closed(); err != nil {
		return err
	}

	s.events = make(chan js.Value, 1)
	s.message = js.FuncOf(func(_ js.Value, args []js.Value) any {
		select {
		case s.events <- args[0].Get("data"):
		default:
		}

		return nil
	})
	s.failure = js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := js.Global().Get("Object").New()
		event.Set("error", args[0].Get("message").String())

		select {
		case s.events <- event:
		default:
		}

		return nil
	})
	s.callbacks = true
	parts := js.Global().Get("Array").New()
	parts.Call("push", basisJS+"\n"+workerJS)
	blob := js.Global().Get("Blob").New(parts)
	s.url = js.Global().Get("URL").Call("createObjectURL", blob).String()
	s.worker = js.Global().Get("Worker").New(s.url)
	s.worker.Set("onmessage", s.message)
	s.worker.Set("onerror", s.failure)

	wasm := js.Global().Get("Uint8Array").New(len(basisWASM))
	js.CopyBytesToJS(wasm, basisWASM)

	input := js.Global().Get("Object").New()
	input.Set("wasm", wasm)

	transfer := js.Global().Get("Array").New()
	transfer.Call("push", wasm.Get("buffer"))
	s.worker.Call("postMessage", input, transfer)

	event, err := s.wait(ctx)
	if err != nil {
		return err
	}

	if !event.Get("ready").Truthy() {
		return ErrProfile
	}

	return nil
}

func (s *session) closed() error {
	select {
	case <-s.done:
		return context.Canceled
	default:
		return nil
	}
}

func copyTranscodedBlocks(event js.Value, info Info, format texturedata.Format) (texturedata.Data, error) {
	levels := event.Get("levels")
	if event.Get("width").Int() != info.Width || event.Get("height").Int() != info.Height || levels.Length() != info.Levels {
		return texturedata.Data{}, ErrProfile
	}

	output := texturedata.Data{
		Width:      info.Width,
		Height:     info.Height,
		ColorSpace: texturedata.SRGB,
		Format:     format,
		Levels:     make([][]byte, info.Levels),
	}
	for level := range info.Levels {
		row, rows := texturedata.Layout(format, max(1, info.Width>>level), max(1, info.Height>>level))

		bytes := levels.Index(level)
		if bytes.Get("byteLength").Int() != row*rows {
			return texturedata.Data{}, ErrProfile
		}

		output.Levels[level] = make([]byte, row*rows)
		if js.CopyBytesToGo(output.Levels[level], bytes) != row*rows {
			return texturedata.Data{}, ErrProfile
		}
	}

	return output, nil
}
