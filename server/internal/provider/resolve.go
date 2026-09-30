package provider

// Shared player-page resolver for the DLE based providers (uakino, eneyida, lavakino).
//
// Background: every DLE item page embeds the real player in an <iframe>. Handing
// that iframe URL to libmpv is a guaranteed 100% playback failure, because the
// iframe serves text/html and mpv answers with
// "Failed to recognize file format." The iframe URL must therefore never reach
// the player: it has to be fetched and reduced to an actual media URL first.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

// ErrUnresolvablePlayer is returned when a player page yields no playable media URL.
var ErrUnresolvablePlayer = errors.New("provider: player page exposes no playable media")

const (
	// MaxPlayerIframes caps how many player iframes a single GetStreams call probes.
	MaxPlayerIframes = 4

	// PlayerResolveTimeout is the per-call budget for the whole iframe fan-out of
	// one GetStreams invocation, so a single hung player host cannot eat the
	// server's whole write budget.
	PlayerResolveTimeout = 20 * time.Second

	// defaultStreamQuality is used when the media URL carries no quality token.
	defaultStreamQuality = "Auto"
)

// Package level regexps: they were previously recompiled on every call, which the
// audit flagged (and which is measurable on the hot path of every item page).
var (
	rePlayerJSFile   = regexp.MustCompile(`file\s*:\s*["']([^"']+)["']`)
	reSourcesBlock   = regexp.MustCompile(`(?s)sources\s*:\s*\[(.*?)\]`)
	reSourcesSrc     = regexp.MustCompile(`src\s*:\s*["']([^"']+)["']`)
	reHlsLoadSource  = regexp.MustCompile(`Hls\.loadSource\(\s*["']([^"']+)["']`)
	reBase64File     = regexp.MustCompile(`["']?file["']?\s*:\s*["']([A-Za-z0-9+/=_-]{16,})["']`)
	reBareMediaURL   = regexp.MustCompile(`["'(](https?://[^"'()\s<>]+\.(?:m3u8|mpd|mp4)(?:\?[^"'()\s<>]*)?)["')]`)
	rePlayerPagePath = regexp.MustCompile(`(?i)/(?:embed|player|iframe|watch)`)
	reMediaExt       = regexp.MustCompile(`(?i)\.(?:m3u8|mpd|mp4)$`)
	reQualityToken   = regexp.MustCompile(`(?i)\b(4k|2160|1080|720|480|360)p?\b`)
)

// resolveStrategy is one extraction attempt over a chunk of player HTML.
type resolveStrategy struct {
	name  string
	match func(text string) (string, bool)
}

// resolveStrategies is the ordered extraction list. Order matters: the first
// entry that yields a valid, playable URL wins.
var resolveStrategies = append(append([]resolveStrategy{}, plainStrategies...),
	resolveStrategy{"base64-playerjs", matchBase64PlayerJS},
	resolveStrategy{"bare-media-url", matchBareMediaURL},
)

// plainStrategies are the strategies that look at raw (non-decoded) player config.
// The base64 strategy re-runs exactly these on the decoded payload.
var plainStrategies = []resolveStrategy{
	{"playerjs-file", matchPlayerJSFile},
	{"sources-block", matchSourcesBlock},
	{"hls-loadsource", matchHlsLoadSource},
}

// ResolvePlayerHTML fetches an HTML player page and extracts a playable media URL.
//
// siteBaseURL is the site base URL used as the Referer for the fetch of playerURL
// itself. The returned source carries Referer/Origin derived from the ORIGIN of the
// resolved media URL (not the aggregator's), because the CDN families behind these
// players (ashdi/ukrtelcdn and friends) validate both.
//
// INVARIANT: this function never returns a non-media URL. On total failure it
// returns a zero-value source together with an error wrapping ErrUnresolvablePlayer,
// so callers can never accidentally feed an HTML page to mpv. A wrong extraction
// here reproduces the very bug this resolver exists to fix, hence the explicit
// validation in isPlayableMediaURL below rather than trusting the regexps.
func ResolvePlayerHTML(ctx context.Context, client *TLSClient, playerURL, siteBaseURL string) (domain.StreamSource, error) {
	source, _, err := resolvePlayerHTML(ctx, client, playerURL, siteBaseURL)
	return source, err
}

// resolvePlayerHTML is the diagnostic variant of ResolvePlayerHTML: it also reports
// which strategy matched (or the last one tried on failure).
func resolvePlayerHTML(ctx context.Context, client *TLSClient, playerURL, siteBaseURL string) (domain.StreamSource, string, error) {
	if strings.TrimSpace(playerURL) == "" {
		return domain.StreamSource{}, "", ErrUnresolvablePlayer
	}

	page, err := client.Get(ctx, playerURL, siteBaseURL)
	if err != nil {
		return domain.StreamSource{}, "fetch", fmt.Errorf("fetch player page %s: %w", playerURL, err)
	}

	if raw, strategy, ok := extractPlayableURL(page); ok {
		return newStreamSource(raw, playerURL), strategy, nil
	}

	return domain.StreamSource{}, "none", fmt.Errorf("%w: %s", ErrUnresolvablePlayer, playerURL)
}

// extractPlayableURL runs the ordered strategy list over text and returns the first
// candidate that passes validation, together with the name of the winning strategy.
func extractPlayableURL(text string) (rawURL, strategy string, ok bool) {
	if text == "" {
		return "", "none", false
	}
	for _, s := range resolveStrategies {
		if candidate, found := s.match(text); found {
			if isPlayableMediaURL(candidate) {
				return candidate, s.name, true
			}
		}
	}
	return "", "none", false
}

// ---- strategies ----

// matchPlayerJSFile: plain PlayerJS object, e.g. file:"https://cdn/x/master.m3u8".
func matchPlayerJSFile(text string) (string, bool) {
	return firstCaptured(rePlayerJSFile, text)
}

// matchSourcesBlock: sources:[{ src: "..." }] / sources: [{src:'...'}].
func matchSourcesBlock(text string) (string, bool) {
	for _, block := range reSourcesBlock.FindAllStringSubmatch(text, -1) {
		if len(block) < 2 {
			continue
		}
		if candidate, ok := firstCaptured(reSourcesSrc, block[1]); ok {
			return candidate, true
		}
	}
	return "", false
}

// matchHlsLoadSource: Hls.loadSource("https://cdn/x/index.m3u8").
func matchHlsLoadSource(text string) (string, bool) {
	return firstCaptured(reHlsLoadSource, text)
}

// matchBase64PlayerJS: {"file":"<base64 of the real player config>"} — decode and
// re-run the plain strategies 1-3 on the decoded text. Handles both the standard
// and URL-safe alphabets as well as missing '=' padding.
func matchBase64PlayerJS(text string) (string, bool) {
	for _, block := range reBase64File.FindAllStringSubmatch(text, -1) {
		if len(block) < 2 {
			continue
		}
		decoded, err := decodeBase64Loose(block[1])
		if err != nil {
			continue
		}
		for _, s := range plainStrategies { // strategies 1-3 only, never recurse
			if candidate, ok := s.match(decoded); ok && isPlayableMediaURL(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// matchBareMediaURL: a quoted media URL anywhere in the page.
func matchBareMediaURL(text string) (string, bool) {
	return firstCaptured(reBareMediaURL, text)
}

// ---- validation / source construction ----

// isPlayableMediaURL is the gate that keeps HTML pages out of the stream list.
// It rejects everything that is not an absolute http(s) URL and everything that
// looks like a player page rather than media.
func isPlayableMediaURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return false
	}
	// Markup, whitespace or an embedded quote means we grabbed a chunk of HTML.
	if strings.ContainsAny(raw, "<>\"' \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	if strings.HasSuffix(u.Path, "/") {
		return false
	}
	// A media extension wins over the player-page heuristic: some CDNs legitimately
	// serve /player/... paths for manifests, but a path that looks like /embed or
	// /watch *without* any media extension is a player page, never a stream.
	if reMediaExt.MatchString(u.Path) {
		return true
	}
	return !rePlayerPagePath.MatchString(u.Path)
}

// newStreamSource builds the StreamSource for a validated media URL.
func newStreamSource(mediaURL, playerURL string) domain.StreamSource {
	headers := map[string]string{
		"User-Agent": Chrome120UserAgent,
	}
	// Referer/Origin must belong to the media host: the CDN validates them.
	// Fall back to the player page when the media URL is not parseable as absolute.
	referer := playerURL
	if u, err := url.Parse(mediaURL); err == nil && u.Scheme != "" && u.Host != "" {
		origin := originOf(u)
		referer = origin + "/"
		headers["Origin"] = origin
	}
	headers["Referer"] = referer

	return domain.StreamSource{
		Quality:       qualityFromURL(mediaURL),
		URL:           mediaURL,
		DirectURL:     mediaURL,
		RequiresProxy: false,
		Headers:       headers,
	}
}

func originOf(u *url.URL) string {
	return u.Scheme + "://" + u.Host
}

// qualityFromURL reports a recognizable quality token (1080/720/480/360/2160/4K,
// optionally suffixed with 'p') or "Auto".
func qualityFromURL(mediaURL string) string {
	m := reQualityToken.FindStringSubmatch(mediaURL)
	if len(m) < 2 {
		return defaultStreamQuality
	}
	if strings.EqualFold(m[1], "4k") {
		return "4K"
	}
	return m[1] + "p"
}

func firstCaptured(re *regexp.Regexp, text string) (string, bool) {
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			return strings.TrimSpace(m[1]), true
		}
	}
	return "", false
}

// decodeBase64Loose accepts standard and URL-safe alphabets with or without '=' padding.
func decodeBase64Loose(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrUnresolvablePlayer
	}
	s = strings.NewReplacer("-", "+", "_", "/", "\n", "", "\r", "").Replace(s)
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	case 1:
		return "", ErrUnresolvablePlayer
	}
	if out, err := base64.StdEncoding.DecodeString(s); err == nil {
		return string(out), nil
	}
	out, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ---- iframe selection ----

// iframeHints are substrings typical for the hosts/paths that actually host a DLE player.
var iframeHints = []string{
	"player", "embed", "ashdi", "zenith", "hdvbua", "videohost", "kodik",
	"megogo", "okdictator", "goodplay", "hydra", "collaps", "hls", "m3u8",
	"mp4", "video", "media", "cdn", "stream",
}

// iframeSkips are comment widgets, ad slots, promo banners and social widgets that
// regularly precede the real player inside the first <iframe> of a DLE page.
var iframeSkips = []string{
	"about:blank", "javascript:", "data:", "recaptcha", "doubleclick",
	"googlesyndication", "googleadservices", "adsbygoogle", "/ads/", "adframe",
	"banner", "promo", "comment", "disqus", "facebook", "vk.com", "vkontakte",
	"telegram", "twitter", "instagram", "subscribe", "widget", "share",
	"trailer", "youtube", "youtu.be", "vimeo",
}

type iframeCandidate struct {
	url   string
	score int
}

// rankPlayerIframes returns candidate player iframe URLs ordered best first and
// de-duplicated. Taking the FIRST <iframe> is unreliable: comment widgets, ad slots
// and promo banners often come first, so candidates are scored and the plausible
// player frames win.
func rankPlayerIframes(html, itemURL string) []string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	itemHost := hostOf(itemURL)
	var candidates []iframeCandidate
	doc.Find("iframe").Each(func(_ int, s *goquery.Selection) {
		sel := s.First()
		src := strings.TrimSpace(sel.AttrOr("src", ""))
		if src == "" {
			src = strings.TrimSpace(sel.AttrOr("data-src", ""))
		}
		abs := absolutizeURL(src, itemURL)
		if abs == "" {
			return
		}
		score := scorePlayerIframe(abs, itemHost)
		if score <= 0 {
			return
		}
		candidates = append(candidates, iframeCandidate{url: abs, score: score})
	})

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	seen := make(map[string]bool, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if seen[c.url] {
			continue
		}
		seen[c.url] = true
		out = append(out, c.url)
	}
	return out
}

// scorePlayerIframe returns 0 for frames that must be skipped, otherwise a positive
// priority: a cross-host iframe is the usual player, and known player hints push it up.
func scorePlayerIframe(abs, itemHost string) int {
	lower := strings.ToLower(abs)
	for _, bad := range iframeSkips {
		if strings.Contains(lower, bad) {
			return 0
		}
	}

	score := 1
	if hostOf(abs) != "" && hostOf(abs) != itemHost {
		score += 4
	}
	for _, hint := range iframeHints {
		if strings.Contains(lower, hint) {
			score += 4
			break
		}
	}
	if u, err := url.Parse(abs); err == nil && reMediaExt.MatchString(u.Path) {
		score += 2
	}
	// A same-host iframe without any player hint stays allowed (some DLE templates
	// inline the player) but ranks below every real player frame.
	return score
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func absolutizeURL(src, base string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		b, err := url.Parse(base)
		if err != nil {
			return ""
		}
		rel, err := url.Parse(src)
		if err != nil {
			return ""
		}
		src = b.ResolveReference(rel).String()
	}
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return ""
	}
	return src
}

// resolveStreamsFromItemPage is the shared tail of the DLE GetStreams implementations.
// It probes the ranked player iframes (capped, deadline applied by the caller) and
// returns the first stream that survived validation. When nothing resolves it returns
// an EMPTY stream list together with an error wrapping ErrUnresolvablePlayer — never
// a list of HTML pages, so the client can show "плеєр не підтримується" instead of
// handing text/html to libmpv.
func resolveStreamsFromItemPage(ctx context.Context, client *TLSClient, providerID, itemURL, html string) (*domain.ContentStreamsResponse, error) {
	resp := &domain.ContentStreamsResponse{
		ProviderID: providerID,
		Streams:    []domain.StreamSource{},
	}

	candidates := rankPlayerIframes(html, itemURL)
	if len(candidates) > MaxPlayerIframes {
		candidates = candidates[:MaxPlayerIframes]
	}

	var lastErr error
	for _, candidate := range candidates {
		source, _, err := resolvePlayerHTML(ctx, client, candidate, itemURL)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Streams = append(resp.Streams, source)
		return resp, nil // early exit: one real stream is enough
	}

	err := lastErr
	switch {
	case err == nil:
		err = ErrUnresolvablePlayer
	case errors.Is(err, ErrUnresolvablePlayer):
	default:
		err = fmt.Errorf("%w: %w", ErrUnresolvablePlayer, err)
	}
	return resp, fmt.Errorf("%s streams: %w", providerID, err)
}
