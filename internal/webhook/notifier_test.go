package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestServer returns a test server that captures every received JSON body
// on the returned channel. Tests read from ch with a short timeout.
func newTestServer(t *testing.T) (*httptest.Server, <-chan map[string]interface{}) {
	t.Helper()
	ch := make(chan map[string]interface{}, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Errorf("unmarshal body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ch <- msg
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func TestNewNotifier_EmptyURLReturnsNil(t *testing.T) {
	assert.Nil(t, NewNotifier("", []string{"team.started"}))
}

func TestNilNotifier_SendEventIsNoOp(t *testing.T) {
	// Unconditionally calling SendEvent when no webhook is configured must not
	// panic — controller code relies on this.
	var n *Notifier
	require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))
}

func TestSendEvent_PostsEnvelope(t *testing.T) {
	srv, ch := newTestServer(t)
	n := NewNotifier(srv.URL, []string{"team.started"})
	require.NotNil(t, n)

	err := n.SendEvent(context.Background(), "team.started", map[string]interface{}{
		"team":      "auth-refactor",
		"namespace": "dev",
		"data":      map[string]interface{}{"lead": "alice"},
	})
	require.NoError(t, err)

	n.inFlight.Wait()

	select {
	case got := <-ch:
		assert.Equal(t, "team.started", got["event"])
		assert.Equal(t, "auth-refactor", got["team"])
		assert.Equal(t, "dev", got["namespace"])
		assert.NotEmpty(t, got["timestamp"], "timestamp must be set by notifier")
		// Timestamp is RFC3339 parseable.
		_, err := time.Parse(time.RFC3339, got["timestamp"].(string))
		assert.NoError(t, err, "timestamp must be RFC3339")
		// Event-specific data round-trips under `data`.
		data, ok := got["data"].(map[string]interface{})
		if assert.True(t, ok) {
			assert.Equal(t, "alice", data["lead"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook POST")
	}
}

func TestSendEvent_FiltersUnsubscribedEvents(t *testing.T) {
	srv, ch := newTestServer(t)
	n := NewNotifier(srv.URL, []string{"team.completed"})

	// team.started is not in the subscription list.
	require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))
	n.inFlight.Wait()

	select {
	case got := <-ch:
		t.Fatalf("unexpected POST for unsubscribed event: %v", got)
	case <-time.After(100 * time.Millisecond):
		// Expected — no delivery.
	}

	// A subscribed event still fires.
	require.NoError(t, n.SendEvent(context.Background(), "team.completed", map[string]interface{}{"team": "x"}))
	n.inFlight.Wait()
	select {
	case got := <-ch:
		assert.Equal(t, "team.completed", got["event"])
	case <-time.After(2 * time.Second):
		t.Fatal("subscribed event did not fire")
	}
}

func TestSendEvent_IsNonBlocking(t *testing.T) {
	// A slow webhook must not stall the caller. Stand up a server that sleeps
	// for longer than a reasonable reconcile and verify SendEvent returns
	// immediately.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slow.Close)

	n := NewNotifier(slow.URL, []string{"team.started"})
	start := time.Now()
	require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "SendEvent returned in %v; must not block on slow webhook", elapsed)
}

func TestSendEvent_SlowWebhookTimesOut(t *testing.T) {
	// Background delivery bounds each POST by DefaultTimeout. A hang longer
	// than that must terminate without leaking the goroutine indefinitely —
	// inFlight.Wait() proves the goroutine exits.
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(DefaultTimeout + 2*time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hang.Close)

	n := NewNotifier(hang.URL, []string{"team.started"})
	require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))

	done := make(chan struct{})
	go func() { n.inFlight.Wait(); close(done) }()
	select {
	case <-done:
		// Good — goroutine exited via timeout rather than waiting on the server.
	case <-time.After(DefaultTimeout + 3*time.Second):
		t.Fatal("background goroutine did not exit after webhook timeout")
	}
}

func TestSendEvent_ServerErrorsAreLoggedNotReturned(t *testing.T) {
	// Non-2xx responses are not programmer errors; they're log-worthy but must
	// not propagate back to the reconciler as errors.
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(fail.Close)

	n := NewNotifier(fail.URL, []string{"team.started"})
	require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))
	n.inFlight.Wait()
}

func TestSendEvent_ConcurrentDeliveries(t *testing.T) {
	// Multiple events in flight must all arrive. Exercises the inFlight
	// WaitGroup under load.
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := NewNotifier(srv.URL, []string{"team.started"})
	const N = 20
	for i := 0; i < N; i++ {
		require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))
	}
	n.inFlight.Wait()
	assert.Equal(t, int32(N), atomic.LoadInt32(&count), "all concurrent POSTs should arrive")
}

func TestSendEvent_ContentTypeIsJSON(t *testing.T) {
	ct := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct <- r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := NewNotifier(srv.URL, []string{"team.started"})
	require.NoError(t, n.SendEvent(context.Background(), "team.started", map[string]interface{}{"team": "x"}))
	n.inFlight.Wait()

	select {
	case got := <-ct:
		assert.Equal(t, "application/json", got)
	case <-time.After(2 * time.Second):
		t.Fatal("no request received")
	}
}

func TestSendEvent_UnmarshalableDataReturnsError(t *testing.T) {
	srv, _ := newTestServer(t)
	n := NewNotifier(srv.URL, []string{"team.started"})

	// A channel cannot be marshaled to JSON — this is a synchronous error.
	err := n.SendEvent(context.Background(), "team.started", map[string]interface{}{
		"bad": make(chan int),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}
