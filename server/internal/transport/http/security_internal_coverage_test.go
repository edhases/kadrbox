package http

// Internal coverage for the remaining pure helpers: the upload file server's
// directory-listing guard, the CORS origin rule, and the SSRF address
// classifier. All three decide access, so each case below is a case a caller
// should not be able to slip through.

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- fileServerNoListing ---------------------------------------------------

// covUploadRoot builds a directory tree to serve: a file, a subdirectory, and
// a file that looks like a listing but is not one.
func covUploadRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "avatar.png"), []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "avatars"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return root
}

func TestCovFileServerNoListingServesAFile(t *testing.T) {
	root := covUploadRoot(t)
	h := fileServerNoListing(root)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/avatar.png", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 for a real file (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Body.String(), "\x89PNG") {
		t.Errorf("body = %q, want the file's bytes", rec.Body.String())
	}
}

func TestCovFileServerNoListingRefusesEveryDirectoryShape(t *testing.T) {
	// A listing leaks every uploaded avatar to anyone who can reach the
	// server, so a directory is 404 in all three spellings rather than an
	// index page.
	root := covUploadRoot(t)
	h := fileServerNoListing(root)

	cases := []struct {
		name string
		path string
	}{
		{name: "trailingSlash", path: "/avatars/"},
		{name: "bareSlash", path: "/"},
		{name: "noTrailingSlash", path: "/avatars"},
		{name: "missingFile", path: "/does-not-exist.png"},
		{name: "traversal", path: "/../etc/passwd"},
		{name: "nestedTraversal", path: "/avatars/../../secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != http.StatusNotFound {
				t.Fatalf("got %d, want 404 for %q (body %s)", rec.Code, tc.path, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "avatar.png") {
				t.Errorf("the response leaked directory contents: %s", rec.Body.String())
			}
		})
	}
}

func TestCovFileServerNoListingOnAMissingRootAnswers404(t *testing.T) {
	h := fileServerNoListing(filepath.Join(t.TempDir(), "not-created"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/anything.png", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404 when the root does not exist", rec.Code)
	}
}

// ---- originAllowed ---------------------------------------------------------

func TestCovOriginAllowed(t *testing.T) {
	allowed := AllowedOrigins("https://film.oxideteam.pp.ua")

	t.Run("nativeClientsSendNoOrigin", func(t *testing.T) {
		// The Flutter desktop and mobile clients send no Origin at all;
		// rejecting them would break every native call.
		if !originAllowed("", allowed) {
			t.Error("an absent Origin was refused; native clients cannot set one")
		}
	})

	t.Run("exactHosts", func(t *testing.T) {
		for _, origin := range []string{
			"https://oxideteam.pp.ua",
			"https://film.oxideteam.pp.ua",
			"http://localhost:3000",
			"http://127.0.0.1:8080",
		} {
			if !originAllowed(origin, allowed) {
				t.Errorf("origin %q was refused", origin)
			}
		}
	})

	t.Run("suffixPatternMatchesSubdomains", func(t *testing.T) {
		if !originAllowed("https://staging.oxideteam.pp.ua", allowed) {
			t.Error("a *.oxideteam.pp.ua subdomain was refused")
		}
	})

	t.Run("suffixMatchingIsNotDoneBySubstring", func(t *testing.T) {
		// The classic bypass: an attacker registers a host whose name merely
		// ends with the allow-listed suffix.
		for _, origin := range []string{
			"https://evil-oxideteam.pp.ua",
			"https://oxideteam.pp.ua.evil.com",
			"https://notoxideteam.pp.ua",
			"https://evil.com?x=oxideteam.pp.ua",
		} {
			if originAllowed(origin, allowed) {
				t.Errorf("origin %q was accepted by substring rather than by suffix rule", origin)
			}
		}
	})

	t.Run("unknownHostsAreRefused", func(t *testing.T) {
		for _, origin := range []string{
			"https://evil.com",
			"https://localhost.evil.com",
			"https://127.0.0.1.nip.io",
		} {
			if originAllowed(origin, allowed) {
				t.Errorf("origin %q was accepted", origin)
			}
		}
	})

	t.Run("unparsableOriginIsRefused", func(t *testing.T) {
		// A malformed Origin must fail closed rather than falling through to
		// the allow-list check with an empty hostname.
		if originAllowed("://nonsense", allowed) {
			t.Error("an unparsable origin was accepted")
		}
	})

	t.Run("emptyAllowListRefusesEverything", func(t *testing.T) {
		if originAllowed("https://oxideteam.pp.ua", nil) {
			t.Error("an origin was accepted against an empty allow-list")
		}
	})
}

func TestCovAllowedOriginsAddsTheAppHostExactlyOnce(t *testing.T) {
	base := AllowedOrigins("")

	// An unparsable or host-less APP_URL must not add junk to the list.
	for _, appURL := range []string{"", "not a url", "/relative/path", "https://"} {
		got := AllowedOrigins(appURL)
		if len(got) != len(base) {
			t.Errorf("AllowedOrigins(%q) grew the list to %v, want the base %v", appURL, got, base)
		}
	}

	// A host already on the list must not be duplicated, or the CORS allow
	// list grows on every restart.
	withKnown := AllowedOrigins("https://localhost:8080")
	count := 0
	for _, o := range withKnown {
		if o == "localhost" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("localhost appears %d times in %v, want once", count, withKnown)
	}

	// A new host is appended so a self-hosted deployment works.
	withNew := AllowedOrigins("https://my.instance.example")
	found := false
	for _, o := range withNew {
		if o == "my.instance.example" {
			found = true
		}
	}
	if !found {
		t.Errorf("AllowedOrigins = %v, want it to include the APP_URL host", withNew)
	}
}

// ---- isPrivateOrLocalIP ---------------------------------------------------

func TestCovIsPrivateOrLocalIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// Blocked: loopback, RFC1918, link-local, CGNAT, multicast,
		// unspecified, and the cloud metadata address.
		{ip: "127.0.0.1", want: true},
		{ip: "127.10.20.30", want: true},
		{ip: "::1", want: true},
		{ip: "10.0.0.5", want: true},
		{ip: "172.16.0.1", want: true},
		{ip: "192.168.1.1", want: true},
		{ip: "169.254.1.1", want: true},
		{ip: "169.254.169.254", want: true},
		{ip: "0.0.0.0", want: true},
		{ip: "0.1.2.3", want: true},
		{ip: "224.0.0.1", want: true},
		{ip: "255.255.255.255", want: true},
		{ip: "::", want: true},
		{ip: "fe80::1", want: true},
		{ip: "fc00::1", want: true},
		// Allowed: ordinary public addresses.
		{ip: "1.1.1.1", want: false},
		{ip: "8.8.8.8", want: false},
		{ip: "93.184.216.34", want: false},
		{ip: "2001:4860:4860::8888", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			parsed := net.ParseIP(tc.ip)
			if parsed == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tc.ip)
			}
			if got := isPrivateOrLocalIP(parsed); got != tc.want {
				t.Errorf("isPrivateOrLocalIP(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestCovIsPrivateOrLocalIPTreatsNilAsBlocked(t *testing.T) {
	// A nil address comes from an empty A/AAAA record. Fail closed: a host
	// that resolves to nothing must not count as public.
	if !isPrivateOrLocalIP(nil) {
		t.Error("a nil address was treated as public")
	}
}
