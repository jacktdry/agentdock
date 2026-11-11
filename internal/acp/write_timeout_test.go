package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

type startedACPWriter struct {
	io.WriteCloser
	started chan struct{}
}

func (w *startedACPWriter) Write(data []byte) (int, error) {
	w.started <- struct{}{}
	return w.WriteCloser.Write(data)
}

func newBlockedWriteConnection(t *testing.T) (*Connection, *os.File, <-chan struct{}) {
	t.Helper()
	reader, peerWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peerWriter.Close(); _ = reader.Close() })
	peerReader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peerReader.Close(); _ = writer.Close() })
	started := &startedACPWriter{WriteCloser: writer, started: make(chan struct{}, 16)}
	connection := NewConnection(reader, started, nil, nil)
	t.Cleanup(func() { _ = connection.Close() })
	return connection, peerReader, started.started
}

func awaitACPWrite(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("pipe write did not start")
	}
}

func awaitACPWriteResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("ACP operation remained blocked for more than one second")
		return nil
	}
}

func requireACPWriteError(t *testing.T, err error, code string, cause error) {
	t.Helper()
	var acpErr *Error
	if !errors.As(err, &acpErr) || acpErr.Code != code || (cause != nil && !errors.Is(err, cause)) {
		t.Fatalf("error = %v, want %s wrapping %v", err, code, cause)
	}
}

func requireNoPendingWrites(t *testing.T, c *Connection) {
	t.Helper()
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if len(c.pending) != 0 {
		t.Errorf("cancelled/failed requests left %d pending entries", len(c.pending))
	}
}

func TestBlockedWriteHonorsRequestContext(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[timeout], func(t *testing.T) {
			connection, _, started := newBlockedWriteConnection(t)
			prompt := strings.Repeat("x", 1<<17)
			ctx, cancel := context.WithCancel(context.Background())
			cause := context.Canceled
			if timeout {
				cancel()
				// Leave room for race-instrumented JSON encoding and scheduling;
				// the started signal still proves the deadline interrupts Write.
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
				cause = context.DeadlineExceeded
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- connection.Request(ctx, "session/prompt", prompt, nil)
			}()
			awaitACPWrite(t, started)
			if !timeout {
				cancel()
			}
			<-ctx.Done()
			requireACPWriteError(t, awaitACPWriteResult(t, done), "ACP_REQUEST_CANCELLED", cause)
			select {
			case <-connection.Closed():
			default:
				t.Fatal("possibly partial JSON-RPC frame left connection reusable")
			}
			requireNoPendingWrites(t, connection)
		})
	}
}

func TestQueuedWriteTimeoutPreservesActiveWrite(t *testing.T) {
	connection, peerReader, started := newBlockedWriteConnection(t)
	first := make(chan error, 1)
	go func() { first <- connection.Notify("large", strings.Repeat("x", 1<<17)) }()
	awaitACPWrite(t, started)
	// Confirm the active frame still requires peer drainage. The fixture must
	// not silently become a fully buffered write on another platform.
	select {
	case err := <-first:
		t.Fatalf("active frame finished before queued request: %v", err)
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() { second <- connection.Request(ctx, "queued", nil, nil) }()
	requireACPWriteError(t, awaitACPWriteResult(t, second), "ACP_REQUEST_CANCELLED", context.DeadlineExceeded)
	// Queued cancellation must not unblock or truncate the active frame.
	select {
	case err := <-first:
		t.Fatalf("active frame finished without peer draining: %v", err)
	default:
	}
	select {
	case <-connection.Closed():
		t.Fatal("queued cancellation invalidated another active write")
	default:
	}
	requireNoPendingWrites(t, connection)
	// Drain only after the queued caller has returned. The active frame must
	// still arrive intact, followed by a usable connection with no queued frame.
	decoded := make(chan rpcMessage, 2)
	decodeErrors := make(chan error, 1)
	go func() {
		decoder := json.NewDecoder(peerReader)
		for i := 0; i < 2; i++ {
			var message rpcMessage
			if err := decoder.Decode(&message); err != nil {
				decodeErrors <- err
				return
			}
			decoded <- message
		}
	}()
	if err := awaitACPWriteResult(t, first); err != nil {
		t.Fatalf("active write failed after queued cancellation: %v", err)
	}
	if err := connection.Notify("healthy", nil); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"large", "healthy"} {
		select {
		case message := <-decoded:
			if message.Method != method {
				t.Fatalf("received method %q, want %q", message.Method, method)
			}
		case err := <-decodeErrors:
			t.Fatal(err)
		case <-time.After(time.Second):
			t.Fatal("peer did not receive complete JSON-RPC frame")
		}
	}
}

func TestAlreadyCancelledWriteDoesNotTouchHealthyConnection(t *testing.T) {
	connection, _, started := newBlockedWriteConnection(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 20; i++ {
		requireACPWriteError(t, connection.Request(ctx, "cancelled", nil, nil), "ACP_REQUEST_CANCELLED", context.Canceled)
	}
	select {
	case <-started:
		t.Fatal("already cancelled request reached writer")
	case <-connection.Closed():
		t.Fatal("already cancelled request closed healthy connection")
	default:
	}
	requireNoPendingWrites(t, connection)
}

type failingACPWriter struct {
	io.WriteCloser
	partial     bool
	err         error
	beforeWrite func()
}

func (w failingACPWriter) Write(data []byte) (int, error) {
	if w.beforeWrite != nil {
		w.beforeWrite()
	}
	if w.partial {
		n, err := w.WriteCloser.Write(data[:3])
		if err != nil {
			return n, err
		}
		return n, w.err
	}
	return 0, w.err
}

func TestWriteFailureClosesFramingAndKeepsClassification(t *testing.T) {
	for _, test := range []struct {
		name    string
		partial bool
		err     error
		cause   error
	}{
		{name: "failure", err: os.ErrClosed, cause: os.ErrClosed},
		{name: "partial failure", partial: true, err: os.ErrClosed, cause: os.ErrClosed},
		{name: "short write", partial: true, cause: io.ErrShortWrite},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, writer, cleanup := pipeConnectionPair(t)
			defer cleanup()
			connection := NewConnection(reader, failingACPWriter{WriteCloser: writer, partial: test.partial, err: test.err}, nil, nil)
			defer connection.Close()
			err := connection.Request(context.Background(), "broken", nil, nil)
			requireACPWriteError(t, err, "ACP_CONNECTION_FAILED", test.cause)
			select {
			case <-connection.Closed():
			default:
				t.Fatal("failed frame left connection reusable")
			}
			requireACPWriteError(t, connection.Notify("later", nil), "ACP_CONNECTION_CLOSED", nil)
			requireNoPendingWrites(t, connection)
		})
	}
}

func TestCloseReleasesBlockedAndQueuedWrites(t *testing.T) {
	connection, _, started := newBlockedWriteConnection(t)
	active := make(chan error, 1)
	go func() { active <- connection.Notify("large", strings.Repeat("x", 1<<20)) }()
	awaitACPWrite(t, started)
	queued := make(chan error, 1)
	go func() { queued <- connection.Request(context.Background(), "queued", nil, nil) }()
	closed := make(chan error, 1)
	go func() { closed <- connection.Close() }()
	if err := awaitACPWriteResult(t, closed); err != nil {
		t.Fatal(err)
	}
	requireACPWriteError(t, awaitACPWriteResult(t, active), "ACP_CONNECTION_CLOSED", nil)
	requireACPWriteError(t, awaitACPWriteResult(t, queued), "ACP_CONNECTION_CLOSED", nil)
	requireNoPendingWrites(t, connection)

	t.Run("before write context cancellation", func(t *testing.T) {
		reader, writer, cleanup := pipeConnectionPair(t)
		defer cleanup()
		started := make(chan struct{})
		releaseCtx, release := context.WithCancel(context.Background())
		defer release()
		connection := NewConnection(reader, failingACPWriter{
			WriteCloser: writer,
			err:         os.ErrClosed,
			beforeWrite: func() { close(started); <-releaseCtx.Done() },
		}, nil, nil)
		// Hold back cancellation propagation to deterministically exercise the
		// gap after Closed is published but before the write context is cancelled.
		cancel := connection.cancel
		defer cancel()
		connection.cancel = func() {}
		defer connection.Close()
		done := make(chan error, 1)
		go func() { done <- connection.Request(context.Background(), "closing", nil, nil) }()
		awaitACPWrite(t, started)
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
		release()
		requireACPWriteError(t, awaitACPWriteResult(t, done), "ACP_CONNECTION_CLOSED", nil)
		requireNoPendingWrites(t, connection)
	})
}

func TestCancelledRequestDoesNotWaitForCancelNotification(t *testing.T) {
	connection, peerReader, started := newBlockedWriteConnection(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := make(chan error, 1)
	go func() { request <- connection.Request(ctx, "session/prompt", nil, nil) }()
	awaitACPWrite(t, started)
	var message rpcMessage
	if err := json.NewDecoder(peerReader).Decode(&message); err != nil {
		t.Fatal(err)
	}
	// Occupy the writer while the first request waits for its response.
	active := make(chan error, 1)
	go func() { active <- connection.Notify("large", strings.Repeat("x", 1<<20)) }()
	awaitACPWrite(t, started)
	cancel()
	requireACPWriteError(t, awaitACPWriteResult(t, request), "ACP_REQUEST_CANCELLED", context.Canceled)
	_ = connection.Close()
	_ = awaitACPWriteResult(t, active)
	requireNoPendingWrites(t, connection)
}

func TestBlockedNotifyHonorsWriteDeadline(t *testing.T) {
	connection, _, started := newBlockedWriteConnection(t)
	if connection.writeTimeout != 30*time.Second {
		t.Fatalf("default write timeout = %v, want 30 seconds", connection.writeTimeout)
	}
	connection.writeTimeout = 50 * time.Millisecond
	before := time.Now()
	done := make(chan error, 1)
	go func() { done <- connection.Notify("large", strings.Repeat("x", 1<<20)) }()
	awaitACPWrite(t, started)
	select {
	case err := <-done:
		requireACPWriteError(t, err, "ACP_REQUEST_CANCELLED", context.DeadlineExceeded)
		if elapsed := time.Since(before); elapsed < connection.writeTimeout {
			t.Fatalf("write deadline took %v, want at least %v", elapsed, connection.writeTimeout)
		}
	case <-time.After(time.Second):
		t.Fatal("Notify ignored the injected 50ms write deadline")
	}
}
