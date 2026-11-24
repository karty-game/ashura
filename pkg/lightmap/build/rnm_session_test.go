//go:build !js

//nolint:varnamelen // HTTP fixture recorders use conventional short names.
package lightmapbuild

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
	"github.com/karty-game/ashura/pkg/lightmap"
)

func TestRNMSessionRejectsInvalidStartup(t *testing.T) {
	t.Parallel()
	canceled, stop := context.WithCancel(t.Context())
	stop()

	if _, err := NewRNMSession(canceled, BrowserOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	//nolint:staticcheck // Nil contexts must fail admission without starting resources.
	if _, err := NewRNMSession(nil, BrowserOptions{}); err == nil {
		t.Fatal("accepted nil context")
	}

	if _, err := NewRNMSession(t.Context(), BrowserOptions{Executable: "/nonexistent/karty-rnm-browser"}); err == nil {
		t.Fatal("accepted missing browser")
	}

	var session *RNMSession
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRNMSessionCapabilityAndResultAdmission(t *testing.T) {
	t.Parallel()
	session := testSession(t)
	handler := session.handler("/capability/")

	request := func(method, path string, body []byte, adapter string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(body))
		r.Header.Set("X-Karty-Adapter", adapter)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		return w
	}
	if w := request(http.MethodGet, "/other/shader.wgsl", nil, ""); w.Code != http.StatusNotFound {
		t.Fatal(w.Code)
	}

	if w := request(http.MethodPost, "/capability/ready", nil, ""); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}

	if w := request(http.MethodPost, "/capability/ready", nil, "fixture hardware"); w.Code != http.StatusNoContent {
		t.Fatal(w.Code)
	}

	if w := request(http.MethodPost, "/capability/ready", nil, "fixture hardware"); w.Code != http.StatusConflict {
		t.Fatal(w.Code)
	}

	job := &rnmSessionJob{
		id:         "1",
		width:      1,
		height:     1,
		parameters: []byte{1},
		scene:      []byte{2},
		points:     []byte{3},
		outcomes:   make(chan browserOutcome, 1),
	}

	session.active = job
	session.jobs <- job

	next := request(http.MethodGet, "/capability/next", nil, "")

	var metadata struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(next.Body.Bytes(), &metadata); err != nil || metadata.ID != "1" {
		t.Fatal(next.Body.String(), err)
	}

	if w := request(
		http.MethodGet,
		"/capability/job/1/scene.bin",
		nil,
		"",
	); w.Code != http.StatusOK ||
		!bytes.Equal(w.Body.Bytes(), job.scene) {
		t.Fatal(w.Code, w.Body.String())
	}

	for _, path := range []string{"/capability/job/2/scene.bin", "/capability/job/1/../scene.bin", "/capability/job/1/private"} {
		if w := request(http.MethodGet, path, nil, ""); w.Code != http.StatusNotFound {
			t.Fatal(path, w.Code)
		}
	}

	for _, test := range []struct {
		body    []byte
		adapter string
	}{{make([]byte, 11), "fixture hardware"}, {make([]byte, 13), "fixture hardware"}, {make([]byte, 12), "other adapter"}} {
		if w := request(http.MethodPost, "/capability/job/1/result", test.body, test.adapter); w.Code != http.StatusBadRequest {
			t.Fatal(w.Code)
		}
	}

	if len(job.outcomes) != 0 {
		t.Fatal("invalid result became observable")
	}

	if w := request(http.MethodPost, "/capability/job/1/result", make([]byte, 12), "fixture hardware"); w.Code != http.StatusNoContent {
		t.Fatal(w.Code)
	}

	if w := request(http.MethodPost, "/capability/job/1/result", make([]byte, 12), "fixture hardware"); w.Code != http.StatusConflict {
		t.Fatal(w.Code)
	}

	if w := request(
		http.MethodPost,
		"/capability/error",
		[]byte(strings.Repeat("x", rnmSessionErrorBytes+1)),
		"",
	); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
}

func TestRNMSessionCancellationInvalidatesWorkerAndBusyRejects(t *testing.T) {
	t.Parallel()

	encoded, err := lightmapdata.Encode(lightmapfixture.RNMDocument())
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	session := testSession(t)
	session.bakeMu.Lock()
	_, err = session.Bake(t.Context(), asset, lightmap.RNMScene{})
	session.bakeMu.Unlock()

	if !errors.Is(err, errRNMSessionBusy) {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan error, 1)

	go func() { _, err := session.Bake(ctx, asset, lightmap.RNMScene{}); result <- err }()

	select {
	case <-session.jobs:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}

	cancel()

	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled job did not stop")
	}

	if _, err = session.Bake(t.Context(), asset, lightmap.RNMScene{}); !errors.Is(err, errRNMSession) {
		t.Fatal(err)
	}

	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
}

func testSession(t *testing.T) *RNMSession {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	session := &RNMSession{
		ctx:      ctx,
		cancel:   cancel,
		finished: make(chan struct{}),
		ready:    make(chan browserOutcome, 1),
		jobs:     make(chan *rnmSessionJob, 1),
	}

	t.Cleanup(func() { _ = session.Close() })

	return session
}

func TestRNMSessionFailureIncludesBrowserDiagnosticsAndClosesWorker(t *testing.T) {
	t.Parallel()
	session := testSession(t)
	session.logs = &browserLog{}
	_, _ = session.logs.Write([]byte("fixture GPU process watchdog\n"))

	failure := session.failureWithDiagnostics(errBrowserExecution)
	if !errors.Is(failure, errBrowserExecution) || !strings.Contains(failure.Error(), "fixture GPU process watchdog") || !session.closed {
		t.Fatal("session omitted crash diagnostics or kept a failed worker", failure)
	}
}
