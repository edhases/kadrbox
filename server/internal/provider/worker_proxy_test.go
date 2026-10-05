package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerProxy_Get_Success(t *testing.T) {
	secret := "test-secret"
	targetURL := "https://example.com/movie"

	var receivedSecret, receivedTarget, receivedTTL string
	var requestsCount int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestsCount, 1)
		receivedSecret = r.Header.Get("X-Proxy-Secret")
		receivedTarget = r.Header.Get("X-Target-URL")
		receivedTTL = r.Header.Get("X-Cache-TTL")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	client, err := NewTLSClient(WithWorkerProxy(ts.URL, secret, 50*time.Millisecond))
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	body, err := client.Get(context.Background(), targetURL, "https://referer.com")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if body != "<html><body>OK</body></html>" {
		t.Errorf("unexpected body: %q", body)
	}
	if receivedSecret != secret {
		t.Errorf("expected secret %q, got %q", secret, receivedSecret)
	}
	if receivedTarget != targetURL {
		t.Errorf("expected target %q, got %q", targetURL, receivedTarget)
	}
	if receivedTTL != "900" {
		t.Errorf("expected TTL 900, got %q", receivedTTL)
	}
}

func TestWorkerProxy_Get_Redirect(t *testing.T) {
	secret := "test-secret"
	firstTarget := "https://example.com/first"
	secondTarget := "https://example.com/second"

	var hops []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.Header.Get("X-Target-URL")
		hops = append(hops, target)

		if target == firstTarget {
			w.Header().Set("Location", secondTarget)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Final</body></html>"))
	}))
	defer ts.Close()

	client, err := NewTLSClient(WithWorkerProxy(ts.URL, secret, 10*time.Millisecond))
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	body, err := client.Get(context.Background(), firstTarget, "")
	if err != nil {
		t.Fatalf("Get with redirect: %v", err)
	}

	if body != "<html><body>Final</body></html>" {
		t.Errorf("unexpected body: %q", body)
	}
	if len(hops) != 2 || hops[0] != firstTarget || hops[1] != secondTarget {
		t.Errorf("expected hops [%s, %s], got %v", firstTarget, secondTarget, hops)
	}
}

func TestWorkerProxy_PostForm_Success(t *testing.T) {
	secret := "test-secret"
	targetURL := "https://example.com/search"

	var receivedBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client, err := NewTLSClient(WithWorkerProxy(ts.URL, secret, 10*time.Millisecond))
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	body, err := client.PostForm(context.Background(), targetURL, "q=test&page=1", "")
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}

	if body != `{"status":"ok"}` {
		t.Errorf("unexpected body: %q", body)
	}
	if receivedBody != "q=test&page=1" {
		t.Errorf("expected form body 'q=test&page=1', got %q", receivedBody)
	}
}

func TestLiveWorkerProxy_HTTPBin(t *testing.T) {
	workerURL := os.Getenv("WORKER_PROXY_URL")
	secret := os.Getenv("WORKER_PROXY_SECRET")
	if workerURL == "" || secret == "" {
		t.Skip("skipping live worker test: WORKER_PROXY_URL and WORKER_PROXY_SECRET not set")
	}

	client, err := NewTLSClient(WithWorkerProxy(workerURL, secret, 150*time.Millisecond))
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	body, err := client.Get(context.Background(), "https://httpbin.org/get", "")
	if err != nil {
		t.Fatalf("Live Get via worker failed: %v", err)
	}

	if len(body) == 0 {
		t.Fatal("empty response from live worker")
	}
}
