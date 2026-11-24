package level

import (
	"fmt"

	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/texture"
)

var (
	// ErrClosed means a loader, chunk, task or its owning App is unavailable.
	ErrClosed = engine.ErrClosed
	// ErrInUse means live presentations or bindings prevent resource release.
	// Remove them before retrying. It is shared with engine.ErrInUse.
	ErrInUse = engine.ErrInUse
	// ErrFormat marks invalid prepared level data or an invalid chunk argument.
	ErrFormat = leveldata.ErrFormat

	errLifetime = fmt.Errorf("level resource is unavailable: %w", ErrClosed)
	errInUse    = fmt.Errorf("level resource has live presentations: %w", ErrInUse)
)

type sharedTexture struct {
	image                       *texture.Texture
	references                  int
	width, height, levels, size int
	space                       texture.ColorSpace
	format                      texture.Format
}

// Loader shares immutable textures among resident chunks. Its methods belong to
// the App loop; resource I/O is performed separately by ReadChunk.
type Loader struct {
	app      *engine.App
	textures map[string]*sharedTexture
	chunks   map[*LoadedChunk]bool
	tasks    map[*LoadTask]bool
	closed   bool
	decoder  basistranscode.Decoder
	cancel   func()
}

// NewLoader establishes App-owned lifetime for offline-prepared GPU payloads.
// It includes no texture decoder. Legacy ETC1S assets require an explicitly
// supplied transcoder through NewLoaderWithTranscoder or level/basis.NewLoader.
func NewLoader(app *engine.App) *Loader {
	return NewLoaderWithTranscoder(app, nil)
}

type loadedPage struct {
	model     *model.Model
	materials []material.SourceMaterial
	standard  []material.Material
}

func (page loadedPage) materialCount() int {
	if len(page.standard) > 0 {
		return len(page.standard)
	}

	return len(page.materials)
}

// LoadedChunk owns prepared geometry and shared textures independently of logical scene state.
type LoadedChunk struct {
	loader      *Loader
	description Chunk
	pages       []loadedPage
	textures    []string
	textureSet  map[string]bool
	instances   map[*Instance]bool
	closed      bool
}

// Instance is one static chunk presentation. Removing it preserves logical objects.
type Instance struct {
	chunk              *LoadedChunk
	world              *engine.World
	models             []*model.Instance
	nextPage, nextPart int
	presenting         bool
	decals             *engine.DecalPlacement
	decalLayers        uint32
}

// Load is the synchronous convenience form of Begin and Step. Interactive
// consumers should call Step once per update to spread admission and CPU copies.
func (loader *Loader) Load(prepared *PreparedChunk) (*LoadedChunk, error) {
	task, err := loader.Begin(prepared)
	if err != nil {
		return nil, err
	}

	for {
		done, stepErr := task.Step()
		if stepErr != nil {
			return nil, stepErr
		}

		if done {
			return task.Result()
		}
	}
}

func (chunk *LoadedChunk) acquire(prepared *PreparedChunk, ref Reference) (*texture.Texture, error) {
	info, found := prepared.imageInfo[ref.SHA256]
	if !found {
		return nil, leveldata.ErrFormat
	}

	existing := chunk.loader.textures[ref.SHA256]
	if existing != nil {
		if !texturedata.Valid(existing.image) {
			return nil, errLifetime
		}

		if existing.metadata() != info {
			return nil, leveldata.ErrFormat
		}

		chunk.retain(ref.SHA256, existing)

		return existing.image, nil
	}

	data, found := prepared.images[ref.SHA256]
	if !found {
		return nil, ErrStaleRequest
	}

	image, err := chunk.loader.app.AdoptTexture(data)
	if err != nil {
		return nil, err
	}

	shared := &sharedTexture{
		image:  image,
		size:   ref.Size,
		width:  data.Width,
		height: data.Height,
		levels: len(data.Levels),
		space:  data.ColorSpace,
		format: data.Format,
	}
	chunk.loader.textures[ref.SHA256] = shared
	chunk.retain(ref.SHA256, shared)

	return image, nil
}

func (chunk *LoadedChunk) retain(key string, shared *sharedTexture) {
	if chunk.textureSet[key] {
		return
	}

	shared.references++

	chunk.textures = append(chunk.textures, key)
	chunk.textureSet[key] = true
}
