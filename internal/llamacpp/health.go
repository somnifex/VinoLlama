package llamacpp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ReadyWaiter interface {
	WaitReady(ctx context.Context, baseURL string) error
}

type HealthChecker interface {
	Check(ctx context.Context, baseURL string) error
}

type HTTPHealthChecker struct {
	Client     *http.Client
	HealthPath string
}

func NewHTTPHealthChecker(healthPath string) HTTPHealthChecker {
	return HTTPHealthChecker{
		Client:     &http.Client{Timeout: 500 * time.Millisecond},
		HealthPath: healthPath,
	}
}

func (h HTTPHealthChecker) Check(ctx context.Context, baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 500 * time.Millisecond}
	}

	var paths []string
	if strings.TrimSpace(h.HealthPath) != "" {
		paths = append(paths, h.HealthPath)
	}
	paths = append(paths, "/health", "/")

	var lastErr error
	concreteHTTPFailure := false
	for _, path := range paths {
		checkURL := *parsed
		checkURL.Path = path
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL.String(), nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("%s returned 404", checkURL.String())
			continue
		}
		lastErr = fmt.Errorf("%s returned HTTP %d", checkURL.String(), resp.StatusCode)
		concreteHTTPFailure = true
	}
	if concreteHTTPFailure {
		return lastErr
	}
	if err := tcpCheck(ctx, parsed); err == nil {
		return nil
	} else if lastErr == nil {
		lastErr = err
	}
	return lastErr
}

type PollReadyWaiter struct {
	Checker  HealthChecker
	Interval time.Duration
	Timeout  time.Duration
}

func NewReadyWaiter(healthPath string, timeout time.Duration) PollReadyWaiter {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return PollReadyWaiter{
		Checker:  NewHTTPHealthChecker(healthPath),
		Interval: 100 * time.Millisecond,
		Timeout:  timeout,
	}
}

func (w PollReadyWaiter) WaitReady(ctx context.Context, baseURL string) error {
	checker := w.Checker
	if checker == nil {
		checker = NewHTTPHealthChecker("")
	}
	interval := w.Interval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	for {
		if err := checker.Check(readyCtx, baseURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
		timer := time.NewTimer(interval)
		select {
		case <-readyCtx.Done():
			timer.Stop()
			return ActionableError{
				What:    "llama.cpp server did not become ready.",
				Reason:  fmt.Sprintf("%v; last check: %v", readyCtx.Err(), lastErr),
				Fix:     "Check the model path, backend binary, internal port, and llama.cpp stderr log.",
				Details: fmt.Sprintf("base_url=%s", baseURL),
			}
		case <-timer.C:
		}
	}
}

func tcpCheck(ctx context.Context, parsed *url.URL) error {
	host := parsed.Host
	if host == "" {
		return fmt.Errorf("base URL has no host")
	}
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
