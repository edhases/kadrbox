package http

// All server-rendered HTML for the auth surface lives here.
//
// Two rules apply to every function in this file:
//
//  1. Interpolation goes through html/template, never fmt.Sprintf("%s"). These
//     pages are served as text/html from the production API origin and
//     interpolate request-derived text (a provider's error_description, for
//     example), so bare interpolation is a reflected-XSS sink.
//  2. Pages that carry or lead to a token are sent with no-store, no-referrer
//     and no-cache, so the token URL is not kept in a cache or leaked through
//     a Referer header.

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"unicode"
)

// maxProviderMessageLen caps provider-supplied text (error_description) before
// it is rendered. Escaping alone would be enough for safety; the cap keeps a
// hostile provider from turning the status page into an arbitrary-content host.
const maxProviderMessageLen = 200

// sanitizeProviderMessage strips control characters (including NUL and
// newlines, which some renderers treat as markup boundaries) and caps length.
func sanitizeProviderMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(msg))
	for _, r := range msg {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	cleaned := strings.Join(strings.Fields(b.String()), " ")

	runes := []rune(cleaned)
	if len(runes) > maxProviderMessageLen {
		cleaned = strings.TrimSpace(string(runes[:maxProviderMessageLen])) + "..."
	}
	return cleaned
}

// strictCSP is the policy for pages that render entirely from this origin.
//
// `frame-ancestors 'none'` is not decoration: these pages carry one-time tokens
// in their URLs, and a framed page is a page whose address bar a hostile site can
// read. The middleware sets the same rule for non-HTML responses, but a page that
// sets its own CSP replaces that header, so it has to repeat the clause here.
//
// script-src allows inline because the pages' only script is a static deep-link
// redirect. Nothing external needs to execute, so there is no external origin in
// the list.
const strictCSP = "default-src 'none'; style-src 'unsafe-inline'; " +
	"script-src 'unsafe-inline'; frame-ancestors 'none'"

// telegramWidgetCSP is the one policy that has to name an external origin.
//
// The Telegram login widget is a script from telegram.org that then injects an
// iframe. The previous build applied strictCSP here as well, which meant
// `script-src 'unsafe-inline'` with no telegram.org in it: the widget script was
// blocked by our own policy and the button never appeared. A page that needs a
// third party must allow exactly that third party and nothing else.
const telegramWidgetCSP = "default-src 'none'; style-src 'unsafe-inline'; " +
	"script-src 'unsafe-inline' https://telegram.org; " +
	"frame-src https://telegram.org; " +
	"frame-ancestors 'none'"

// setNoTokenCacheHeaders marks a response as never cacheable and never
// referrer-leaking. Used on every page that contains, or redirects to, a token.
func setNoTokenCacheHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	header.Set("Pragma", "no-cache")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", strictCSP)
}

// statusPageData is the whole data set for the status templates. Keeping it a
// struct (rather than positional args) is what lets the deep link be typed as
// template.URL: html/template refuses to emit a non-http(s) URL from a plain
// string in an href, and oxide:// must not be scrubbed to #ZgotmplZ.
type statusPageData struct {
	Title       string
	Message     string
	Icon        string
	AccentColor string
	// DeepLink is pre-validated and assembled only from server-generated values
	// (a generated access token, a UUID refresh token, a fixed provider name).
	HasDeepLink bool
	DeepLink    template.URL
}

var statusPageTemplate = template.Must(template.New("status").Parse(`<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}} — Oxide Film</title>
  <style>
    body {
      margin: 0; padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #0f172a; color: #f8fafc;
      display: flex; align-items: center; justify-content: center; min-height: 100vh;
    }
    .card {
      background: #1e293b; border: 1px solid #334155; border-radius: 16px;
      padding: 40px; max-width: 440px; margin: 20px; text-align: center;
      box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    .icon { font-size: 54px; margin-bottom: 20px; }
    h1 { font-size: 22px; margin: 0 0 12px; color: #fff; }
    p { color: #94a3b8; font-size: 15px; line-height: 1.5; margin: 0 0 24px; }
    .logo { font-size: 14px; color: {{.AccentColor}}; font-weight: 700; letter-spacing: 0.5px; text-transform: uppercase; margin-bottom: 8px; }
{{if .HasDeepLink}}    .btn {
      display: inline-block; padding: 12px 24px; color: #fff; text-decoration: none;
      border-radius: 8px; font-weight: bold; font-size: 15px; transition: opacity 0.2s;
    }
    .btn:hover { opacity: 0.9; }
{{end}}  </style>
</head>
<body>
  <div class="card">
    <div class="logo">Oxide Film</div>
    <div class="icon">{{.Icon}}</div>
    <h1>{{.Title}}</h1>
    <p>{{.Message}}</p>
    {{if .HasDeepLink}}<a href="{{.DeepLink}}" class="btn" style="background: {{.AccentColor}};">Відкрити Oxide Film</a>{{end}}
  </div>
  {{if .HasDeepLink}}<script>
    try {
      window.location.href = {{.DeepLink}};
    } catch(e) {}
  </script>{{end}}
</body>
</html>
`))

// renderOAuthStatusHTML renders the OAuth result/error page. Title and message
// are escaped by the template; callers should still pass sanitized text.
func renderOAuthStatusHTML(success bool, title, message, provider, accessToken, refreshToken string) string {
	icon := "✅"
	accentColor := "#10b981"
	if !success {
		icon = "❌"
		accentColor = "#ef4444"
	} else {
		switch provider {
		case "discord":
			accentColor = "#5865F2"
		case "telegram":
			accentColor = "#229ED9"
		case "google":
			accentColor = "#EA4335"
		}
	}

	data := statusPageData{
		Title:       sanitizeProviderMessage(title),
		Message:     sanitizeProviderMessage(message),
		Icon:        icon,
		AccentColor: accentColor,
	}

	if success && accessToken != "" && refreshToken != "" {
		data.DeepLink = template.URL(buildDeepLink(provider, accessToken, refreshToken))
		data.HasDeepLink = true
	}

	var sb strings.Builder
	if err := statusPageTemplate.Execute(&sb, data); err != nil {
		// A template error must not leak the raw inputs into the page.
		return "<!DOCTYPE html><html lang=\"uk\"><body>Помилка сервера</body></html>"
	}
	return sb.String()
}

func renderEmailStatusHTML(success bool, title, message string) string {
	icon := "✅"
	accentColor := "#6366f1"
	if !success {
		icon = "❌"
		accentColor = "#ef4444"
	}

	var sb strings.Builder
	data := statusPageData{
		Title:       sanitizeProviderMessage(title),
		Message:     sanitizeProviderMessage(message),
		Icon:        icon,
		AccentColor: accentColor,
	}
	if err := statusPageTemplate.Execute(&sb, data); err != nil {
		return "<!DOCTYPE html><html lang=\"uk\"><body>Помилка сервера</body></html>"
	}
	return sb.String()
}

// buildDeepLink assembles the oxide:// deep link from server-generated values
// only. provider is matched against a fixed set before it is interpolated, so
// the result cannot carry attacker-controlled characters.
func buildDeepLink(provider, accessToken, refreshToken string) string {
	safeProvider := ""
	switch provider {
	case "google", "discord", "telegram":
		safeProvider = provider
	}
	if safeProvider == "" {
		return fmt.Sprintf("oxide://auth?access_token=%s&refresh_token=%s",
			template.URLQueryEscaper(accessToken), template.URLQueryEscaper(refreshToken))
	}
	return fmt.Sprintf("oxide://auth/%s?access_token=%s&refresh_token=%s",
		safeProvider,
		template.URLQueryEscaper(accessToken),
		template.URLQueryEscaper(refreshToken),
	)
}

// writeHTMLStatus renders one of the status pages with the no-store headers.
func writeHTMLStatus(w http.ResponseWriter, status int, html string) {
	writeHTMLStatusWithCSP(w, status, html, strictCSP)
}

// writeHTMLStatusWithCSP is writeHTMLStatus for a page that needs a different
// policy. The no-store headers still apply: a page holding a token is never
// cacheable whatever it is allowed to load.
func writeHTMLStatusWithCSP(w http.ResponseWriter, status int, html, csp string) {
	setNoTokenCacheHeaders(w)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html))
}

// ---- Telegram login widget ---------------------------------------------------

var telegramWidgetTemplate = template.Must(template.New("telegram-widget").Parse(`<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Вхід через Telegram — Oxide Film</title>
  <style>
    body {
      margin: 0; padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #0f172a; color: #f8fafc;
      display: flex; align-items: center; justify-content: center; min-height: 100vh;
    }
    .card {
      background: #1e293b; border: 1px solid #334155; border-radius: 16px;
      padding: 40px; max-width: 440px; margin: 20px; text-align: center;
      box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    .icon { font-size: 50px; margin-bottom: 16px; }
    h1 { font-size: 22px; margin: 0 0 12px; color: #fff; }
    p { color: #94a3b8; font-size: 15px; line-height: 1.5; margin: 0 0 24px; }
    .logo { font-size: 14px; color: #229ED9; font-weight: 700; letter-spacing: 0.5px; text-transform: uppercase; margin-bottom: 8px; }
    .widget-container { display: flex; justify-content: center; margin: 16px 0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">Oxide Film</div>
    <div class="icon">✈️</div>
    <h1>Вхід через Telegram</h1>
    <p>Натисніть кнопку нижче для авторизації за допомогою вашого облікового запису Telegram:</p>
    <div class="widget-container">
      <script async src="https://telegram.org/js/telegram-widget.js?22"
              data-telegram-login="{{.BotUsername}}"
              data-size="large"
              data-radius="12"
              data-auth-url="{{.AuthURL}}"
              data-request-access="write"></script>
    </div>
    <div id="tg-notice" style="display:none; margin-top: 16px; padding: 12px; background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.3); border-radius: 8px; font-size: 13px; color: #fca5a5; line-height: 1.4;">
      Кнопка Telegram не з'явилася? Переконайтеся, що домен сайту прив'язано до бота в <b>@BotFather</b> через команду <code>/setdomain</code>.
    </div>
    <script>
      setTimeout(function() {
        var container = document.querySelector('.widget-container');
        if (!container || !container.querySelector('iframe')) {
          var notice = document.getElementById('tg-notice');
          if (notice) notice.style.display = 'block';
        }
      }, 2500);
    </script>
  </div>
</body>
</html>
`))

func renderTelegramWidgetHTML(botUsername, authURL string) string {
	var sb strings.Builder
	err := telegramWidgetTemplate.Execute(&sb, struct {
		BotUsername string
		AuthURL     string
	}{BotUsername: botUsername, AuthURL: authURL})
	if err != nil {
		return "<!DOCTYPE html><html lang=\"uk\"><body>Помилка сервера</body></html>"
	}
	return sb.String()
}

// ---- password reset form -----------------------------------------------------

var resetPasswordTemplate = template.Must(template.New("reset-password").Parse(`<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Скидання пароля — Oxide Film</title>
  <style>
    body {
      margin: 0; padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #0f172a; color: #f8fafc;
      display: flex; align-items: center; justify-content: center; min-height: 100vh;
    }
    .card {
      background: #1e293b; border: 1px solid #334155; border-radius: 16px;
      padding: 36px; max-width: 400px; width: 90%; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    .logo { color: #6366f1; font-weight: 700; text-transform: uppercase; font-size: 14px; margin-bottom: 8px; }
    h1 { font-size: 20px; margin: 0 0 16px; color: #fff; }
    p { color: #94a3b8; font-size: 14px; margin: 0 0 20px; line-height: 1.5; }
    input {
      width: 100%; padding: 12px; border-radius: 8px; border: 1px solid #475569;
      background: #0f172a; color: #fff; font-size: 15px; margin-bottom: 16px; box-sizing: border-box;
    }
    input:focus { outline: none; border-color: #6366f1; }
    button {
      width: 100%; padding: 12px; background: #6366f1; color: #fff; border: none;
      border-radius: 8px; font-size: 15px; font-weight: bold; cursor: pointer;
    }
    button:hover { background: #4f46e5; }
    .msg { margin-top: 16px; font-size: 14px; display: none; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">Oxide Film</div>
    <h1>Новий пароль</h1>
    <p>Введіть новий пароль для вашого облікового запису (мінімум 6 символів):</p>
    <input type="password" id="pwd" placeholder="Новий пароль" minlength="6" required />
    <button onclick="submitReset()">Зберегти пароль</button>
    <div id="res" class="msg"></div>
  </div>
  <script>
    async function submitReset() {
      var p = document.getElementById('pwd').value;
      var res = document.getElementById('res');
      if (!p || p.length < 6) {
        res.style.display = 'block'; res.style.color = '#ef4444';
        res.innerText = 'Пароль має містити щонайменше 6 символів';
        return;
      }
      try {
        var resp = await fetch('/api/v1/auth/reset-password', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({token: {{.Token}}, password: p})
        });
        var data = await resp.json();
        res.style.display = 'block';
        if (resp.ok) {
          res.style.color = '#10b981';
          res.innerText = 'Пароль успішно змінено! Тепер ви можете увійти в застосунок.';
          document.getElementById('pwd').style.display = 'none';
          document.querySelector('button').style.display = 'none';
        } else {
          res.style.color = '#ef4444';
          res.innerText = data.error || 'Помилка при збереженні пароля';
        }
      } catch (e) {
        res.style.display = 'block'; res.style.color = '#ef4444';
        res.innerText = 'Мережева помилка';
      }
    }
  </script>
</body>
</html>
`))

// renderResetPasswordForm embeds the reset token in a JS string literal.
// html/template's JS-string escaper neutralises quotes and "</script>", which the
// previous fmt.Sprintf("%q") did not: %q escapes Go string delimiters, not HTML
// script boundaries.
func renderResetPasswordForm(token string) string {
	var sb strings.Builder
	err := resetPasswordTemplate.Execute(&sb, struct{ Token string }{Token: token})
	if err != nil {
		return "<!DOCTYPE html><html lang=\"uk\"><body>Помилка сервера</body></html>"
	}
	return sb.String()
}
