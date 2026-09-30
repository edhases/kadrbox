package http_test

import (
	"errors"
	"testing"

	transportHttp "github.com/edhases/oxide-server/internal/transport/http"
)

func TestValidateSafeURL(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		expectError bool
		targetErr   error
	}{
		// Безпечні посилання
		{name: "safe absolute https", raw: "https://uakino.me/film/123-dune.html", expectError: false},
		{name: "safe relative path", raw: "/serials/gra-v-kalmara.html", expectError: false},
		{name: "safe json payload", raw: `{"href":"https://uaflix.net/video/123"}`, expectError: false},
		{name: "safe json id ref", raw: `{"id":12345}`, expectError: false},

		// SSRF атаки (IP)
		{name: "block 127.0.0.1", raw: "http://127.0.0.1:8080/admin", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block localhost", raw: "http://localhost:5432/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block private 192.168", raw: "http://192.168.1.1/router", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block private 10.x", raw: "http://10.0.0.5/secrets", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block AWS metadata", raw: "http://169.254.169.254/latest/meta-data/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block 0.0.0.0", raw: "http://0.0.0.0:80/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block internal domain", raw: "http://service.internal/api", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},

		// Небезпечні схеми
		{name: "block file://", raw: "file:///etc/passwd", expectError: true, targetErr: transportHttp.ErrUnsafeURLScheme},
		{name: "block gopher://", raw: "gopher://127.0.0.1:6379", expectError: true, targetErr: transportHttp.ErrUnsafeURLScheme},
		{name: "block ftp://", raw: "ftp://user:pass@example.com", expectError: true, targetErr: transportHttp.ErrUnsafeURLScheme},

		// SSRF у JSON payload
		{name: "block ssrf in json href", raw: `{"href":"http://127.0.0.1:8000/internal"}`, expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := transportHttp.ValidateSafeURL(tc.raw)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.raw)
				}
				if tc.targetErr != nil && !errors.Is(err, tc.targetErr) {
					t.Fatalf("expected target error %v, got %v", tc.targetErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.raw, err)
				}
			}
		})
	}
}
