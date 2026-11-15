//go:build !js

package lightmapbuild

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
)

const (
	browserShaderName      = "shader.wgsl"
	browserParametersName  = "parameters.bin"
	rnmSessionErrorBytes   = 8192
	rnmSessionAdapterBytes = 4096
)

func (session *RNMSession) handler(prefix string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, prefix) {
			http.NotFound(writer, request)

			return
		}

		name := strings.TrimPrefix(request.URL.Path, prefix)

		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")

		switch {
		case request.Method == http.MethodGet && name == "":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(
				writer,
				"<!doctype html><meta charset=utf-8><title>Karty RNM session</title><script src=\"session.js\" defer></script>",
			)
		case request.Method == http.MethodGet && name == "session.js":
			writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = writer.Write(rnmSessionScript)
		case request.Method == http.MethodGet && name == browserShaderName:
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(writer, lightmapkernel.RNMSource)
		case request.Method == http.MethodGet && name == "pathtrace.wgsl":
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(writer, lightmapkernel.RNMPathTraceSource())
		case request.Method == http.MethodGet && name == "next":
			session.nextJob(writer, request)
		case request.Method == http.MethodPost && name == "ready":
			session.acceptReady(writer, request)
		case request.Method == http.MethodPost && name == browserErrorRoute:
			session.acceptError(writer, request)
		case strings.HasPrefix(name, "job/"):
			session.serveJob(writer, request, strings.TrimPrefix(name, "job/"))
		default:
			http.NotFound(writer, request)
		}
	})
}

func (session *RNMSession) nextJob(writer http.ResponseWriter, request *http.Request) {
	timer := time.NewTimer(browserHeaderTimeout)
	defer timer.Stop()

	select {
	case job := <-session.jobs:
		writer.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(writer).Encode(struct {
			ID         string `json:"id"`
			PathTraced bool   `json:"pathTraced,omitempty"`
		}{ID: job.id, PathTraced: job.pathTraced}); err != nil {
			return
		}
	case <-timer.C:
		writer.WriteHeader(http.StatusNoContent)
	case <-request.Context().Done():
	case <-session.ctx.Done():
		writer.WriteHeader(http.StatusGone)
	}
}

func (session *RNMSession) acceptReady(writer http.ResponseWriter, request *http.Request) {
	adapter := request.Header.Get("X-Karty-Adapter")
	if adapter == "" || len(adapter) > rnmSessionAdapterBytes {
		http.Error(writer, "invalid adapter", http.StatusBadRequest)

		return
	}

	session.mu.Lock()
	if session.adapter != "" || session.closed || session.failure != nil {
		session.mu.Unlock()
		http.Error(writer, "session unavailable", http.StatusConflict)

		return
	}

	session.adapter = adapter
	session.mu.Unlock()

	select {
	case session.ready <- browserOutcome{adapter: adapter}:
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.Error(writer, "duplicate ready", http.StatusConflict)
	}
}

func (session *RNMSession) acceptError(writer http.ResponseWriter, request *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, rnmSessionErrorBytes))
	if err != nil {
		http.Error(writer, "invalid error size", http.StatusBadRequest)

		return
	}

	failure := fmt.Errorf("%w: %s", errBrowserExecution, data)

	session.mu.Lock()
	if session.failure == nil {
		session.failure = failure
	}

	job := session.active
	session.mu.Unlock()

	outcome := browserOutcome{err: failure}
	select {
	case session.ready <- outcome:
	default:
	}

	if job != nil {
		select {
		case job.outcomes <- outcome:
		default:
		}
	}

	writer.WriteHeader(http.StatusNoContent)
}

func (session *RNMSession) serveJob(writer http.ResponseWriter, request *http.Request, name string) {
	identity, file, ok := strings.Cut(name, "/")
	if !ok {
		http.NotFound(writer, request)

		return
	}

	session.mu.Lock()
	job := session.active
	adapter := session.adapter
	session.mu.Unlock()

	if job == nil || identity != job.id {
		http.NotFound(writer, request)

		return
	}

	if request.Method == http.MethodPost && file == "result" {
		session.acceptResult(writer, request, job, adapter)

		return
	}

	if request.Method != http.MethodGet {
		http.NotFound(writer, request)

		return
	}

	resources := map[string][]byte{browserParametersName: job.parameters, "scene.bin": job.scene, "points.bin": job.points}
	if job.pathTraced {
		resources["geometric.bin"] = job.geometricNormals
	}

	data, exists := resources[file]
	if !exists {
		http.NotFound(writer, request)

		return
	}

	writer.Header().Set("Content-Type", "application/octet-stream")
	_, _ = writer.Write(data)
}

func (*RNMSession) acceptResult(writer http.ResponseWriter, request *http.Request, job *rnmSessionJob, adapter string) {
	const coefficientPixelBytes = 12

	size := int(job.width) * int(job.height) * coefficientPixelBytes

	data, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, int64(size)))
	if err != nil || len(data) != size || request.Header.Get("X-Karty-Adapter") != adapter {
		http.Error(writer, "invalid RNM job result", http.StatusBadRequest)

		return
	}

	select {
	case job.outcomes <- browserOutcome{pixels: data, adapter: adapter}:
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.Error(writer, "duplicate RNM job result", http.StatusConflict)
	}
}
