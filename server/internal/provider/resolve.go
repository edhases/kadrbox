package provider

// Shared player-page resolver for the DLE based providers (uakino, eneyida, lavakino).
//
// Background: every DLE item page embeds the real player in an <iframe> or tabbed
// player containers (Ashdi, Zenith, HDVB, Videohost, etc.). Handing that iframe URL
// to libmpv is a guaranteed 100% playback failure, because the iframe serves text/html
// and mpv answers with "Failed to recognize file format." The iframe URL must therefore
// never reach the player: it has to be fetched and reduced to actual media URLs first.
//
// Modern movie and TV pages often have MULTIPLE players (e.g. up to 4 players for
// "Величне століття: Роксолана"), each exposing different balancers, audio dubbings,
// video qualities (1080p, 720p, 480p), and subtitles. This unified resolver discovers
// all player candidates on the page, extracts all playable streams with their qualities
// and dubbings, and merges subtitles into a unified ContentStreamsResponse.
//
// # Function inventory (a future split should follow these groups)
//
//	entry points   ResolvePlayerHTML, resolvePlayerHTML, resolveStreamsFromItemPage
//	extraction     extractAllStreamsFromPlayer, extractStreamsFromPlaylistTree,
//	               parseMultiQualityString, parseSubtitlesFromPlayerHTML,
//	               parseSeasonOrEpisodeNum
//	URL strategies extractPlayableURL + matchPlayerJSFile / matchSourcesBlock /
//	               matchHlsLoadSource / matchBase64PlayerJS / matchBareMediaURL
//	validation     isPlayableMediaURL, newStreamSource, qualityFromURL,
//	               normalizeQualityLabel, isPlausiblePlayerOrMedia
//	candidates     rankPlayerCandidates, rankPlayerIframes, scorePlayerIframe,
//	               scanPlayerURL, absolutizeURL, hostOf, detectPlayerBalancer
//	utilities      firstCaptured, decodeBase64Loose, originOf
//
// Sentinel convention for the error-less parsers: a nil slice means "nothing
// matched" (the input carried no player config at all), while a non-empty slice
// that is shorter than the input carried means "parsed but partially rejected"
// (an entry failed isPlayableMediaURL or was a duplicate). The two are NOT
// interchangeable — an empty result makes the caller fall through to the next
// strategy, a short result is returned as-is.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

// ErrUnresolvablePlayer is returned when a player page yields no playable media URL.
var ErrUnresolvablePlayer = errors.New("provider: player page exposes no playable media")

const (
	// MaxPlayerIframes caps how many player candidates a single GetStreams call probes.
	MaxPlayerIframes = 8

	// PlayerResolveTimeout is the per-call budget for the whole iframe fan-out of
	// one GetStreams invocation, so a single hung player host cannot eat the
	// server's whole write budget.
	PlayerResolveTimeout = 20 * time.Second

	// defaultStreamQuality is used when the media URL carries no quality token.
	defaultStreamQuality = "Auto"

	// playerFanoutConcurrency bounds how many player candidates are fetched at
	// once. 4 keeps a resolve from opening 8 TLS connections to third-party
	// hosts (which look like a scraper attack) while still overlapping the
	// round-trips.
	playerFanoutConcurrency = 4

	// playerCandidateTimeout is the per-candidate sub-budget. All candidates
	// share PlayerResolveTimeout, so without a sub-budget ONE hung player host
	// eats 15s of the 20s and the loop gives up on a title that a later
	// candidate would have resolved.
	playerCandidateTimeout = 5 * time.Second
)

// Package level regexps: compiled once for high-throughput stream resolution.
var (
	rePlayerJSFile     = regexp.MustCompile(`file\s*:\s*["']([^"']+)["']`)
	rePlayerJSJSONFile = regexp.MustCompile(`file\s*:\s*(\[[^"'].*?\])`)
	reSourcesBlock     = regexp.MustCompile(`(?s)sources\s*:\s*\[(.*?)\]`)
	reSourcesSrc       = regexp.MustCompile(`src\s*:\s*["']([^"']+)["']`)
	reSourcesFile      = regexp.MustCompile(`file\s*:\s*["']([^"']+)["']`)
	reSourcesLabel     = regexp.MustCompile(`(?:label|title)\s*:\s*["']([^"']+)["']`)
	reHlsLoadSource    = regexp.MustCompile(`Hls\.loadSource\(\s*["']([^"']+)["']`)
	reBase64File       = regexp.MustCompile(`["']?file["']?\s*:\s*["']([A-Za-z0-9+/=_-]{16,})["']`)
	reBareMediaURL     = regexp.MustCompile(`["'(](https?://[^"'()\s<>]+\.(?:m3u8|mpd|mp4)(?:\?[^"'()\s<>]*)?)["')]`)
	rePlayerPagePath   = regexp.MustCompile(`(?i)/(?:embed|player|iframe|watch)`)
	reMediaExt         = regexp.MustCompile(`(?i)\.(?:m3u8|mpd|mp4)$`)
	reQualityToken     = regexp.MustCompile(`(?i)\b(4k|2160|1080|720|480|360)p?\b`)
	rePlayerJSSubtitle = regexp.MustCompile(`["']?subtitle(?:s)?["']?\s*:\s*["']([^"']+)["']`)
	reTrackTag         = regexp.MustCompile(`(?i)<track[^>]+>`)
	reSrcAttr          = regexp.MustCompile(`(?i)\bsrc\s*=\s*["']([^"']+)["']`)
	reLabelAttr        = regexp.MustCompile(`(?i)\blabel\s*=\s*["']([^"']+)["']`)
	reSrclangAttr      = regexp.MustCompile(`(?i)\bsrclang\s*=\s*["']([^"']+)["']`)
	reDigits           = regexp.MustCompile(`\d+`)
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
// so callers can never accidentally feed an HTML page to mpv.
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
		src := newStreamSource(raw, playerURL)
		balancer := detectPlayerBalancer(playerURL)
		src.Player = balancer
		src.Voiceover = balancer
		return src, strategy, nil
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
		if candidate, ok := firstCaptured(reSourcesFile, block[1]); ok {
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

// qualityResolutionTokens is checked in this exact order (highest first), so
// "1080p" wins over "720p" when a label mentions both. Package level: it was a
// slice literal inside the range expression, i.e. one allocation per call.
var qualityResolutionTokens = []string{"1440", "1080", "720", "480", "360"}

// normalizeQualityLabel cleans up quality tokens like "1080p", "720", "4K", "FHD".
func normalizeQualityLabel(q string) string {
	q = strings.TrimSpace(q)
	lower := strings.ToLower(q)
	if strings.Contains(lower, "4k") || strings.Contains(lower, "2160") {
		return "4K"
	}
	if strings.Contains(lower, "fullhd") || strings.Contains(lower, "fhd") {
		return "1080p"
	}
	for _, res := range qualityResolutionTokens {
		if strings.Contains(lower, res) {
			return res + "p"
		}
	}
	if strings.EqualFold(lower, "hd") {
		return "720p"
	}
	if strings.EqualFold(q, "auto") || strings.EqualFold(q, "авто") {
		return "Auto"
	}
	if q != "" {
		return q
	}
	return defaultStreamQuality
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

// ---- Balancer identification & candidate detection ----

// detectPlayerBalancer maps candidate host/path to standard human-readable names.
func detectPlayerBalancer(rawURL string) string {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, "ashdi"):
		return "Ashdi"
	case strings.Contains(lower, "zenith"):
		return "Zenith"
	case strings.Contains(lower, "hdvb") || strings.Contains(lower, "vidcache") || strings.Contains(lower, "streamcdn"):
		return "HDVB"
	case strings.Contains(lower, "videohost"):
		return "Videohost"
	case strings.Contains(lower, "tortuga"):
		return "Tortuga"
	case strings.Contains(lower, "kodik"):
		return "Kodik"
	case strings.Contains(lower, "collaps"):
		return "Collaps"
	case strings.Contains(lower, "goodplay"):
		return "Goodplay"
	case strings.Contains(lower, "hydra"):
		return "Hydra"
	case strings.Contains(lower, "okdictator"):
		return "Okdictator"
	case strings.Contains(lower, "uakino"):
		return "UAKino"
	case strings.Contains(lower, "eneyida"):
		return "Eneyida"
	case strings.Contains(lower, "lavakino"):
		return "Lavakino"
	default:
		if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
			parts := strings.Split(u.Host, ".")
			if len(parts) >= 2 {
				name := parts[len(parts)-2]
				if len(name) > 0 {
					return strings.ToUpper(name[:1]) + name[1:]
				}
			}
			return u.Host
		}
		return "Плеєр"
	}
}

// iframeHints are substrings typical for the hosts/paths that actually host a DLE player.
var iframeHints = []string{
	"player", "embed", "ashdi", "zenith", "hdvbua", "hdvb", "vidcache", "streamcdn",
	"videohost", "kodik", "megogo", "okdictator", "goodplay", "hydra", "collaps",
	"tortuga", "hls", "m3u8", "mp4", "video", "media", "cdn", "stream",
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

// PlayerCandidate represents one discovered player iframe or player tab on an item page.
type PlayerCandidate struct {
	URL   string
	Label string
	Score int
}

// candidateDataAttrs are the attributes a DLE theme can hang a player URL on.
// Package level: this was a slice literal inside a per-node loop, i.e. one
// allocation for each of the ~1000 nodes the selector below matches.
var candidateDataAttrs = []string{"data-src", "data-player", "data-url", "data-iframe", "data-link", "value"}

// candidateSelector is the theme-agnostic set of elements that can carry a
// player URL in an attribute (tabs, buttons, select options).
const candidateSelector = "li, button, a, div, span, option"

// hasURIPrefix reports whether s can be resolved against an item URL.
func hasURIPrefix(s string) bool {
	return strings.HasPrefix(s, "//") || strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "/")
}

// playerURLScan is the per-candidate view shared by the plausibility test and
// the scorer. Previously each of those re-lowercased the URL and re-parsed it
// (hostOf was called twice inside a single expression plus url.Parse for the
// media-extension probe), so one candidate cost three parses and two ~48-entry
// substring scans.
type playerURLScan struct {
	lower string
	host  string
	path  string
	skip  bool // matches an iframeSkips entry
	hint  bool // matches an iframeHints entry
}

func scanPlayerURL(raw string) playerURLScan {
	s := playerURLScan{lower: strings.ToLower(raw)}
	if u, err := url.Parse(raw); err == nil {
		s.host = u.Host
		s.path = u.Path
	}
	for _, skip := range iframeSkips {
		if strings.Contains(s.lower, skip) {
			s.skip = true
			break
		}
	}
	// A skipped URL is rejected by both callers, so the hint scan is dead work.
	if !s.skip {
		for _, hint := range iframeHints {
			if strings.Contains(s.lower, hint) {
				s.hint = true
				break
			}
		}
	}
	return s
}

// isPlausiblePlayerOrMedia checks whether a raw string looks like a player or media link.
func isPlausiblePlayerOrMedia(s string) bool {
	if !hasURIPrefix(s) {
		return false
	}
	scan := scanPlayerURL(s)
	return !scan.skip && scan.hint
}

// rankPlayerCandidates extracts all player candidates from tabs, buttons, data-attributes
// and iframes on the page, with appropriate labels and priority scoring.
func rankPlayerCandidates(html, itemURL string) []PlayerCandidate {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	itemHost := hostOf(itemURL)
	var candidates []PlayerCandidate
	seen := make(map[string]bool)

	addCandidate := func(rawSrc, rawLabel string, bonusScore int) {
		abs := absolutizeURL(rawSrc, itemURL)
		if abs == "" || seen[abs] {
			return
		}
		score := scorePlayerIframe(abs, itemHost) + bonusScore
		if score <= 0 {
			return
		}
		seen[abs] = true

		label := strings.TrimSpace(rawLabel)
		balancer := detectPlayerBalancer(abs)
		if label == "" {
			label = balancer
		} else if !strings.Contains(strings.ToLower(label), strings.ToLower(balancer)) && balancer != "Плеєр" {
			label = label + " (" + balancer + ")"
		}

		candidates = append(candidates, PlayerCandidate{
			URL:   abs,
			Label: label,
			Score: score,
		})
	}

	// 1. Scan player tabs, buttons, select options and data attributes (e.g. Lavakino, Uakino tabs)
	doc.Find(candidateSelector).Each(func(_ int, s *goquery.Selection) {
		for _, attr := range candidateDataAttrs {
			if val, ok := s.Attr(attr); ok && strings.TrimSpace(val) != "" {
				val = strings.TrimSpace(val)
				if isPlausiblePlayerOrMedia(val) {
					text := strings.TrimSpace(s.Text())
					if text == "" {
						text, _ = s.Attr("title")
					}
					addCandidate(val, text, 3)
				}
			}
		}
	})

	// 2. Scan all iframes
	doc.Find("iframe").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			src = strings.TrimSpace(s.AttrOr("data-src", ""))
		}
		if src == "" {
			src = strings.TrimSpace(s.AttrOr("data-player", ""))
		}
		if src == "" {
			return
		}

		label := strings.TrimSpace(s.AttrOr("title", ""))
		if label == "" {
			label = strings.TrimSpace(s.AttrOr("name", ""))
		}
		if label == "" {
			parentTab := s.Closest(".tab, .tabs-b, .player-box, .tab-pane, div[id*='tab']")
			if parentTab.Length() > 0 {
				label = strings.TrimSpace(parentTab.AttrOr("data-title", ""))
			}
		}
		addCandidate(src, label, 0)
	})

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	return candidates
}

// rankPlayerIframes returns candidate player iframe URLs ordered best first and de-duplicated.
func rankPlayerIframes(html, itemURL string) []string {
	candidates := rankPlayerCandidates(html, itemURL)
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.URL)
	}
	return out
}

// scorePlayerIframe returns 0 for frames that must be skipped, otherwise a positive
// priority: a cross-host iframe is the usual player, and known player hints push it up.
func scorePlayerIframe(abs, itemHost string) int {
	return scanPlayerURL(abs).score(itemHost)
}

// score applies the same ranking rules as scorePlayerIframe to an
// already-computed scan.
func (s playerURLScan) score(itemHost string) int {
	if s.skip {
		return 0
	}

	score := 1
	if s.host != "" && s.host != itemHost {
		score += 4
	}
	if s.hint {
		score += 4
	}
	if reMediaExt.MatchString(s.path) {
		score += 2
	}
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

// ---- Multi-Quality, Playlist & Subtitle Parsers ----

// parseMultiQualityString parses strings like:
// "[1080p]https://cdn/1080.m3u8,[720p]https://cdn/720.m3u8"
// or "[1080p]https://cdn/1080.m3u8 or [720p]https://cdn/720.m3u8"
// or a single URL.
//
// No error channel: nil means "this input carried no media URL at all" (nothing
// matched), while a slice shorter than the number of comma/or-separated parts
// means "parsed but partially rejected" — a part failed isPlayableMediaURL or
// duplicated an earlier URL. Callers cannot distinguish the two, which is why
// the caller falls through to the next strategy on nil and accepts a short
// result as final.
func parseMultiQualityString(raw, playerURL, playerLabel string) []domain.StreamSource {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var parts []string
	if strings.Contains(raw, " or ") {
		parts = strings.Split(raw, " or ")
	} else if strings.Contains(raw, ",[") {
		parts = strings.Split(raw, ",")
	} else if strings.Contains(raw, ",") && strings.Contains(raw, "http") {
		parts = strings.Split(raw, ",")
	} else {
		parts = []string{raw}
	}

	var sources []domain.StreamSource
	seenURLs := make(map[string]bool)

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var qLabel, mediaURL string
		if m := reQualityBracket.FindStringSubmatch(p); len(m) == 3 {
			qLabel = strings.TrimSpace(m[1])
			mediaURL = strings.TrimSpace(m[2])
		} else {
			mediaURL = p
		}

		if !isPlayableMediaURL(mediaURL) || seenURLs[mediaURL] {
			continue
		}
		seenURLs[mediaURL] = true

		src := newStreamSource(mediaURL, playerURL)
		if qLabel != "" {
			src.Quality = normalizeQualityLabel(qLabel)
		}
		src.Player = playerLabel
		src.Voiceover = playerLabel
		sources = append(sources, src)
	}

	return sources
}

// playerJSPlaylistItem represents a node in a PlayerJS JSON playlist tree.
type playerJSPlaylistItem struct {
	Title    string                 `json:"title"`
	File     json.RawMessage        `json:"file"`
	Folder   []playerJSPlaylistItem `json:"folder"`
	Subtitle string                 `json:"subtitle"`
}

// parseSeasonOrEpisodeNum extracts an integer number from a title string (e.g. "1 сезон", "Серія 2").
func parseSeasonOrEpisodeNum(text string, fallback int) int {
	text = strings.TrimSpace(text)
	if match := reDigits.FindString(text); match != "" {
		if n, err := strconv.Atoi(match); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// extractStreamsFromPlaylistTree searches a PlayerJS playlist structure for streams
// matching the requested season, episode and voice.
//
// No error channel, same convention as parseMultiQualityString: two nils mean
// "no episode matched the request AND the first-available fallback also yielded
// nothing"; a nil stream slice with non-nil subs means "subtitles only"; a
// non-nil stream slice means the request or the fallback produced streams. A
// season/episode miss is therefore indistinguishable from a genuine absence.
func extractStreamsFromPlaylistTree(items []playerJSPlaylistItem, playerURL, playerLabel string, season, episode int, voiceID string) ([]domain.StreamSource, []domain.SubtitleSource) {
	var streams []domain.StreamSource
	var subs []domain.SubtitleSource

	if len(items) == 0 {
		return nil, nil
	}

	// Helper to extract file content (which may be multi-quality or direct URL)
	processItem := func(it playerJSPlaylistItem, voiceName string) {
		var fileStr string
		if err := json.Unmarshal(it.File, &fileStr); err == nil && fileStr != "" {
			itemLabel := playerLabel
			if voiceName != "" {
				itemLabel = voiceName
				if playerLabel != "" && !strings.Contains(voiceName, playerLabel) {
					itemLabel = voiceName + " (" + playerLabel + ")"
				}
			}
			st := parseMultiQualityString(fileStr, playerURL, itemLabel)
			streams = append(streams, st...)
		}
		if it.Subtitle != "" {
			parsedSubs := parseSubtitlesFromPlayerHTML(it.Subtitle)
			subs = append(subs, parsedSubs...)
		}
	}

	// Check if this is a flat list of qualities (e.g. [{title: "1080p", file: "..."}, {title: "720p", file: "..."}])
	isFlatQualityList := true
	for _, it := range items {
		if len(it.Folder) > 0 || len(it.File) == 0 {
			isFlatQualityList = false
			break
		}
	}
	if isFlatQualityList {
		for _, it := range items {
			var fileStr string
			if err := json.Unmarshal(it.File, &fileStr); err == nil && fileStr != "" {
				st := parseMultiQualityString(fileStr, playerURL, playerLabel)
				for i := range st {
					if it.Title != "" {
						st[i].Quality = normalizeQualityLabel(it.Title)
					}
					streams = append(streams, st[i])
				}
			}
		}
		return streams, subs
	}

	// Series folder tree traversal
	for sIdx, rootItem := range items {
		// If root represents a voice/dub (e.g. "1+1", "Незупиняй", "HDVB")
		voiceName := ""
		currentFolders := rootItem.Folder
		if len(currentFolders) > 0 && (strings.Contains(rootItem.Title, "сезон") || strings.Contains(rootItem.Title, "Сезон")) {
			// Root is a Season
			sNum := parseSeasonOrEpisodeNum(rootItem.Title, sIdx+1)
			if season > 0 && sNum != season {
				continue
			}
			for eIdx, epItem := range currentFolders {
				eNum := parseSeasonOrEpisodeNum(epItem.Title, eIdx+1)
				if episode > 0 && eNum != episode {
					continue
				}
				processItem(epItem, voiceName)
			}
		} else if len(currentFolders) > 0 {
			// Root is a Voiceover Studio
			voiceName = rootItem.Title
			if voiceID != "" && !strings.EqualFold(voiceName, voiceID) {
				continue
			}
			for sIdx2, sItem := range currentFolders {
				if len(sItem.Folder) > 0 {
					sNum := parseSeasonOrEpisodeNum(sItem.Title, sIdx2+1)
					if season > 0 && sNum != season {
						continue
					}
					for eIdx2, epItem := range sItem.Folder {
						eNum := parseSeasonOrEpisodeNum(epItem.Title, eIdx2+1)
						if episode > 0 && eNum != episode {
							continue
						}
						processItem(epItem, voiceName)
					}
				} else {
					// Flat episodes under voice
					eNum := parseSeasonOrEpisodeNum(sItem.Title, sIdx2+1)
					if episode > 0 && eNum != episode {
						continue
					}
					processItem(sItem, voiceName)
				}
			}
		} else {
			// Flat episode list
			eNum := parseSeasonOrEpisodeNum(rootItem.Title, sIdx+1)
			if episode > 0 && eNum != episode {
				continue
			}
			processItem(rootItem, "")
		}
	}

	// Fallback if requested episode not found: extract first available item
	if len(streams) == 0 && len(items) > 0 {
		var firstItem *playerJSPlaylistItem
		if len(items[0].Folder) > 0 {
			if len(items[0].Folder[0].Folder) > 0 {
				firstItem = &items[0].Folder[0].Folder[0]
			} else {
				firstItem = &items[0].Folder[0]
			}
		} else {
			firstItem = &items[0]
		}
		if firstItem != nil {
			processItem(*firstItem, items[0].Title)
		}
	}

	return streams, subs
}

// parseSubtitlesFromPlayerHTML extracts subtitles from subtitle: "..." or <track> tags.
func parseSubtitlesFromPlayerHTML(text string) []domain.SubtitleSource {
	var subs []domain.SubtitleSource
	seen := make(map[string]bool)

	add := func(u, label, lang string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		if label == "" {
			label = lang
		}
		if label == "" {
			label = "Субтитри"
		}
		subs = append(subs, domain.SubtitleSource{
			URL:      u,
			Label:    label,
			Language: lang,
		})
	}

	// 1. subtitle: "[Українська]https://...,[English]https://..."
	for _, m := range rePlayerJSSubtitle.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		raw := m[1]
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if bm := reQualityBracket.FindStringSubmatch(part); len(bm) == 3 {
				add(bm[2], bm[1], bm[1])
			} else if strings.HasPrefix(part, "http") {
				add(part, "Субтитри", "uk")
			}
		}
	}

	// 2. <track kind="subtitles" src="..." label="..." srclang="...">
	for _, trackTag := range reTrackTag.FindAllString(text, -1) {
		srcMatch := reSrcAttr.FindStringSubmatch(trackTag)
		if len(srcMatch) < 2 {
			continue
		}
		label := ""
		if lm := reLabelAttr.FindStringSubmatch(trackTag); len(lm) >= 2 {
			label = lm[1]
		}
		lang := ""
		if sm := reSrclangAttr.FindStringSubmatch(trackTag); len(sm) >= 2 {
			lang = sm[1]
		}
		add(srcMatch[1], label, lang)
	}

	return subs
}

// extractAllStreamsFromPlayer probes a single player URL and extracts all available
// streams (multi-quality, playlist items) and subtitles.
func extractAllStreamsFromPlayer(ctx context.Context, client *TLSClient, playerURL, siteBaseURL, defaultLabel string, season, episode int, voiceID string) ([]domain.StreamSource, []domain.SubtitleSource, error) {
	if strings.TrimSpace(playerURL) == "" {
		return nil, nil, ErrUnresolvablePlayer
	}

	page, err := client.Get(ctx, playerURL, siteBaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch player page %s: %w", playerURL, err)
	}

	playerLabel := strings.TrimSpace(defaultLabel)
	if playerLabel == "" {
		playerLabel = detectPlayerBalancer(playerURL)
	}

	var allStreams []domain.StreamSource
	var allSubs []domain.SubtitleSource
	seenStreams := make(map[string]bool)

	addStreams := func(streams []domain.StreamSource) {
		for _, s := range streams {
			if !seenStreams[s.URL] {
				seenStreams[s.URL] = true
				if s.Player == "" {
					s.Player = playerLabel
				}
				if s.Voiceover == "" {
					s.Voiceover = playerLabel
				}
				allStreams = append(allStreams, s)
			}
		}
	}

	// 1. Extract subtitles
	subs := parseSubtitlesFromPlayerHTML(page)
	allSubs = append(allSubs, subs...)

	// 2. Strategy A: Check for PlayerJS JSON playlist tree in page or decoded base64
	for _, jm := range rePlayerJSJSONFile.FindAllStringSubmatch(page, -1) {
		if len(jm) >= 2 {
			var playlistItems []playerJSPlaylistItem
			if err := json.Unmarshal([]byte(jm[1]), &playlistItems); err == nil && len(playlistItems) > 0 {
				st, sb := extractStreamsFromPlaylistTree(playlistItems, playerURL, playerLabel, season, episode, voiceID)
				addStreams(st)
				allSubs = append(allSubs, sb...)
			}
		}
	}

	// 3. Strategy B: Base64 PlayerJS payload decode
	for _, b64m := range reBase64File.FindAllStringSubmatch(page, -1) {
		if len(b64m) >= 2 {
			if decoded, err := decodeBase64Loose(b64m[1]); err == nil {
				// Try playlist JSON in decoded base64
				var playlistItems []playerJSPlaylistItem
				if err := json.Unmarshal([]byte(decoded), &playlistItems); err == nil && len(playlistItems) > 0 {
					st, sb := extractStreamsFromPlaylistTree(playlistItems, playerURL, playerLabel, season, episode, voiceID)
					addStreams(st)
					allSubs = append(allSubs, sb...)
				}
				// Also check PlayerJS file & sources in decoded text
				if fm := rePlayerJSFile.FindStringSubmatch(decoded); len(fm) >= 2 {
					st := parseMultiQualityString(fm[1], playerURL, playerLabel)
					addStreams(st)
				}
				// Also check subtitles in decoded text
				allSubs = append(allSubs, parseSubtitlesFromPlayerHTML(decoded)...)
			}
		}
	}

	// 4. Strategy C: Plain PlayerJS file: "[1080p]https://..."
	if len(allStreams) == 0 {
		for _, fm := range rePlayerJSFile.FindAllStringSubmatch(page, -1) {
			if len(fm) >= 2 {
				st := parseMultiQualityString(fm[1], playerURL, playerLabel)
				addStreams(st)
			}
		}
	}

	// 5. Strategy D: Sources block with multiple sources
	for _, block := range reSourcesBlock.FindAllStringSubmatch(page, -1) {
		if len(block) < 2 {
			continue
		}
		for _, sm := range reSourcesSrc.FindAllStringSubmatch(block[1], -1) {
			if len(sm) >= 2 && isPlayableMediaURL(sm[1]) {
				src := newStreamSource(sm[1], playerURL)
				src.Player = playerLabel
				src.Voiceover = playerLabel
				if lm := reSourcesLabel.FindStringSubmatch(block[1]); len(lm) >= 2 {
					src.Quality = normalizeQualityLabel(lm[1])
				}
				addStreams([]domain.StreamSource{src})
			}
		}
	}

	// 6. Strategy E: Hls.loadSource
	if len(allStreams) == 0 {
		if hlsSrc, ok := firstCaptured(reHlsLoadSource, page); ok && isPlayableMediaURL(hlsSrc) {
			src := newStreamSource(hlsSrc, playerURL)
			src.Player = playerLabel
			src.Voiceover = playerLabel
			addStreams([]domain.StreamSource{src})
		}
	}

	// 7. Strategy F: Bare media URL fallback
	if len(allStreams) == 0 {
		if bareSrc, ok := firstCaptured(reBareMediaURL, page); ok && isPlayableMediaURL(bareSrc) {
			src := newStreamSource(bareSrc, playerURL)
			src.Player = playerLabel
			src.Voiceover = playerLabel
			addStreams([]domain.StreamSource{src})
		}
	}

	if len(allStreams) == 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnresolvablePlayer, playerURL)
	}

	return allStreams, allSubs, nil
}

// playerProbeResult is one candidate's outcome, indexed by candidate rank so the
// merged response keeps the ranked preference regardless of completion order.
type playerProbeResult struct {
	streams []domain.StreamSource
	subs    []domain.SubtitleSource
	err     error
}

// probePlayerCandidates fetches every candidate with a bounded-parallel fan-out.
//
// Semantics preserved from the sequential version: EVERY candidate is probed (not
// just the first success) and results are merged in rank order, de-duplicated by
// URL. Only the wall-clock changed: latency is now the max of the round-trips
// instead of their sum, and each candidate carries its own playerCandidateTimeout
// sub-budget so a hung host cannot consume the whole PlayerResolveTimeout.
func probePlayerCandidates(ctx context.Context, client *TLSClient, itemURL string, candidates []PlayerCandidate, season, episode int, voiceID string) []playerProbeResult {
	results := make([]playerProbeResult, len(candidates))
	sem := make(chan struct{}, playerFanoutConcurrency)
	var wg sync.WaitGroup

	for i, cand := range candidates {
		wg.Add(1)
		go func(idx int, c PlayerCandidate) {
			defer wg.Done()

			// Respect an already-cancelled caller instead of queueing for a slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[idx] = playerProbeResult{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			// Per-candidate sub-budget: one hung player host must not consume the
			// shared PlayerResolveTimeout and starve the candidates behind it.
			candCtx, cancel := context.WithTimeout(ctx, playerCandidateTimeout)
			defer cancel()

			streams, subs, err := extractAllStreamsFromPlayer(candCtx, client, c.URL, itemURL, c.Label, season, episode, voiceID)
			results[idx] = playerProbeResult{streams: streams, subs: subs, err: err}
		}(i, cand)
	}

	wg.Wait()
	return results
}

// resolveStreamsFromItemPage is the shared engine for all DLE GetStreams implementations.
// It discovers all player candidates on the page, fetches them up to MaxPlayerIframes,
// collects all playable streams across all balancers/qualities/dubbings, merges subtitles,
// and returns a complete, unified stream response.
func resolveStreamsFromItemPage(ctx context.Context, client *TLSClient, providerID, itemURL, html string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	resp := &domain.ContentStreamsResponse{
		ProviderID: providerID,
		Streams:    []domain.StreamSource{},
		Subtitles:  []domain.SubtitleSource{},
	}

	candidates := rankPlayerCandidates(html, itemURL)
	if len(candidates) > MaxPlayerIframes {
		candidates = candidates[:MaxPlayerIframes]
	}

	seenStreamURLs := make(map[string]bool)
	seenSubURLs := make(map[string]bool)
	var lastErr error

	for _, res := range probePlayerCandidates(ctx, client, itemURL, candidates, season, episode, voiceID) {
		if res.err != nil {
			lastErr = res.err
			continue
		}

		for _, s := range res.streams {
			if !seenStreamURLs[s.URL] {
				seenStreamURLs[s.URL] = true
				resp.Streams = append(resp.Streams, s)
			}
		}
		for _, sub := range res.subs {
			if !seenSubURLs[sub.URL] {
				seenSubURLs[sub.URL] = true
				resp.Subtitles = append(resp.Subtitles, sub)
			}
		}
	}

	if len(resp.Streams) == 0 {
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

	return resp, nil
}
