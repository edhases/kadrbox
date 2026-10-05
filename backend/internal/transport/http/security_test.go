package http_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	transportHttp "github.com/edhases/kadrbox-server/internal/transport/http"
)

// ---- мінімальний DNS-сервер для детермінованого тесту resolve-then-check ----
//

type fakeDNS struct {
	t    *testing.T
	conn *net.UDPConn

	mu       sync.Mutex
	zone     map[string][]net.IP
	hits     map[string]int
	total    int
	resolver *net.Resolver
}

func startFakeDNS(t *testing.T, zone map[string][]net.IP) *fakeDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	f := &fakeDNS{t: t, conn: conn, zone: zone, hits: map[string]int{}}
	go f.serve()
	t.Cleanup(func() { _ = conn.Close() })

	f.resolver = &net.Resolver{
		PreferGo:     true,
		StrictErrors: false,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", conn.LocalAddr().String())
		},
	}
	return f
}

func (f *fakeDNS) serve() {
	buf := make([]byte, 1500)
	for {
		n, addr, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if resp := f.respond(buf[:n]); resp != nil {
			_, _ = f.conn.WriteToUDP(resp, addr)
		}
	}
}

func (f *fakeDNS) respond(q []byte) []byte {
	if len(q) < 12 {
		return nil
	}
	name, end, ok := parseQName(q, 12)
	if !ok {
		return nil
	}
	qtype := uint16(q[end])<<8 | uint16(q[end+1])

	f.mu.Lock()
	f.hits[name]++
	f.total++
	ips := append([]net.IP(nil), f.zone[name]...)
	f.mu.Unlock()

	question := append([]byte(nil), q[12:end+4]...)

	out := make([]byte, 0, 512)
	out = append(out, q[0], q[1], 0x81, 0x80) // QR=1, RD=1, RA=1, RCODE=0
	out = append(out, 0x00, 0x01)             // QDCOUNT
	out = append(out, uint8(len(ips)>>8), uint8(len(ips)))
	out = append(out, 0x00, 0x00, 0x00, 0x00)

	out = append(out, question...)
	for _, ip := range ips {
		v4 := ip.To4()
		if qtype == 1 && v4 == nil {
			continue
		}
		if qtype == 28 && v4 != nil {
			continue
		}
		// RR: NAME (pointer) | TYPE | CLASS | TTL | RDLENGTH | RDATA
		out = append(out, 0xc0, 0x0c)
		if v4 != nil {
			out = append(out, 0x00, 0x01, 0x00, 0x01) // TYPE=A, CLASS=IN
			out = append(out, 0x00, 0x00, 0x00, 0x3c) // TTL 60
			out = append(out, 0x00, 0x04)             // RDLENGTH
			out = append(out, v4...)
		} else {
			out = append(out, 0x00, 0x1c, 0x00, 0x01) // TYPE=AAAA, CLASS=IN
			out = append(out, 0x00, 0x00, 0x00, 0x3c)
			out = append(out, 0x00, 0x10)
			out = append(out, ip.To16()...)
		}
	}
	return out
}

func parseQName(msg []byte, off int) (string, int, bool) {
	var labels []string
	for off < len(msg) {
		length := int(msg[off])
		off++
		if length == 0 {
			break
		}
		if length&0xc0 != 0 || off+length > len(msg) {
			return "", 0, false
		}
		labels = append(labels, string(msg[off:off+length]))
		off += length
	}
	if off+4 > len(msg) {
		return "", 0, false
	}
	return strings.Join(labels, ".") + ".", off, true
}

// lookupCount рахує всі DNS-запити, які надійшли до фейкового сервера.
// Рахуємо загальну кількість, а не за ім'ям: резолвер може додавати
// search-домени, тому точний name у запиті залежить від resolv.conf.
func (f *fakeDNS) lookupCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.total
}

// ---- helpers ----

// resetSecurityState повертає глобальний стан валідатора у нейтральний:
// без allow-list, з системним резолвером і порожнім кешем.
func resetSecurityState(t *testing.T) {
	t.Helper()
	restore := func() {
		transportHttp.SetUpstreamHostAllowlist(nil)
		transportHttp.SetHostResolver(nil)
		transportHttp.ClearResolveCache()
	}
	restore()
	t.Cleanup(restore)
}

// maxURLLengthTest дублює ліміт із security.go: 2048 байт на «звичайний» URL.
const maxURLLengthTest = 2048

// ---- tests ----

func TestValidateSafeURL(t *testing.T) {
	resetSecurityState(t)

	// DNS вимкнено: публічні хости перевіряються літералами, імена — відкидаються.
	transportHttp.SetHostResolver(&net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dns disabled in test")
		},
	})

	tests := []struct {
		name        string
		raw         string
		expectError bool
		targetErr   error
	}{
		// Безпечні посилання
		{name: "safe public ip literal", raw: "https://93.184.216.34/film/123-dune.html", expectError: false},
		{name: "safe relative path", raw: "/serials/gra-v-kalmara.html", expectError: false},
		{name: "safe catalog url", raw: "https://catalog.example/film/tt0111161", expectError: false},

		// SSRF атаки (IP-літерали)
		{name: "block 127.0.0.1", raw: "http://127.0.0.1:8080/admin", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block localhost", raw: "http://localhost:5432/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block private 192.168", raw: "http://192.168.1.1/router", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block private 10.x", raw: "http://10.0.0.5/secrets", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block AWS metadata", raw: "http://169.254.169.254/latest/meta-data/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block 0.0.0.0", raw: "http://0.0.0.0:80/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block 127.0.0.1 bare", raw: "http://127.0.0.1/x", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block ipv6 loopback", raw: "http://[::1]/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block ipv6 unspecified", raw: "http://[::]/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block ipv6 ula", raw: "http://[fd00::1]/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block cgnat 100.64", raw: "http://100.64.0.1/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block internal domain", raw: "http://service.internal/api", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{name: "block protocol relative", raw: "//169.254.169.254/latest/meta-data/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},

		// Небезпечні схеми
		{name: "block file://", raw: "file:///etc/passwd", expectError: true, targetErr: transportHttp.ErrUnsafeURLScheme},
		{name: "block gopher://", raw: "gopher://127.0.0.1:6379", expectError: true, targetErr: transportHttp.ErrUnsafeURLScheme},
		{name: "block ftp://", raw: "ftp://user:pass@example.com", expectError: true, targetErr: transportHttp.ErrUnsafeURLScheme},

		// Розмір та керуючі символи
		{name: "block control char", raw: "https://catalog.example/a\r\nHost: evil", expectError: true, targetErr: transportHttp.ErrUnsafeCharInput},
		{name: "block nul byte", raw: "https://catalog.example/a\x00b", expectError: true, targetErr: transportHttp.ErrUnsafeCharInput},
		{name: "block oversize url", raw: "https://93.184.216.34/" + strings.Repeat("a", maxURLLengthTest), expectError: true, targetErr: transportHttp.ErrURLTooLong},
		{name: "block oversize relative path", raw: "/" + strings.Repeat("a", maxURLLengthTest), expectError: true, targetErr: transportHttp.ErrURLTooLong},

		// Рядки, які виглядають як конверт, але більше не приймаються: бекенд
		// не отримує контентних URL від клієнта, тому це вже не схема, а
		// просто некоректний URL.
		{name: "block json object", raw: `{"source":"catalog","ref":"x"}`, expectError: true},
		{name: "block json array", raw: `["http://93.184.216.34/"]`, expectError: true},
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
			} else if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
		})
	}
}

func TestValidateSafeURLRejectsOversizedInput(t *testing.T) {
	resetSecurityState(t)
	for _, raw := range []string{
		"https://93.184.216.34/" + strings.Repeat("a", 64<<10),
		"/" + strings.Repeat("a", 64<<10),
		// The envelope path is gone, so a JSON-looking body is measured by the
		// single URL cap like anything else — it must not be silently allowed
		// just because it is long and no longer parsed.
		`{"source":"catalog","ref":"` + strings.Repeat("a", 64<<10) + `"}`,
	} {
		err := transportHttp.ValidateSafeURL(raw)
		if !errors.Is(err, transportHttp.ErrURLTooLong) {
			t.Fatalf("expected ErrURLTooLong for a %d-byte input, got %v", len(raw), err)
		}
	}
}

// TestValidateSafeURLResolveThenCheck — ядро фіксу: hostname, який резолвиться
// у приватну адресу, більше не проходить лише тому, що не парситься як IP.
func TestValidateSafeURLResolveThenCheck(t *testing.T) {
	resetSecurityState(t)

	fake := startFakeDNS(t, map[string][]net.IP{
		"rebind.example.":   {net.ParseIP("169.254.169.254")},
		"private.example.":  {net.ParseIP("10.0.0.5")},
		"loopback.example.": {net.ParseIP("127.0.0.1")},
		"mixed.example.":    {net.ParseIP("93.184.216.34"), net.ParseIP("192.168.1.10")},
		"public.example.":   {net.ParseIP("93.184.216.34")},
		"ula.example.":      {net.ParseIP("fd00::1")},
	})
	transportHttp.SetHostResolver(fake.resolver)

	cases := []struct {
		raw         string
		expectError bool
		targetErr   error
	}{
		{raw: "http://rebind.example/latest/meta-data/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{raw: "http://private.example/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{raw: "http://loopback.example/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{raw: "http://mixed.example/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{raw: "https://ula.example/", expectError: true, targetErr: transportHttp.ErrSSRFBlocked},
		{raw: "https://public.example/film/1.html", expectError: false},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			err := transportHttp.ValidateSafeURL(tc.raw)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.raw)
				}
				if tc.targetErr != nil && !errors.Is(err, tc.targetErr) {
					t.Fatalf("expected %v, got %v", tc.targetErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
		})
	}
}

// TestValidateSafeURLCachesResolution — другий виклик не має ходити в DNS.
func TestValidateSafeURLCachesResolution(t *testing.T) {
	resetSecurityState(t)
	fake := startFakeDNS(t, map[string][]net.IP{"cached.example.": {net.ParseIP("93.184.216.34")}})
	transportHttp.SetHostResolver(fake.resolver)

	if err := transportHttp.ValidateSafeURL("https://cached.example/1.html"); err != nil {
		t.Fatalf("unexpected error on the first call: %v", err)
	}
	afterFirst := fake.lookupCount()
	if afterFirst == 0 {
		t.Fatal("expected at least one DNS lookup for the first call")
	}

	// Наступні виклики мають іти з кешу: DNS не торкаємося.
	for i := 0; i < 4; i++ {
		if err := transportHttp.ValidateSafeURL("https://cached.example/1.html"); err != nil {
			t.Fatalf("unexpected error on call %d: %v", i+2, err)
		}
	}
	if got := fake.lookupCount(); got != afterFirst {
		t.Fatalf("the resolution cache did not hold: %d -> %d lookups", afterFirst, got)
	}

	transportHttp.ClearResolveCache()
	if err := transportHttp.ValidateSafeURL("https://cached.example/1.html"); err != nil {
		t.Fatalf("unexpected error after cache clear: %v", err)
	}
	if got := fake.lookupCount(); got <= afterFirst {
		t.Fatalf("expected a fresh lookup after ClearResolveCache: %d -> %d", afterFirst, got)
	}
}

// TestValidateSafeURLUnresolvableIsTolerated — помилка резолву НЕ відкидає
// запит (див. requirePublicResolution): недоступний провайдер і так не
// завантажиться, а збій DNS не має вимикати контент-API.
func TestValidateSafeURLUnresolvableIsTolerated(t *testing.T) {
	resetSecurityState(t)
	transportHttp.SetHostResolver(startFakeDNS(t, map[string][]net.IP{}).resolver)

	if err := transportHttp.ValidateSafeURL("https://nx.example/"); err != nil {
		t.Fatalf("an unresolvable host must not fail validation: %v", err)
	}

	// ...але хост, який РЕАЛЬНО резолвиться у приватну адресу, — відкидається.
	fake := startFakeDNS(t, map[string][]net.IP{"nx.example.": {net.ParseIP("10.0.0.5")}})
	transportHttp.SetHostResolver(fake.resolver)
	if err := transportHttp.ValidateSafeURL("https://nx.example/"); !errors.Is(err, transportHttp.ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked once the host resolves privately, got %v", err)
	}
}

func TestValidateSafeURLHostAllowlist(t *testing.T) {
	resetSecurityState(t)

	// Порожній allow-list = будь-який публічний хост (документовано як unsafe).
	transportHttp.SetHostResolver(startFakeDNS(t, map[string][]net.IP{
		"public.example.": {net.ParseIP("93.184.216.34")},
		"evil.example.":   {net.ParseIP("169.254.169.254")},
	}).resolver)

	if err := transportHttp.ValidateSafeURL("https://public.example/a"); err != nil {
		t.Fatalf("empty allow-list must permit a public host, got %v", err)
	}
	if err := transportHttp.ValidateSafeURL("https://evil.example/a"); err == nil {
		t.Fatal("empty allow-list must still reject a host resolving to link-local")
	}

	transportHttp.SetUpstreamHostAllowlist([]string{"CATALOG.EXAMPLE", ".media.example"})
	cases := []struct {
		raw   string
		valid bool
	}{
		{raw: "https://catalog.example/film/1.html", valid: true},
		{raw: "https://www.media.example/video/1", valid: true},
		{raw: "https://public.example/a", valid: false},
		{raw: "https://evil.example/a", valid: false},
		// The classic suffix bypass: a host that merely *ends with* an allowed
		// name must not inherit the allow-list.
		{raw: "https://catalog.example.evil.example/a", valid: false},
		{raw: "http://127.0.0.1/", valid: false},
	}
	for _, tc := range cases {
		err := transportHttp.ValidateSafeURL(tc.raw)
		if (err == nil) != tc.valid {
			t.Errorf("allow-list verdict for %q: err=%v, want valid=%v", tc.raw, err, tc.valid)
		}
	}

	// Wildcard-запис зірочкою.
	transportHttp.SetUpstreamHostAllowlist([]string{"*.catalog.example"})
	if err := transportHttp.ValidateSafeURL("https://film.catalog.example/x.html"); err != nil {
		t.Fatalf("wildcard allow-list entry must match, got %v", err)
	}
}

// TestValidateSafeURLPublicHostEndToEnd — публічні літерали не потребують DNS.
func TestValidateSafeURLPublicLiteralNoResolver(t *testing.T) {
	resetSecurityState(t)
	transportHttp.SetHostResolver(startFakeDNS(t, map[string][]net.IP{}).resolver)

	for _, raw := range []string{
		"https://93.184.216.34/film/1.html",
		"http://8.8.8.8/",
		"https://[2606:2800:220:1:248:1893:25c8:1946]/",
	} {
		if err := transportHttp.ValidateSafeURL(raw); err != nil {
			t.Errorf("public literal %q rejected: %v", raw, err)
		}
	}
}

// TestValidateSafeURLIsSynchronousAndContextFree — сигнатура заморожена:
// усі викликачі (auth-редиректи та Watch Party hub) беруть її синхронно,
// без context.
func TestValidateSafeURLIsSynchronousAndContextFree(t *testing.T) {
	var fn func(string) error = transportHttp.ValidateSafeURL
	resetSecurityState(t)
	transportHttp.SetHostResolver(&net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dns disabled in test")
		},
	})

	done := make(chan error, 1)
	go func() { done <- fn("https://nx.example/") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ValidateSafeURL ran for more than 10s: the DNS timeout is not bounded")
	}
}

func TestSetHostResolverNilRestoresDefault(t *testing.T) {
	resetSecurityState(t)
	transportHttp.SetHostResolver(&net.Resolver{})
	transportHttp.SetHostResolver(nil)
	if err := transportHttp.ValidateSafeURL("http://127.0.0.1/"); !errors.Is(err, transportHttp.ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked, got %v", err)
	}
}
