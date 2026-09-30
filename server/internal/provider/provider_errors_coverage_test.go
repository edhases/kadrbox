package provider

// Error-гілки fetch-then-parse методів: скасований контекст роняє
// транспорт до парсингу, тож покриваємо обгортки помилок без мережі.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
)

func TestCovProviderTransportErrorPaths(t *testing.T) {
	tls := covTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // транспорт гарантовано падає

	uakino := &UakinoProvider{client: tls, baseURL: "http://127.0.0.1:1"}
	eneyida := &EneyidaProvider{client: tls, baseURL: "http://127.0.0.1:1"}

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

// Сторінка плеєра без розпізнаного медіа: кожен із трьох DLE-провайдерів має
// повернути порожній список + ErrUnresolvablePlayer, а НЕ URL iframe-сторінки
// (HTML, який libmpv не демодулює — «Failed to recognize file format.»).
func TestCovProvidersUnresolvablePlayerPath(t *testing.T) {
	tls := covTLS(t)

	itemHTML := `<html><body>
<iframe src="about:blank"></iframe>
<iframe src="https://www.google.com/recaptcha/api2/anchor"></iframe>
<iframe src="https://ad.doubleclick.net/ads/creative.html"></iframe>
<iframe src="/player/dead"></iframe>
</body></html>`
	deadPlayerHTML := `<html><body><div id="player"></div><script>var cfg={};</script></body></html>`
	// Плеєр, який сам повертає лише URL сторінки — найгірший випадок.
	playerPageHTML := `<html><body><script>new Playerjs({file:"https://player.example.com/embed/4242"});</script></body></html>`

	srv := covFixtureServer(map[string]string{
		"/item/1":      itemHTML,
		"/player/dead": deadPlayerHTML,
		"/item/2":      `<html><body><iframe src="/player/page"></iframe></body></html>`,
		"/player/page": playerPageHTML,
	})
	defer srv.Close()

	uakino := &UakinoProvider{client: tls, baseURL: srv.URL}
	eneyida := &EneyidaProvider{client: tls, baseURL: srv.URL}
	lavakino := &LavakinoProvider{client: tls, baseURL: srv.URL}

	providers := []struct {
		name string
		call func(ctx context.Context, itemURL string) (*domain.ContentStreamsResponse, error)
	}{
		{"uakino", func(ctx context.Context, itemURL string) (*domain.ContentStreamsResponse, error) {
			return uakino.GetStreams(ctx, itemURL, 0, 0, "")
		}},
		{"eneyida", func(ctx context.Context, itemURL string) (*domain.ContentStreamsResponse, error) {
			return eneyida.GetStreams(ctx, itemURL, 0, 0, "")
		}},
		{"lavakino", func(ctx context.Context, itemURL string) (*domain.ContentStreamsResponse, error) {
			return lavakino.GetStreams(ctx, itemURL, 0, 0, "")
		}},
	}

	for _, p := range providers {
		p := p
		t.Run(p.name+" player without media", func(t *testing.T) {
			resp, err := p.call(context.Background(), srv.URL+"/item/1")
			if !errors.Is(err, ErrUnresolvablePlayer) {
				t.Fatalf("expected ErrUnresolvablePlayer, got %v", err)
			}
			assertNoHTMLStreams(t, resp)
		})

		// item/2: плеєр віддає лише посилання на власну сторінку (без медіа-розширення).
		t.Run(p.name+" player page fallback rejected", func(t *testing.T) {
			resp, err := p.call(context.Background(), srv.URL+"/item/2")
			if !errors.Is(err, ErrUnresolvablePlayer) {
				t.Fatalf("expected ErrUnresolvablePlayer, got %v", err)
			}
			assertNoHTMLStreams(t, resp)
		})
	}
}

func assertNoHTMLStreams(t *testing.T, resp *domain.ContentStreamsResponse) {
	t.Helper()
	if resp == nil {
		return
	}
	for _, st := range resp.Streams {
		if !rePlayableURL.MatchString(st.URL) {
			t.Fatalf("non-playable stream url: %q", st.URL)
		}
	}
	if len(resp.Streams) != 0 {
		t.Fatalf("expected no streams, got %+v", resp.Streams)
	}
}
