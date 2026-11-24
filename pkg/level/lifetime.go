package level

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/texture"
)

func (chunk *LoadedChunk) inUse() bool {
	if chunk.loader.app.Closed() {
		return false
	}

	for instance := range chunk.instances {
		if instance.presenting {
			return true
		}

		for _, item := range instance.models {
			if item.Len() > 0 {
				return true
			}
		}
	}

	return false
}

// Unload rejects live instances and atomically admits the texture-release batch
// before changing reference counts. Other chunks retain shared texture handles.
func (chunk *LoadedChunk) Unload() error {
	if chunk == nil || chunk.closed {
		return nil
	}

	if chunk.inUse() {
		return errInUse
	}

	var release []*texture.Texture

	for _, key := range chunk.textures {
		shared := chunk.loader.textures[key]
		if shared.references == 1 {
			release = append(release, shared.image)
		}
	}

	if !chunk.loader.app.Closed() {
		if err := chunk.loader.app.UnloadTextures(release); err != nil {
			return fmt.Errorf("chunk texture release: %w", err)
		}
	}

	for instance := range chunk.instances {
		instance.Remove()
	}
	// Geometry-only models have no embedded textures; the live-instance preflight
	// above makes their resource release independent of shared Source materials.
	for _, page := range chunk.pages {
		if err := page.model.Unload(); err != nil {
			return err
		}
	}

	for _, key := range chunk.textures {
		shared := chunk.loader.textures[key]

		shared.references--
		if shared.references == 0 {
			delete(chunk.loader.textures, key)
		}
	}

	delete(chunk.loader.chunks, chunk)
	chunk.closed = true
	chunk.pages = nil
	chunk.textures = nil
	chunk.textureSet = nil
	chunk.instances = nil

	return nil
}

// Close releases resident chunks after all presentations have been removed.
// App.Close invokes it after marking the App closed, forcing presentation cleanup.
func (loader *Loader) Close() error {
	if loader == nil || loader.closed {
		return nil
	}

	for chunk := range loader.chunks {
		if chunk.inUse() {
			return errInUse
		}
	}

	for task := range loader.tasks {
		if err := task.Cancel(); err != nil {
			return err
		}
	}

	for chunk := range loader.chunks {
		if err := chunk.Unload(); err != nil {
			return err
		}
	}

	loader.closed = true
	if loader.decoder != nil {
		loader.decoder.Close()
	}

	if loader.cancel != nil {
		loader.cancel()
		loader.cancel = nil
	}

	return nil
}
