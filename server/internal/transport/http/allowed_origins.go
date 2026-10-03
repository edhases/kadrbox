package http

import (
	"net/url"
	"strings"
)

// AllowedOrigins будує allow-list джерел для CORS і для WebSocket handshake.
// Джерела мають збігатися: інакше браузер успішно проходить REST-виклики,
// але отримує 403 origin_not_allowed при апгрейді Watch Party-сокета.
//
// appURL — публічна адреса сервісу (APP_URL); її host додається до allow-list
// разом із базовим доменом і localhost для локальної розробки.
func AllowedOrigins(appURL string) []string {
	origins := []string{"localhost", "127.0.0.1", "oxideteam.pp.ua", ".oxideteam.pp.ua"}
	if u, err := url.Parse(appURL); err == nil && u.Hostname() != "" {
		host := u.Hostname()
		found := false
		for _, existing := range origins {
			if existing == host {
				found = true
				break
			}
		}
		if !found {
			origins = append(origins, host)
		}
	}
	return origins
}

// originAllowed повторює семантику CORS AllowOriginFunc. Експортовано, бо
// middleware.RequestLogger і ws.Options мають перевіряти те саме правило.
func originAllowed(origin string, allowed []string) bool {
	// Нативні клієнти (Flutter desktop/mobile) не надсилають Origin.
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	hostname := u.Hostname()
	for _, pattern := range allowed {
		if strings.HasPrefix(pattern, ".") {
			if strings.HasSuffix(hostname, pattern) {
				return true
			}
			continue
		}
		if hostname == pattern {
			return true
		}
	}
	return false
}
