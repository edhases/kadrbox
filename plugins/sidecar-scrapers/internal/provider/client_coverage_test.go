package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

type covEchoHeaders struct {
	Referer string `json:"referer"`
	UA      string `json:"ua"`
	Accept  string `json:"accept"`
}

func TestCovClientGetReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"referer": r.Header.Get("Referer"),
			"ua":      r.Header.Get("User-Agent"),
			"accept":  r.Header.Get("Accept"),
		})
	}))
	defer srv.Close()

	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}

	body, err := c.Get(context.Background(), srv.URL, "https://ref.example/")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	var got covEchoHeaders
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("response is not JSON echo: %v (body=%q)", err, body)
	}
	if got.Referer != "https://ref.example/" {
		t.Errorf("expected Referer %q, got %q", "https://ref.example/", got.Referer)
	}
	if !strings.Contains(got.UA, "Chrome/120") {
		t.Errorf("expected User-Agent to contain Chrome/120, got %q", got.UA)
	}
	if got.Accept == "" {
		t.Errorf("expected non-empty Accept header, got empty")
	}
}

func TestCovClientGetWithoutReferer(t *testing.T) {
	var gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}

	body, err := c.Get(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if body != "ok" {
		t.Errorf("expected body %q, got %q", "ok", body)
	}
	if gotReferer != "" {
		t.Errorf("expected no Referer header, got %q", gotReferer)
	}
}

func TestCovClientGetFailsOnServerErrorStatus(t *testing.T) {
	// Перевірка: client.go повертає помилку на 500 status code (захист від Cloudflare/upstream error).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}

	_, err = c.Get(context.Background(), srv.URL, "https://ref.example/")
	if err == nil {
		t.Fatalf("expected error on 500 status, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}

func TestCovClientGetInvalidURL(t *testing.T) {
	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}

	if _, err := c.Get(context.Background(), "://bad-url", ""); err == nil {
		t.Errorf("expected error for invalid URL, got nil")
	}
}

func TestCovClientPostFormSendsForm(t *testing.T) {
	var gotCT, gotXRW, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotXRW = r.Header.Get("X-Requested-With")
		buf := make([]byte, r.ContentLength)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		_, _ = w.Write([]byte("ok-post"))
	}))
	defer srv.Close()

	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}

	resp, err := c.PostForm(context.Background(), srv.URL, "a=1&b=2", srv.URL)
	if err != nil {
		t.Fatalf("PostForm failed: %v", err)
	}
	if resp != "ok-post" {
		t.Errorf("expected body %q, got %q", "ok-post", resp)
	}
	if !strings.Contains(gotCT, "x-www-form-urlencoded") {
		t.Errorf("expected Content-Type to contain x-www-form-urlencoded, got %q", gotCT)
	}
	if gotXRW != "XMLHttpRequest" {
		t.Errorf("expected X-Requested-With XMLHttpRequest, got %q", gotXRW)
	}
	if gotBody != "a=1&b=2" {
		t.Errorf("expected echoed body %q, got %q", "a=1&b=2", gotBody)
	}
}

func TestCovClientPostFormCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.PostForm(ctx, srv.URL, "a=1", ""); err == nil {
		t.Errorf("expected error for cancelled context, got nil")
	}
}

func TestCovClientNewSucceeds(t *testing.T) {
	c, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient failed: %v", err)
	}
	if c == nil {
		t.Fatalf("expected non-nil client")
	}
}
