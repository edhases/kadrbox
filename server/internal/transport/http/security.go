package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	ErrUnsafeURLScheme = errors.New("unsafe url scheme: only http and https are allowed")
	ErrSSRFBlocked     = errors.New("ssrf blocked: target points to internal, private or loopback address")
)

// ValidateSafeURL перевіряє вхідний URL або JSON-об'єкт на наявність загроз SSRF.
// Дозволені лише схеми http/https, публічні домени або безпечні відносні шляхи.
// Заборонені loopback (127.0.0.1, ::1, localhost), приватні підмережі (RFC 1918, RFC 4193),
// link-local (169.254.x.x - AWS/GCP metadata) та небезпечні протоколи (file, gopher, ftp).
func ValidateSafeURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("empty url")
	}

	// Якщо вхідний рядок є валідним JSON (наприклад, Bandera payload: {"href":"...", ...})
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			for _, key := range []string{"href", "url", "link"} {
				if val, ok := payload[key]; ok {
					if strVal, isStr := val.(string); isStr && strVal != "" {
						if err := validateSingleURL(strVal); err != nil {
							return fmt.Errorf("field %s: %w", key, err)
						}
					}
				}
			}
			return nil
		}
	}

	return validateSingleURL(raw)
}

func validateSingleURL(raw string) error {
	// Безпечний відносний шлях у межах сайту (наприклад, /serial/123-dune.html)
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed url: %w", err)
	}

	// Відносні ідентифікатори або шляхи без схеми
	if parsed.Scheme == "" {
		if strings.Contains(parsed.Host, "localhost") {
			return ErrSSRFBlocked
		}
		return nil
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrUnsafeURLScheme
	}

	host := parsed.Hostname()
	if host == "" {
		return errors.New("missing host in url")
	}

	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" ||
		lowerHost == "metadata.google.internal" ||
		lowerHost == "instance-data" ||
		strings.HasSuffix(lowerHost, ".localhost") ||
		strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".local") {
		return ErrSSRFBlocked
	}

	// Перевірка IP-адреси
	ip := net.ParseIP(host)
	if ip != nil {
		if isPrivateOrLocalIP(ip) {
			return ErrSSRFBlocked
		}
	}

	return nil
}

func isPrivateOrLocalIP(ip net.IP) bool {
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() {
		return true
	}

	// Перевірка 169.254.169.254 (Cloud metadata service)
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 0.0.0.0/8
		if ip4[0] == 0 {
			return true
		}
	}

	return false
}
