package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCovMigrateFetchSuccess(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if !strings.HasSuffix(r.URL.Path, "/api/collections/users/records") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("perPage"); got != "500" {
			t.Errorf("unexpected perPage: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"u1","email":"a@x.com","username":"ann","avatar":"av1","bio":"b1"},
			{"id":"u2","email":"b@x.com","username":"bob","avatar":"","bio":""}
		]}`))
	}))
	defer srv.Close()

	users, err := fetchPBRecords[PBUserRecord](srv.URL, "users", "admintoken123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %+v", users)
	}
	if users[0].Email != "a@x.com" || users[0].Username != "ann" || users[0].Avatar != "av1" {
		t.Errorf("unexpected first user: %+v", users[0])
	}
	if gotAuth != "admintoken123" {
		t.Errorf("expected Authorization header passthrough, got %q", gotAuth)
	}
}

func TestCovMigrateFetchNoToken(t *testing.T) {
	var gotAuth, present string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, present = r.Header.Get("Authorization"), "set"
		if gotAuth == "" {
			present = "empty"
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	items, err := fetchPBRecords[PBUserRecord](srv.URL, "users", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty items, got %+v", items)
	}
	if present != "empty" {
		t.Errorf("expected no Authorization header without token, got %q", gotAuth)
	}
}

func TestCovMigrateFetchStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`token required`))
	}))
	defer srv.Close()

	_, err := fetchPBRecords[PBUserRecord](srv.URL, "users", "")
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Errorf("expected status 403 error, got %v", err)
	}
}

func TestCovMigrateFetchInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{broken`))
	}))
	defer srv.Close()

	_, err := fetchPBRecords[PBUserRecord](srv.URL, "users", "")
	if err == nil {
		t.Error("expected JSON decode error, got nil")
	}
}

func TestCovMigrateFetchUnreachable(t *testing.T) {
	// Порт 1 гарантовано відхиляє з'єднання — швидко, без таймаутів.
	_, err := fetchPBRecords[PBUserRecord]("http://127.0.0.1:1", "users", "")
	if err == nil {
		t.Error("expected transport error, got nil")
	}
}

func TestCovMigrateFetchBadURL(t *testing.T) {
	_, err := fetchPBRecords[PBUserRecord]("://bad-url", "users", "")
	if err == nil {
		t.Error("expected request-build error, got nil")
	}
}
