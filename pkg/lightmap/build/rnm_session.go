//go:build !js

package lightmapbuild

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/karty-game/ashura/internal/lightmapkernel"
	"github.com/karty-game/ashura/pkg/lightmap"
	"github.com/karty-game/ashura/pkg/texture"
)

//go:embed rnm_session.js
var rnmSessionScript []byte

var (
	errRNMSession     = errors.New("RNM hardware session is closed or invalid")
	errRNMSessionBusy = errors.New("RNM hardware session already has an active job")
)

const rnmSessionJobTimeout = 2 * time.Minute

// RNMSession owns one hardware browser, GPU device and shared kernel pipelines
// across sequential offline jobs. Each job releases its buffers and textures
// after readback. The complete session is invalidated if an in-flight job is
// canceled, so stale GPU work can never publish into a subsequent job.
type RNMSession struct {
	mu        sync.Mutex
	bakeMu    sync.Mutex
	closeOnce sync.Once
	//nolint:containedctx // The owned worker lifetime must span sequential job contexts.
	ctx          context.Context
	cancel       context.CancelFunc
	server       *http.Server
	command      *exec.Cmd
	logs         *browserLog
	profile      string
	finished     chan struct{}
	processError error
	ready        chan browserOutcome
	jobs         chan *rnmSessionJob
	active       *rnmSessionJob
	adapter      string
	failure      error
	sequence     uint64
	closed       bool
	closeError   error
}

type rnmSessionJob struct {
	id                        string
	parameters, scene, points []byte
	geometricNormals          []byte
	pathTraced                bool
	width, height             uint32
	outcomes                  chan browserOutcome
}

// NewRNMSession starts one capability-token loopback executor and waits for
// hardware admission and compilation of the exact runtime RNM shader.
func NewRNMSession(ctx context.Context, options BrowserOptions) (*RNMSession, error) {
	if ctx == nil {
		return nil, errBrowserArguments
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	executable, err := findBrowser(options.Executable)
	if err != nil {
		return nil, err
	}

	lifetime, cancel := context.WithCancel(ctx)
	session := &RNMSession{
		ctx:      lifetime,
		cancel:   cancel,
		finished: make(chan struct{}),
		ready:    make(chan browserOutcome, 1),
		jobs:     make(chan *rnmSessionJob, 1),
	}

	url, err := session.startServer()
	if err != nil {
		cancel()

		return nil, err
	}

	if err = session.launch(executable, url); err != nil {
		_ = session.Close()

		return nil, err
	}

	timer := time.NewTimer(browserTransferTimeout)
	defer timer.Stop()

	select {
	case result := <-session.ready:
		if result.err != nil {
			return nil, session.failureWithDiagnostics(result.err)
		}
	case <-session.finished:
		err = session.exitError()
		_ = session.Close()

		return nil, err
	case <-lifetime.Done():
		_ = session.Close()

		return nil, ctx.Err()
	case <-timer.C:
		_ = session.Close()

		return nil, fmt.Errorf("RNM session startup timed out: %w", errBrowserExecution)
	}

	go session.watch()

	return session, nil
}

// Bake runs one bounded job. Concurrent jobs are rejected; callers may keep one
// session per explicit GPU worker. Duration excludes session startup/compilation.
func (session *RNMSession) Bake(ctx context.Context, asset *lightmap.Asset, scene lightmap.RNMScene) (RNMResult, error) {
	return session.bake(ctx, asset, scene, nil, nil)
}

// Close aborts pending work, closes the owned listener/process and removes only
// its private profile. It is safe to call repeatedly or concurrently with Bake.
func (session *RNMSession) Close() error {
	if session == nil {
		return nil
	}

	session.closeOnce.Do(func() {
		session.mu.Lock()
		session.closed = true
		session.active = nil
		session.mu.Unlock()

		if session.cancel != nil {
			session.cancel()
		}

		select {
		case <-session.jobs:
		default:
		}

		if session.server != nil {
			_ = session.server.Close()
		}

		if session.command != nil && session.command.Process != nil {
			_ = session.command.Process.Kill()
			select {
			case <-session.finished:
			case <-time.After(browserHeaderTimeout):
				session.closeError = fmt.Errorf("RNM browser shutdown timed out: %w", errBrowserExecution)
			}
		}

		if err := os.RemoveAll(session.profile); err != nil {
			session.closeError = errors.Join(session.closeError, err)
		}
	})

	return session.closeError
}

func (session *RNMSession) bake(
	ctx context.Context,
	asset *lightmap.Asset,
	scene lightmap.RNMScene,
	geometricNormals []byte,
	options *PathTraceOptions,
) (RNMResult, error) {
	if ctx == nil || session == nil || session.ctx == nil || asset == nil {
		return RNMResult{}, errBrowserArguments
	}

	if !session.bakeMu.TryLock() {
		return RNMResult{}, errRNMSessionBusy
	}
	defer session.bakeMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, rnmSessionJobTimeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return RNMResult{}, err
	}

	started := time.Now()

	job, scale, normalized, err := session.configuredJob(asset, scene, geometricNormals, options)
	if err != nil {
		return RNMResult{}, err
	}

	defer session.clearJob()

	select {
	case session.jobs <- job:
	case <-ctx.Done():
		return RNMResult{}, ctx.Err()
	case <-session.ctx.Done():
		return RNMResult{}, errRNMSession
	}

	select {
	case outcome := <-job.outcomes:
		if outcome.err != nil {
			return RNMResult{}, session.failureWithDiagnostics(outcome.err)
		}

		result := session.result(asset, outcome, scale, normalized, time.Since(started))
		if job.pathTraced {
			result.Encoding = texture.RNMRGBE
		}

		return result, nil
	case <-session.finished:
		return RNMResult{}, session.exitError()
	case <-session.ctx.Done():
		return RNMResult{}, errRNMSession
	case <-ctx.Done():
		return RNMResult{}, session.failureWithDiagnostics(ctx.Err())
	}
}

// Stop the owned process before reading its capped stderr/stdout buffer. Wait
// joins the pipe writer, so diagnostics cannot race a browser write or omit the
// GPU-process crash reason behind a generic WebGPU device-lost notification.
func (session *RNMSession) failureWithDiagnostics(failure error) error {
	_ = session.Close()
	if session.logs == nil {
		return failure
	}

	return fmt.Errorf("%w\nbrowser diagnostics:\n%s", failure, session.logs.String())
}

func (session *RNMSession) clearJob() {
	session.mu.Lock()
	session.active = nil
	closed := session.closed
	session.mu.Unlock()

	if closed {
		select {
		case <-session.jobs:
		default:
		}
	}
}

func (session *RNMSession) watch() {
	select {
	case <-session.ctx.Done():
	case <-session.finished:
	}

	_ = session.Close()
}

func (session *RNMSession) exitError() error {
	return fmt.Errorf("RNM hardware browser exited: %s: %w", session.logs.String(), errors.Join(errBrowserExecution, session.processError))
}

func (session *RNMSession) newJob(asset *lightmap.Asset, parameters, points []byte) (*rnmSessionJob, error) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.closed || session.failure != nil {
		if session.failure != nil {
			return nil, session.failure
		}

		return nil, errRNMSession
	}

	session.sequence++
	width, height := asset.Size()
	job := &rnmSessionJob{
		id:         strconv.FormatUint(session.sequence, 10),
		parameters: parameters,
		points:     points,
		scene:      asset.Document().GPUData,
		width:      width,
		height:     height,
		outcomes:   make(chan browserOutcome, 1),
	}
	session.active = job

	return job, nil
}

func (session *RNMSession) result(
	asset *lightmap.Asset,
	outcome browserOutcome,
	scale float32,
	normalized lightmap.RNMScene,
	duration time.Duration,
) RNMResult {
	result := RNMResult{
		Scale:    scale,
		Adapter:  session.adapter,
		Duration: duration,
		Samples:  normalized.Samples,
		Bounces:  normalized.Bounces,
	}
	if asset.Document().Transport() {
		result.Encoding = texture.RNMRGBE
	}

	width, height := asset.Size()

	const pixelBytes = 4

	size := int(width) * int(height) * pixelBytes
	for index := range result.Textures {
		result.Textures[index] = texture.Data{
			Width:      int(width),
			Height:     int(height),
			ColorSpace: texture.Linear,
			Levels:     [][]byte{outcome.pixels[index*size : (index+1)*size]},
		}
	}

	return result
}

func (session *RNMSession) startServer() (string, error) {
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}

	prefix := "/" + hex.EncodeToString(token[:]) + "/"

	listener, err := (&net.ListenConfig{}).Listen(session.ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	session.server = &http.Server{
		Handler:           session.handler(prefix),
		ReadHeaderTimeout: browserHeaderTimeout,
		ReadTimeout:       browserTransferTimeout,
		WriteTimeout:      browserTransferTimeout,
		MaxHeaderBytes:    browserHeaderBytes,
	}
	startBrowserServer(session.server, listener)

	return "http://" + listener.Addr().String() + prefix, nil
}

func (session *RNMSession) launch(executable, url string) error {
	var err error

	session.profile, err = os.MkdirTemp("", "karty-rnm-session-")
	if err != nil {
		return err
	}

	session.command, session.logs, err = startBrowser(session.ctx, executable, session.profile, url)
	if err != nil {
		return err
	}

	go func() { session.processError = session.command.Wait(); close(session.finished) }()

	return nil
}

func (session *RNMSession) configuredJob(
	asset *lightmap.Asset,
	scene lightmap.RNMScene,
	geometricNormals []byte,
	options *PathTraceOptions,
) (*rnmSessionJob, float32, lightmap.RNMScene, error) {
	var parameters [lightmapkernel.RNMUniformSize]byte

	var points []byte

	var scale float32

	var normalized lightmap.RNMScene

	var err error
	if options == nil {
		parameters, points, scale, normalized, err = lightmapkernel.RNMParameters(asset.Document(), scene)
	} else {
		parameters, points, scale, normalized, err = pathTraceParameters(asset, geometricNormals, scene, *options)
	}

	if err != nil {
		return nil, 0, scene, err
	}

	job, err := session.newJob(asset, parameters[:], points)
	if err != nil {
		return nil, 0, scene, err
	}

	job.geometricNormals = geometricNormals
	job.pathTraced = options != nil

	return job, scale, normalized, nil
}
