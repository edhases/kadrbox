package provider

// Error-гілки fetch-then-parse методів: скасований контекст роняє
// транспорт до парсингу, тож покриваємо обгортки помилок без мережі.

import (
	"context"
	"strings"
	"testing"
)

func TestCovProviderTransportErrorPaths(t *testing.T) {
	tls := covTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // транспорт гарантовано падає

	uakino := &UakinoProvider{client: tls, baseURL: "http://127.0.0.1:1"}
	eneyida := &EneyidaProvider{client: tls, baseURL: "http://127.0.0.1:1"}
	hdrezka := &HdrezkaProvider{client: tls, baseURL: "http://127.0.0.1:1"}

	cases := []struct {
		name string
		call func(context.Context) error
		want string
	}{
		{"uakino search", func(c context.Context) error { _, e := uakino.Search(c, "x"); return e }, "uakino search request"},
		{"uakino details", func(c context.Context) error { _, e := uakino.GetDetails(c, "u"); return e }, "uakino get details"},
		{"uakino streams", func(c context.Context) error { _, e := uakino.GetStreams(c, "u", 0, 0, ""); return e }, "uakino get streams html"},
		{"eneyida search", func(c context.Context) error { _, e := eneyida.Search(c, "x"); return e }, "eneyida search error"},
		{"eneyida details", func(c context.Context) error { _, e := eneyida.GetDetails(c, "u"); return e }, "eneyida get details"},
		{"eneyida streams", func(c context.Context) error { _, e := eneyida.GetStreams(c, "u", 0, 0, ""); return e }, "eneyida get streams"},
		{"hdrezka search", func(c context.Context) error { _, e := hdrezka.Search(c, "x"); return e }, "hdrezka search request"},
		{"hdrezka details", func(c context.Context) error { _, e := hdrezka.GetDetails(c, "u"); return e }, "hdrezka get details"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(ctx)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected wrapped %q error, got %v", tc.want, err)
			}
		})
	}
}
