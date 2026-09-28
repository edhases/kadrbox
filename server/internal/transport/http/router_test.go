package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

func TestRouterHealthEndpoint(t *testing.T) {
	reg := provider.NewRegistry()
	hub := ws.NewHub(nil)
	contentH := transporthttp.NewContentHandler(reg, nil)
	authH := transporthttp.NewAuthHandler(nil, nil, "secret")
	syncH := transporthttp.NewSyncHandler(nil, nil)

	router := transporthttp.NewRouter("secret", authH, contentH, syncH, hub)

	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" || resp["service"] != "oxide-server" {
		t.Errorf("unexpected body content: %v", resp)
	}
}

func TestRouterCORSHeaders(t *testing.T) {
	reg := provider.NewRegistry()
	hub := ws.NewHub(nil)
	contentH := transporthttp.NewContentHandler(reg, nil)
	authH := transporthttp.NewAuthHandler(nil, nil, "secret")
	syncH := transporthttp.NewSyncHandler(nil, nil)

	router := transporthttp.NewRouter("secret", authH, contentH, syncH, hub)

	req, _ := http.NewRequest("OPTIONS", "/api/v1/content/search", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
		t.Errorf("expected 200 or 204 for OPTIONS preflight, got %d", rr.Code)
	}

	allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" && allowOrigin != "http://localhost:3000" {
		t.Errorf("expected Allow-Origin header, got %s", allowOrigin)
	}
}
