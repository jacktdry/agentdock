package browser

import (
	"context"
	"crypto/rand"
	"errors"
	"reflect"
	"sync"
	"time"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type WorkerState string

const (
	WorkerStarting   WorkerState = "starting"
	WorkerValidating WorkerState = "validating"
	WorkerReady      WorkerState = "ready"
	WorkerStopping   WorkerState = "stopping"
	WorkerStopped    WorkerState = "stopped"
	WorkerFailed     WorkerState = "failed"
)

type WorkerInfo struct {
	WorkerID      string
	CreatedAt     time.Time
	State         WorkerState
	Session       mcpclient.SessionInfo
	Compatibility ManagedCompatibility
	Error         string
}
type WorkerSession interface {
	Info() mcpclient.SessionInfo
	Tools() map[string]mcpclient.Tool
	Call(context.Context, string, map[string]any) (map[string]any, error)
	Close() error
}
type WorkerDependencies struct {
	Probe VersionProbe
	Open  func(context.Context, mcpclient.ServerConfig) (WorkerSession, error)
}
type ManagedWorkerOptions struct{ Cwd string }

type managedWorker struct {
	mu        sync.Mutex
	info      WorkerInfo
	session   WorkerSession
	cancel    context.CancelFunc
	startDone chan struct{}
	stopDone  chan struct{}
	stopOnce  sync.Once
	stopErr   error
}

// WorkerRegistry owns ephemeral workers. This substrate is not a bounded pool,
// queue or concurrency scheduler. Call is internal, never a public MCP tool.
type WorkerRegistry struct {
	mu      sync.Mutex
	workers map[string]*managedWorker
	closed  bool
	deps    WorkerDependencies
}

var _ EngineLifecycle = (*WorkerRegistry)(nil)

func NewWorkerRegistry(deps WorkerDependencies) *WorkerRegistry {
	if deps.Probe == nil {
		deps.Probe = ProbeManagedVersion
	}
	if deps.Open == nil {
		deps.Open = func(ctx context.Context, cfg mcpclient.ServerConfig) (WorkerSession, error) {
			session, err := mcpclient.OpenStdioSession(ctx, cfg)
			if session == nil {
				return nil, err
			}
			return session, err
		}
	}
	return &WorkerRegistry{workers: make(map[string]*managedWorker), deps: deps}
}

func workerSessionNil(session WorkerSession) bool {
	if session == nil {
		return true
	}
	v := reflect.ValueOf(session)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func workerError(code, id string, cause error) *Error {
	return browserError(code, "managed browser worker unavailable", "worker", &ErrorDetails{WorkerID: id}, cause)
}

func (r *WorkerRegistry) StartManaged(ctx context.Context, options ManagedWorkerOptions) (WorkerInfo, error) {
	return r.startWorker(ctx, ManagedWorkerConfig(options.Cwd))
}

func (r *WorkerRegistry) startWorker(ctx context.Context, cfg mcpclient.ServerConfig) (WorkerInfo, error) {
	startTimeout := 30 * time.Second
	if configured := time.Duration(cfg.TimeoutMS) * time.Millisecond; configured >= startTimeout {
		startTimeout = configured + 5*time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	w := &managedWorker{info: WorkerInfo{WorkerID: rand.Text(), CreatedAt: time.Now().UTC(), State: WorkerStarting}, cancel: cancel, startDone: make(chan struct{}), stopDone: make(chan struct{})}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return WorkerInfo{}, workerError(ErrWorkerNotReady, "", errors.New("registry shut down"))
	}
	r.workers[w.info.WorkerID] = w
	r.mu.Unlock()
	defer close(w.startDone)
	fail := func(err error) (WorkerInfo, error) {
		if !workerSessionNil(w.session) {
			err = errors.Join(err, w.session.Close())
		}
		w.mu.Lock()
		w.info.State = WorkerFailed
		w.info.Error = err.Error()
		info := w.info
		w.mu.Unlock()
		return info, err
	}
	version, err := r.deps.Probe(ctx, cfg)
	if err != nil {
		return fail(workerError(ErrEngineUnavailable, w.info.WorkerID, err))
	}
	if err := validateManagedVersion(version); err != nil {
		return fail(err)
	}
	session, err := r.deps.Open(ctx, cfg)
	w.mu.Lock()
	w.session = session
	w.mu.Unlock()
	if err != nil {
		var mcpErr *mcpclient.Error
		code := ErrEngineUnavailable
		if errors.As(err, &mcpErr) && mcpErr.Code == "MCP_SCHEMA_INVALID" {
			code = ErrEngineSchemaIncompatible
		}
		return fail(workerError(code, w.info.WorkerID, err))
	}
	if workerSessionNil(session) {
		return fail(workerError(ErrEngineUnavailable, w.info.WorkerID, errors.New("session missing")))
	}
	info := session.Info()
	w.mu.Lock()
	w.info.State = WorkerValidating
	w.info.Session = info
	w.mu.Unlock()
	if info.ServerName != ManagedEngineServerName || info.ServerVersion != ManagedEngineVersion {
		return fail(browserError(ErrEngineVersionIncompatible, "managed browser handshake identity incompatible", "validation", &ErrorDetails{WorkerID: w.info.WorkerID, EngineVersion: info.ServerVersion, Reason: info.ServerName}, nil))
	}
	compatibility, err := ValidateManagedTools(session.Tools())
	if err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(workerError(ErrEngineUnavailable, w.info.WorkerID, err))
	}
	w.mu.Lock()
	w.info.State = WorkerReady
	w.info.Compatibility = compatibility
	result := w.info
	w.mu.Unlock()
	return result, nil
}

func (r *WorkerRegistry) worker(id string) (*managedWorker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.workers[id]
	if w == nil {
		return nil, workerError(ErrWorkerNotFound, id, nil)
	}
	return w, nil
}
func (r *WorkerRegistry) Info(id string) (WorkerInfo, error) {
	w, err := r.worker(id)
	if err != nil {
		return WorkerInfo{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.info, nil
}

func (r *WorkerRegistry) Call(ctx context.Context, id, tool string, args map[string]any) (map[string]any, error) {
	w, err := r.worker(id)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	state, session := w.info.State, w.session
	w.mu.Unlock()
	if state != WorkerReady {
		return nil, workerError(ErrWorkerNotReady, id, nil)
	}
	// Only this step's validated tools can be called. In particular select_page
	// cannot create shared selected-page state, even if a server advertises it.
	switch tool {
	case "list_pages", "new_page", "close_page", "navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key":
	default:
		return nil, workerError(ErrEngineSchemaIncompatible, id, errors.New("tool outside validated contract"))
	}
	return session.Call(ctx, tool, args)
}

func (r *WorkerRegistry) stop(ctx context.Context, w *managedWorker) error {
	w.cancel()
	w.stopOnce.Do(func() {
		go func() {
			<-w.startDone
			w.mu.Lock()
			failed := w.info.State == WorkerFailed
			w.info.State = WorkerStopping
			session := w.session
			w.mu.Unlock()
			var err error
			if !workerSessionNil(session) {
				err = session.Close()
			}
			w.mu.Lock()
			w.stopErr = err
			if err != nil {
				w.info.State = WorkerFailed
				w.info.Error = err.Error()
			} else if failed {
				w.info.State = WorkerFailed
			} else {
				w.info.State = WorkerStopped
			}
			w.mu.Unlock()
			close(w.stopDone)
		}()
	})
	select {
	case <-w.stopDone:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.stopErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (r *WorkerRegistry) Stop(id string) error {
	w, err := r.worker(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return r.stop(ctx, w)
}
func (r *WorkerRegistry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	workers := make([]*managedWorker, 0, len(r.workers))
	for _, w := range r.workers {
		workers = append(workers, w)
	}
	r.mu.Unlock()
	for _, w := range workers {
		w.cancel()
	}
	var result error
	for _, w := range workers {
		result = errors.Join(result, r.stop(ctx, w))
	}
	return result
}
