package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

// ---- SSRF: редиректи в бік внутрішніх адрес ----

func TestClientGetRefusesRedirectToMetadataService(t *testing.T) {
	// Публічний-looking редирект на cloud metadata: без перевірки хопів
	// клієнт повернув би тіло метаданих клієнту (read SSRF).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	c, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	body, err := c.Get(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatalf("expected the redirect to the metadata service to be refused, got body %q", body)
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("expected a redirect-related error, got %v", err)
	}
}

func TestClientGetRefusesRedirectToLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:22/", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	c, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	if _, err := c.Get(context.Background(), srv.URL, ""); err == nil {
		t.Fatal("expected the redirect to loopback to be refused")
	}
}

func TestClientGetRefusesRedirectToPrivateNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.5/admin", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	c, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	if _, err := c.Get(context.Background(), srv.URL, ""); err == nil {
		t.Fatal("expected the redirect to a private address to be refused")
	}
}

func TestClientPostFormRefusesRedirectToPrivateNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.168.1.1/", http.StatusFound)
	}))
	defer srv.Close()

	c, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	if _, err := c.PostForm(context.Background(), srv.URL, "a=1", ""); err == nil {
		t.Fatal("expected the redirect to a private address to be refused")
	}
}

func TestClientGetStopsAfterRedirectBudget(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	defer srv.Close()

	req, err := fhttp.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	via := make([]*fhttp.Request, maxRedirects)
	if err := validateRedirectHop(req, via); err == nil {
		t.Fatal("expected the redirect budget to be enforced")
	}
}

func TestValidateUpstreamURL(t *testing.T) {
	cases := []struct {
		raw   string
		valid bool
	}{
		{raw: "http://169.254.169.254/", valid: false},
		{raw: "http://127.0.0.1/", valid: false},
		{raw: "http://10.0.0.5/", valid: false},
		{raw: "http://192.168.0.1/", valid: false},
		{raw: "http://0.0.0.0/", valid: false},
		{raw: "http://[::1]/", valid: false},
		{raw: "http://[fd00::1]/", valid: false},
		{raw: "http://100.64.0.1/", valid: false},
		{raw: "http://localhost/", valid: false},
		{raw: "http://db.internal/", valid: false},
		{raw: "http://metadata.google.internal/", valid: false},
		{raw: "file:///etc/passwd", valid: false},
		{raw: "gopher://127.0.0.1:6379", valid: false},
		{raw: "https://93.184.216.34/x", valid: true},
		{raw: "http://8.8.8.8/", valid: true},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.raw, err)
		}
		err = validateUpstreamURL(u)
		if (err == nil) != tc.valid {
			t.Errorf("validateUpstreamURL(%q) = %v, want valid=%v", tc.raw, err, tc.valid)
		}
	}
}

func TestUpstreamBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "0.0.0.0", "::", "10.1.2.3", "172.16.0.1",
		"192.168.1.1", "169.254.169.254", "fe80::1", "100.64.0.1", "fd00::1",
		"224.0.0.1", "255.255.255.255",
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("bad test IP %q", raw)
		}
		if !upstreamBlockedIP(ip) {
			t.Errorf("expected %s to be blocked", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "93.184.216.34", "1.1.1.1", "2606:2800:220:1:248:1893:25c8:1946"} {
		if upstreamBlockedIP(net.ParseIP(raw)) {
			t.Errorf("expected %s to be allowed", raw)
		}
	}
	if !upstreamBlockedIP(nil) {
		t.Error("a nil IP must be treated as blocked")
	}
}

// ---- стеля розміру тіла відповіді ----

func TestClientGetRejectsOversizedBody(t *testing.T) {
	const payloadSize = MaxUpstreamBodyBytes + (1 << 20)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		chunk := bytes.Repeat([]byte("A"), 64<<10)
		written := 0
		for written < payloadSize {
			n, err := w.Write(chunk)
			written += n
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	body, err := c.Get(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatalf("expected an error for a %d byte body, got %d bytes back", payloadSize, len(body))
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected the error to mention the size limit, got %v", err)
	}
	if len(body) > MaxUpstreamBodyBytes {
		t.Errorf("a partial body of %d bytes was returned despite the limit", len(body))
	}
}

func TestReadLimitedBodyRejectsOverflow(t *testing.T) {
	huge := io.LimitReader(zeroReader{}, MaxUpstreamBodyBytes+(1<<20))
	if _, err := readLimitedBody(huge); err == nil {
		t.Fatal("expected readLimitedBody to reject an oversized stream")
	}

	exact := io.LimitReader(zeroReader{}, MaxUpstreamBodyBytes)
	body, err := readLimitedBody(exact)
	if err != nil {
		t.Fatalf("a body of exactly the limit must be accepted, got %v", err)
	}
	if len(body) != MaxUpstreamBodyBytes {
		t.Fatalf("expected %d bytes, got %d", MaxUpstreamBodyBytes, len(body))
	}
}

func TestDecodeBodyBoundsTranscodedOutput(t *testing.T) {
	// UTF-16 -> UTF-8 майже подвоює тіло (2 байти на символ), тому щоб
	// результат перевищив ліміт, вхід має бути близько 2xMaxUpstreamBodyBytes.
	const units = MaxUpstreamBodyBytes + 1024
	var buf bytes.Buffer
	buf.Grow(units*2 + 2)
	buf.Write([]byte{0xFF, 0xFE}) // BOM: UTF-16LE
	unit := []byte{'A', 0x00}
	for i := 0; i < units; i++ {
		buf.Write(unit)
	}

	if _, err := decodeBody("text/html; charset=utf-16", buf.Bytes()); err == nil {
		t.Fatal("expected decodeBody to enforce the size limit on its output")
	}

	// Невелике тіло транскодується без помилки. Декодер x/text зберігає BOM
	// як U+FEFF, тому перевіряємо суфікс, а не точний рядок.
	small := append([]byte{0xFF, 0xFE}, append(unit, unit...)...)
	out, err := decodeBody("text/html; charset=utf-16", small)
	if err != nil {
		t.Fatalf("small UTF-16 body must transcode cleanly, got %v", err)
	}
	if !strings.HasSuffix(out, "AA") {
		t.Errorf("expected the body to decode to ...AA, got %q", out)
	}
}

func TestDecodeBodyPassesThroughUnknownCharset(t *testing.T) {
	out, err := decodeBody("text/html; charset=definitely-not-a-charset", []byte("raw"))
	if err != nil {
		t.Fatalf("unknown charset must not break the request: %v", err)
	}
	if out != "raw" {
		t.Errorf("expected the raw bytes back, got %q", out)
	}
}

// zeroReader — нескінченний потік нульових байт (джерело великих тіл).
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestMaxUpstreamBodyBytesIsBounded(t *testing.T) {
	// Константа має залишатися в межах, придатних для 256 MiB контейнера.
	if MaxUpstreamBodyBytes <= 0 || MaxUpstreamBodyBytes > 16<<20 {
		t.Fatalf("MaxUpstreamBodyBytes = %d is outside the reviewed range", MaxUpstreamBodyBytes)
	}
	fmt.Fprintf(io.Discard, "%d", MaxUpstreamBodyBytes)
}
