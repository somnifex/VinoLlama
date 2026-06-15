package llamacpp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPHealthCheckerUsesHealthEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHTTPHealthChecker("/health")
	if err := checker.Check(context.Background(), server.URL); err != nil {
		t.Fatalf("Check() = %v, want nil", err)
	}
}

func TestHTTPHealthCheckerFallsBackToTCPWhenEndpointMissing(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	checker := NewHTTPHealthChecker("")
	if err := checker.Check(context.Background(), server.URL); err != nil {
		t.Fatalf("Check() = %v, want nil through TCP fallback", err)
	}
}

func TestReadyWaiterTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	waiter := PollReadyWaiter{
		Checker:  NewHTTPHealthChecker("/health"),
		Interval: 10 * time.Millisecond,
		Timeout:  30 * time.Millisecond,
	}
	if err := waiter.WaitReady(context.Background(), server.URL); err == nil {
		t.Fatal("expected ready timeout")
	}
}
