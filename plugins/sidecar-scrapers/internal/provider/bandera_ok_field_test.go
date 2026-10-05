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

// The aggregator reports a failed lookup as HTTP 200 with {"ok":false}, not as
// a status code. Both /search and /content decoded the `ok` field and never
// looked at it, so an upstream failure was indistinguishable from a genuine
// zero-hit: the registry recorded a successful health streak, the empty result
// was cached, and GetPopular then fired a SECOND upstream call on its fallback
// path. Each test below fails if the ok check is removed.
func TestBanderaClient_OKFalseIsAnError(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok": false, "error": "source list is empty", "items": []}`))
		}))
		defer srv.Close()

		c := provider.NewBanderaClient(srv.URL, srv.Client())
		_, err := c.SearchWithMeta(context.Background(), "дюна", 0, 0)
		if err == nil {
			t.Fatal("expected an error for ok:false, got nil")
		}
		if !strings.Contains(err.Error(), "source list is empty") {
			t.Errorf("error should carry the upstream message, got %q", err)
		}
	})

	t.Run("search without an error message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": false, "items": []}`))
		}))
		defer srv.Close()

		c := provider.NewBanderaClient(srv.URL, srv.Client())
		if _, err := c.SearchWithMeta(context.Background(), "дюна", 0, 0); err == nil {
			t.Fatal("expected an error for ok:false with no message, got nil")
		}
	})

	t.Run("content", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": false, "error": "material not found"}`))
		}))
		defer srv.Close()

		c := provider.NewBanderaClient(srv.URL, srv.Client())
		_, err := c.GetContent(context.Background(), "uatut", json.RawMessage(`{"id":1}`))
		if err == nil {
			t.Fatal("expected an error for ok:false, got nil")
		}
		if !strings.Contains(err.Error(), "material not found") {
			t.Errorf("error should carry the upstream message, got %q", err)
		}
	})
}

// A true ok must still succeed, and the items must survive the check.
func TestBanderaClient_OKTrueStillSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/content") {
			_, _ = w.Write([]byte(`{"ok": true, "type": "series", "seasons": [{"title": 1}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok": true, "items": [{"source":"uakino","title":"Дюна","year":2021}]}`))
	}))
	defer srv.Close()

	c := provider.NewBanderaClient(srv.URL, srv.Client())

	searchResp, err := c.SearchWithMeta(context.Background(), "дюна", 0, 0)
	if err != nil {
		t.Fatalf("ok:true search returned error: %v", err)
	}
	if len(searchResp.Items) != 1 || searchResp.Items[0].Title != "Дюна" {
		t.Errorf("unexpected items: %+v", searchResp.Items)
	}

	contentResp, err := c.GetContent(context.Background(), "uakino", json.RawMessage(`{"id":1}`))
	if err != nil {
		t.Fatalf("ok:true content returned error: %v", err)
	}
	if contentResp.Type != "series" {
		t.Errorf("type = %q, want series", contentResp.Type)
	}
	if len(contentResp.Seasons) != 1 {
		t.Errorf("expected 1 season, got %d", len(contentResp.Seasons))
	}
}

// The point of the ok check is that an upstream failure is reported as a
// failure, not as a successful empty search. GetPopular still retries the same
// endpoint once with the serial constraint dropped — that retry is deliberate
// (the aggregator rejects some serial=1 queries) — but when BOTH attempts fail
// the error has to reach the caller, so the registry records it against provider
// health instead of recording a clean success. Before the ok check, ok:false
// read as "no items" and the caller saw success with an empty list.
func TestBanderaProvider_OKFalseSurfacesAsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/search") {
			_, _ = w.Write([]byte(`{"ok": false, "error": "upstream busy"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sources") {
			_, _ = w.Write([]byte(`{"ok": true, "sources": [{"key":"uakino","enabled":true,"can_search":true,"can_content":true,"can_stream":true,"stream_keys":["file"]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	p := provider.NewBanderaProviderWithConfig(srv.URL, "", srv.Client())
	items, err := p.GetPopular(context.Background(), "movie", 1)
	if err == nil {
		t.Fatal("expected an error so the failure is recorded against provider health, got nil (an ok:false upstream must not read as a successful empty search)")
	}
	if len(items) != 0 {
		t.Errorf("expected no items, got %d", len(items))
	}
}
