package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// get issues a request against a fresh mux and returns the recorder.
func get(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestLivenessReturnsOKWithNoDependency(t *testing.T) {
	t.Parallel()

	// nil stands in for a service with nothing to check. Liveness must not depend on it.
	rec := get(t, New(nil), pathLiveness)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q, want %q", got, "{\"status\":\"ok\"}\n")
	}
	if got := rec.Header().Get(contentTypeHeader); got != contentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", got, contentTypeJSON)
	}
}

func TestReadinessReturnsOKWhenCheckPasses(t *testing.T) {
	t.Parallel()

	rec := get(t, New(func(context.Context) error { return nil }), pathReadiness)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadinessReturnsServiceUnavailableWhenCheckFails(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("nats: no connection")
	rec := get(t, New(func(context.Context) error { return wantErr }), pathReadiness)

	if rec.Code != statusServiceUnavl {
		t.Errorf("status = %d, want %d", rec.Code, statusServiceUnavl)
	}
	// The body must not claim ok beside a 503.
	var got Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body %q: %v", rec.Body.String(), err)
	}
	if got.Status == statusOK {
		t.Errorf("body reports %q alongside a 503", got.Status)
	}
}

func TestReadinessReturnsServiceUnavailableWhenContextCancelled(t *testing.T) {
	t.Parallel()

	// The check would return nil. The cancellation must still produce a 503, so a
	// dependency that stops responding cannot hold the probe open past its deadline.
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, pathReadiness, nil).WithContext(ctx)
	cancel()

	New(func(context.Context) error { return nil }).ServeHTTP(rec, req)

	if rec.Code != statusServiceUnavl {
		t.Errorf("status = %d, want %d", rec.Code, statusServiceUnavl)
	}
}

func TestReadinessReturnsOKWithNilCheck(t *testing.T) {
	t.Parallel()

	if rec := get(t, New(nil), pathReadiness); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLivenessDoesNotConsultTheReadinessCheck(t *testing.T) {
	t.Parallel()

	// A failing dependency must not make liveness fail, or a dependency blip would
	// restart every replica instead of just removing it from the load balancer.
	called := false
	rec := get(t, New(func(context.Context) error {
		called = true
		return errors.New("down")
	}), pathLiveness)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if called {
		t.Error("liveness consulted the readiness check")
	}
}

func TestUnknownPathReturnsNotFound(t *testing.T) {
	t.Parallel()

	if rec := get(t, New(nil), "/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandlersAnswerAnyMethod(t *testing.T) {
	t.Parallel()

	mux := New(nil)
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, pathLiveness, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s = %d, want %d", method, pathLiveness, rec.Code, http.StatusOK)
		}
	}
}

func TestExportedHandlersMatchTheMux(t *testing.T) {
	t.Parallel()

	// A service that mounts its own routes must get the same answers.
	rec := httptest.NewRecorder()
	LivenessHandler(rec, httptest.NewRequest(http.MethodGet, pathLiveness, nil))
	if rec.Code != http.StatusOK {
		t.Errorf("LivenessHandler status = %d, want %d", rec.Code, http.StatusOK)
	}

	rec = httptest.NewRecorder()
	ReadinessHandler(func(context.Context) error { return errors.New("down") })(rec,
		httptest.NewRequest(http.MethodGet, pathReadiness, nil))
	if rec.Code != statusServiceUnavl {
		t.Errorf("ReadinessHandler status = %d, want %d", rec.Code, statusServiceUnavl)
	}
}
