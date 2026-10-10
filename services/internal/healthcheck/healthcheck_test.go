package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunReturnsZeroWhenHealthy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if got := Run(srv.URL); got != 0 {
		t.Errorf("Run() = %d, want 0", got)
	}
}

func TestRunReturnsOneOnNonSuccessStatus(t *testing.T) {
	t.Parallel()

	for _, code := range []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		if got := Run(srv.URL); got != 1 {
			t.Errorf("Run() for status %d = %d, want 1", code, got)
		}
		srv.Close()
	}
}

func TestRunReturnsOneWhenUnreachable(t *testing.T) {
	t.Parallel()

	// Port 0 is never listening, so the request fails fast rather than hanging.
	if got := Run("http://127.0.0.1:0/healthz"); got != 1 {
		t.Errorf("Run() = %d, want 1", got)
	}
}
