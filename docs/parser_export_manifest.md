# Parser export manifest

Status: **exported out of the app repository on 2026-10-05 (branch `dev`).**

The scrapers that used to live under `plugins/sidecar-scrapers/internal/` in this
repository have been deleted from it. They were verified working end-to-end
immediately before deletion: a four-season series resolved to 155 episodes with
real dubbing-studio names and three real CDN qualities. That verification is the
reason this document is long.

This file is the export record. Someone picking the work up on the community side
should be able to reproduce a working provider from this document plus the source
files listed in §1, without re-reading this repository's git history.

## How to read this document

* Provider names (`uakino`, `eneyida`, `lavakino`, `uaserials`, `bandera`,
  `tortuga`, `ashdi`, `hdvb`, `zenith`) appear as plain identifiers because they
  are needed to identify a file or a technique. They are not links and are not
  examples to click.
* Real hostnames and live media URLs are deliberately **not** reproduced here.
  Where a host matters, it is described structurally ("the provider's own domain",
  "the CDN host the player resolves to"). The host constants live in the deleted
  source, which the community repository will receive directly.
* Every claim in §3 was checked against a live site on **2026-10-05** and is
  cited to `path:line` in the deleted source so it can be re-checked during the
  port.

---

## 1. File inventory

76 files, 24 884 lines total, all tracked in git and all deleted by a single
`git rm -r plugins/`:

* **65 Go files**, 24 235 lines
* **11 fixture files** under `provider/testdata/`

There is no `go.mod` under `plugins/`. The tree was never a buildable module of
its own — it imported `github.com/edhases/oxide-server/internal/{domain,search}`
and was compiled as part of the backend module. A port therefore also has to
supply the `domain` types (see §5).

### 1a. `provider/` — scrapers and shared extraction engine

| File | Lines | Purpose |
|---|---:|---|
| `bandera.go` | 729 | Aggregator-API provider (Lampa-style `/api/v2` sources catalogue). Search, catalogue, details, stream resolution across a rotating source list. |
| `bandera_client.go` | 425 | HTTP client for the aggregator: singleflight-coalesced `GET /sources`, TTL cache, stale-while-error fallback, response body cap. |
| `bandera_types.go` | 588 | Lenient JSON decoding for aggregator payloads (`FlexibleString` accepts string/number/bool/null), source metadata, series and episode models. |
| `bandera_select.go` | 152 | Chooses between stream refs (`/stream`) and content refs (`/content`) using each source's declared `streamKeys`/`contentKeys`. |
| `bandera_normalize.go` | 137 | Packed-URL unpacking (`[ 720p ] https://…`), JS-whitespace normalisation, per-CDN proxy policy (which hosts need a proxy, which must not have Referer/Origin). |
| `client.go` | 531 | `TLSClient`: TLS-fingerprinted HTTP client, charset detection, body cap, redirect policy, User-Agent constant, `GetNoCache`/`PostForm`. |
| `resolve.go` | 1287 | **The shared player resolver.** Ordered extraction strategies over player HTML, iframe candidate ranking, balancer detection, multi-quality strings, subtitle parsing, `resolveStreamsFromItemPage` entry point. |
| `playlist_tree.go` | 617 | PlayerJS playlist tree model: recursive walk, node role inference (season / episode / dub), season+voiceover building, `SelectPlaylistStream`. |
| `playerjs_scan.go` | 226 | Byte-accurate bracket scanner that locates `file:` values inside player HTML and decodes the embedded PlayerJS JSON. |
| `tortuga_cdn.go` | 673 | Playlist-CDN extractor: cipher decode, URL sanitising, trailer pruning, per-episode fetch, master-playlist quality expansion. |
| `tortuga_format.go` | 99 | Parses the `{dub}url(subtitle:…)` `file:` value format into dub / media URL / subtitles. |
| `hls_variants.go` | 240 | HLS master-playlist parsing and expansion into one source per declared variant. |
| `uaserials.go` | 1199 | Encrypted-tag provider: catalogue, search, details, AES tag decryption, tab selection, stream resolution. |
| `eneyida.go` | 561 | DLE provider: catalogue, details, plus the shared DLE playlist-tree helpers (`applyPlaylistDetails`, `mergePlaylistSeasons`). |
| `uakino.go` | 432 | DLE provider: catalogue, details, stream fan-out. Also hosts `parseYear` and the shared rating regex. |
| `lavakino.go` | 347 | DLE provider: catalogue (form POST search), details, stream fan-out. |
| `studios.go` | 234 | Dubbing-studio name normalisation: bracketed names to canonical IDs, alias resolution. |
| `studios_data.go` | 137 | The studio table — canonical IDs, display names, aliases. Port this verbatim; it is the mapping the client-facing IDs depend on. |
| `registry.go` | 442 | Provider registry: registration, enable/disable kill-switch, panic isolation, health tracking, coalesced search fan-out. |
| `selection_ref.go` | 163 | Encodes/decodes the opaque episode selection ref (`<voice>:<season>:<episode>:<dub>` with a JSON long form). |
| `dub_weight.go` | 126 | Orders streams by dubbing priority (subtitles-only < original < mono < two-voice < multi < full dub) and quality. |

### 1b. `provider/` — tests

| File | Lines | Purpose |
|---|---:|---|
| `bandera_test.go` | 717 | Aggregator provider behaviour, proxy rules per CDN host. |
| `bandera_coverage_test.go` | 1349 | Broad aggregator coverage: catalogue, details, stream selection, error paths. |
| `bandera_flexible_seasons_test.go` | 235 | Season structures that differ per source. |
| `bandera_ok_field_test.go` | 134 | `ok`-flag handling on aggregator responses. |
| `bandera_stable_id_test.go` | 102 | Stable item IDs across ref shapes. |
| `bandera_voiceid_test.go` | 129 | Voiceover ID derivation. |
| `bandera_voiceover_test.go` | 122 | Voiceover naming from aggregator payloads. |
| `client_test.go` | 268 | TLS client behaviour: charset, body cap, redirects. |
| `client_coverage_test.go` | 176 | Client edge cases. |
| `eneyida_catalogue_test.go` | 856 | Catalogue card parsing, section slugs, playlist probing, the live-selector regressions. |
| `lavakino_test.go` | 165 | Catalogue and details parsing. |
| `lavakino_coverage_test.go` | 382 | Error paths and stream fan-out. |
| `uakino_test.go` | 49 | Small provider surface tests. |
| `uakino_coverage_test.go` | 609 | Section slugs, playlist details, stream selection. |
| `uakino_fixture_test.go` | 121 | Fixture-driven catalogue test — asserts real titles, years, types, poster joins. |
| `uaserials_test.go` | 982 | Tag decryption, tab selection, catalogue and search card parsing, key-bundle obfuscation. |
| `hdrezka_coverage_test.go` | 20 | Identity/name assertions for the sibling DLE provider. |
| `hls_variants_test.go` | 248 | Master-playlist parsing, variant ordering, quality labels. |
| `playlist_tree_test.go` | 702 | Tree walk, season/episode numbering, trailer detection, selection refs. |
| `resolve_test.go` | 646 | Strategy order, iframe ranking, multi-player fan-out, subtitles. |
| `selection_ref_test.go` | 231 | Ref encode/decode round-trips. |
| `quality_contract_test.go` | 195 | **Contract tests for `Quality` vs `Voiceover` vs `Player`** — the fields must not be conflated. Read this before touching stream construction. |
| `provider_errors_coverage_test.go` | 123 | Fetch-then-parse error propagation. |
| `provider_http_coverage_test.go` | 408 | In-package HTTP-level coverage with `httptest` servers. |
| `registry_test.go` | 67 | Registry basics. |
| `registry_coverage_test.go` | 257 | Registry edge cases. |
| `registry_ctx_test.go` | 374 | Context cancellation through the registry. |
| `registry_dispatch_test.go` | 489 | Per-provider dispatch and fan-out budgets. |
| `perf_bandera_test.go` | 353 | Concurrency and body-size regressions for the aggregator client. |
| `perf_parse_test.go` | 101 | `parseYear` regex-hoist semantics and goroutine safety. |
| `perf_resolve_test.go` | 260 | Iframe fan-out latency regression. |
| `parse_year_test.go` | 24 | Year extraction edge cases. |
| `worker_proxy_test.go` | 219 | Media proxy worker behaviour. |
| `export_test.go` | 39 | Test-only hooks for package-internal functions. |

### 1c. `search/` — search pipeline

| File | Lines | Purpose |
|---|---:|---|
| `plan.go` | 165 | Query normalisation: canonical form, tokenisation, year extraction, type hint, noise-word stripping, SHA1 hash. |
| `score.go` | 174 | Relevance scoring with a hard cutoff; returns score, match reason, dropped flag. |
| `cluster.go` | 262 | Groups results from several sources into one title, records per-source refs and source status. |
| `search_test.go` | 102 | Plan and score basics. |
| `search_relevance_test.go` | 642 | Relevance behaviour. |
| `cluster_year_test.go` | 153 | Cluster merge with year disagreements. |
| `confusables_test.go` | 83 | Homoglyph / confusable title handling. |

### 1d. `transport/http/` — HTTP surface

| File | Lines | Purpose |
|---|---:|---|
| `content_handler.go` | 366 | Content endpoints, cache repo interface, singleflight on cache misses. |
| `search_pipeline.go` | 532 | Search endpoint orchestration: fan-out, clustering, TTLs, cache policy, per-source degradation. |

---

## 2. Fixtures — `provider/testdata/`

**All 11 fixture files were captured from live sites on 2026-10-05.** They are
the only record of what those sites actually returned. They are required for the
porting work and must be carried over with the source; do not regenerate them
from imagination.

They are not decorative. Each one exists because the first parser version passed
its tests and still failed on the live site.

| Fixture | Lines | What real response it holds |
|---|---:|---|
| `uakino_catalog.html` | 144 | Real catalogue markup for the four live sections (films / series / cartoon / anime). Seven cards: each pins one parsing rule — the year lives in a `Рік виходу:` row (not in `.movie-date`); an anime path contains both "anime" and "series" so check order matters; a cartoon path contains both "cartoon" and "series"; one poster exists only in `data-src`; one card has no `href` and must be skipped. |
| `eneyida_catalog.html` | 97 | Real catalogue markup, seven cards. Its whole purpose is that `.short_title` **is** the `<a>`, not an `<h2>` wrapping one — see §3.2. Also carries `.metaBottom` episode badges and `.short_subtitle` with a year link plus `&bull;`-separated original title. |
| `eneyida_item_series.html` | 86 | Real series description page: bare `<li>` rows inside `ul.full_info` (not `.full_info-item`), the actor row labelled `В ролях:`, and the description container `.full_content-desc` — `.full-text` on this site is the comment block. |
| `eneyida_player_hdvb.html` | 25 | Real player page behind the iframe: a PlayerJS `file:` holding a two-season tree. This is the proof that the playlist lives in the player iframe, not on the item page. |
| `uaserials_catalog.html` | 27 | Real catalogue cards (`.short-item`, `a.short-img`, `.th-title` as a `<div>`, lazy `data-src` posters with `/media/default.png` fallbacks, `.short-label-level-1` season label). |
| `uaserials_search.html` | 6 | Real search-results page. Five real `a.uas-card` cards **plus** a `uas-people` block of `/person/…` links that carry `data-uas-id` too — so ID collection must select `a.uas-card[data-uas-id]`, not "anything with a data-uas-id". |
| `uaserials_roksolana.html` | 94 | Real description page for the four-season series used in the end-to-end verification. Carries the `<player-control data-tag1=…>` element whose value is the encrypted tab list. |
| `uaserials_tag_vector.json` | 10 | The real `data-tag` JSON (`ciphertext` / `iv` / `salt`) from that page plus the passphrase that decrypts it. This is the **test vector for the AES implementation** — keep it and its plaintext expectation together or the decryptor is unverifiable. |
| `tortuga_encoded_sample.txt` | 1 | One long line: a real encrypted `file:` value, base64 with **stripped padding**. Round-trip it to prove the XOR decode. |
| `tortuga_roksolana.json` | 86 | The decrypted PlayerJS tree from the same player, truncated: season → episode nesting with `{dub}url(subtitle:…)` file values and studio names in node titles. |
| `tortuga_vod_plain.txt` | 73 | A **plain, unencrypted** playlist from an older-style player page. Needed because not every player page encrypts; a decrypt-only implementation would wrongly discard these. |

---

## 3. Non-obvious knowledge

This is the part that cannot be recovered by reading the code later. Each item
was verified live.

### 3.1 uakino — the playlist is AJAX-loaded

Scraping the static HTML of a description page yields **empty seasons**. The
item page renders a player-tab container (`UA #1` / `#torrent`) whose content is
filled in by client-side JavaScript; the PlayerJS tree is not in the response.
Measured on a real description page: 2 iframes (the only one with `src` being a
YouTube embed for the trailer), and zero occurrences of `file:`, `sources:`,
`Hls.loadSource` or `.m3u8`.

Consequence for a port: a pure static parse of the item page cannot produce
seasons for this site. `GetDetails` deliberately leaves the season fields empty
in that case — empty is correct, invented seasons are not — and the stream path
resolves the player through the iframe instead
(`uakino.go:404-411`, `uakino.go:428-438`).

The container also carries `data-lebclicks="yes"`, which is the DLE marker for a
click-counting ad/player wrapper — see §3.7.

Section slugs, verified live (200): `/filmy/`, `/seriesss/` (**three** s — not a
typo), `/cartoon/`, `/animeukr/`. Confirmed 404: `/multfilmy/`, `/anime/`,
`/serials/`, `/cartoonss/`, `/animes/` (`uakino.go:120-144`).

### 3.2 eneyida — catalogue section selectors 404, and `.short_title` is an `<a>`

Two separate defects, both of which made the catalogue silently empty while the
tests stayed green.

**(a) Dead section slugs.** The old values `serials` and `multfilmy` both return
404, which is why the series and cartoon sections were empty. Live 200:
`/films/`, `/series/`, `/cartoon/`, `/cartoon-series/`, `/anime/`
(`eneyida.go:103-128`, asserted in `eneyida_catalogue_test.go:110-148`).

**(b) `.short_title` is the anchor itself.** The live markup is

```html
<a class="short_title" id="short_title" href="…">Title</a>
```

The old selector `h2.short_title a, .short_title a` searched for a descendant
*inside itself* and matched **zero** cards. The correct selector puts
`a.short_title` first (`eneyida.go:151-157`). The fixture header says the same
thing in prose: the old selector passed on invented markup `<h2 class="short_title"><a…>`
and failed on the real page, so the test was green and the catalogue was empty.

Two more eneyida-specific traps in the same area: the description is
`.full_content-desc`, because `.full-text` is the **comments** block; and the
actor row is labelled `В ролях:`, not `Актори:` (`eneyida.go:307`, `:326`, `:394`).

### 3.3 uaserials — search uses `a.uas-card`, catalogue uses `.short-item`

The search results page does **not** use the catalogue card markup at all:

* catalogue → `.short-item`, link in `a.short-img`, title in `.th-title`
  (which is a `<div>`, so `.th-title a` matches nothing)
* search → `a.uas-card`, title in `.uas-card__title`, id in `data-uas-id`

A parser that only knows `.short-item` returns zero search results with no error.
The provider parses `.short-item` first and falls back to `a.uas-card[href]`
(`uaserials.go:713-793`); the first version looked for `.th-title a` and got
nothing on a page that plainly contained the cards.

**The `<mark>` trap.** The live search page returns 18 `a.uas-card` elements. The
search term is highlighted inside card titles with `<mark>`, so exactly one of the
18 titles contains markup inside the title element. A naive
`([^<]*)`-style selector — anything that grabs text between `<` and `>` — silently
drops that one card and returns 17. Use goquery `.Text()` on the title element
instead of a regex over raw HTML, or the card count will be wrong with no
failure anywhere.

Search result cards carry no type marker, so the provider makes two extra
parallel requests to the filtered tab URLs (`?cat=serial`, `?cat=cartoonserial`)
and intersects the returned post IDs to recover `series` / `cartoon`
(`uaserials.go:594-653`). Best-effort by design: if those fail, cards stay
`movie` rather than the search failing.

### 3.4 uaserials — the AES scheme, and where the key really lives

Player tabs are not in the HTML as text. The page carries one element:

```html
<player-control data-default='Плеєр'
    data-tag1='{"ciphertext":"…","iv":"…","salt":"…"}'>
```

and the decrypted tab list arrives to the browser from a JS bundle.

Scheme, as implemented and verified:

| Parameter | Value |
|---|---|
| Cipher | AES-256-CBC |
| KDF | PBKDF2-HMAC-SHA512 |
| Iterations | **999** (CryptoJS `iterations: 0x3e7`) |
| Key length | **32 bytes** (CryptoJS `keySize: 0x8`, i.e. 8 × 4-byte words) |
| Ciphertext encoding | base64 |
| Salt encoding | hex |
| IV encoding | hex |

`uaserials.go:75-79` for the constants, `uaserials.go:314-382` for the decryptor.

**Where the key lives — and why the obvious regex never matches.** The passphrase
is not a literal in the bundle. On the 2026-10-05 snapshot it was assembled by an
obfuscated expression, e.g. `var dd=_0x4bfc33(0x13f)+_0x4bfc33(0x185)+'25'`.
The implementation keeps a plain `var dd="…"` regex as a fast path
(`uaserials.go:177`) and, when it does not match, keeps the cached key and returns
a decrypt error that names the real cause rather than silently returning an empty
season list. A port must do the same: **a key-discovery failure must be an
explicit error, not an empty result.**

**PKCS#7 is mandatory.** CryptoJS strips padding automatically; Go does not.
Without explicit unpadding you get JSON with trailing `\x0a` bytes, `json.Unmarshal`
fails, and the natural but wrong conclusion is "wrong key". The unpadding check is
deliberately strict — last byte equals tail length and every tail byte equals it —
precisely so that a lucky padding guess under a wrong key does not pass
(`uaserials.go:291-311`).

Two further traps on this site:

* The **"Trailer" tab has no word "trailer" in its URL.** Filtering on URL alone
  ships the user an advert instead of the series. Filter on tab name **and** URL
  (`uaserials.go:445-509`).
* Decrypted tabs also contain ordinary internal navigation links ("watch all
  episodes", "registration"). They are same-host HTML pages and must be dropped
  before stream resolution, or the player is handed a web page
  (`uaserials.go:452-484`).

### 3.5 tortuga — XOR cipher, stripped base64 padding, Latin-1, and the `file:` format

The `file:` value is **not** JSON. It is an encrypted string. Full cycle, verified
end-to-end 2026-10-05 on a four-season series:

```
page → encrypted file: value
     → base64 (padding stripped) → bytes
     → key = raw[0]; body[i] = raw[i+1] XOR ((key + i*7 + 13) mod 256)
     → 57 835 bytes of JSON on 155 episodes
     → PlayerJS tree → master playlist → 3 CDN qualities
```

Decode steps, in order, with the reason each is required
(`tortuga_cdn.go:76-171`):

1. Strip **all** trailing `=`. The real value carries none — the player strips
   padding before handing it over.
2. Re-pad to a multiple of 4, or `StdEncoding` refuses.
3. Decode base64 to **bytes, not runes.** Do not convert through `[]rune` and do
   not decode UTF-8. The algorithm operates on bytes: a six-letter Cyrillic word
   must pass through XOR as six bytes, not two runes.
4. `key = raw[0]`; XOR the rest with the linear shift above.

The step and offset constants (`7`, `13`) are **not free parameters.** They belong
to a third-party player. Changing them for tidiness breaks decryption on real
traffic; there is a comment at `tortuga_cdn.go:59-68` saying so, for the next
person who reads them as magic numbers.

JSON nesting depth is measured on the **decrypted** output, not the ciphertext.
Ciphertext is random bytes with no braces in it, so the check always passed and
therefore did nothing — a mutation test is what caught this
(`tortuga_cdn.go:163-169`).

**The `{dub}url(subtitle:…)` file format.** Observed live:

```
{1+1}https://<cdn-host>/hls/serials/<slug>/s01/<slug>.s01e01.<dub>_<id>/hls/index.m3u8(subtitle:)
```

The dubbing-studio name is in **braces before the URL**; subtitles are in
**parentheses after it**. This is the only case in the whole codebase where the
studio name comes from the file value rather than from the tree node's `title` —
which is why the generic tree walk saw no voiceovers for this player and the
client got an empty selector. Parse this format **before** URL validation: the
generic `isPlayableMediaURL` rejects any string containing braces, so routing this
player through the generic resolver yields zero streams
(`tortuga_format.go:10-72`, `tortuga_cdn.go:568-578`).

Subtitle values are a bare labelled list with **no `subtitle:` key**:
`[Українська]https://…,…,[English]https://…`. The generic subtitle parser looks
for the key or a `<track>` tag and misses it, so it is parsed separately
(`tortuga_cdn.go:266-324`).

**Trailers share the encrypted tree.** A trailer node sits at episode position 3
inside a season. A positional walk numbers it "episode 3", and the real episode 3
is then discarded as a duplicate — season 1 ends up with 3 episodes instead of 4
and the user physically cannot select the third one. Nodes declare numbers in
their title, so **removing** trailer nodes is safe (it does not renumber the
rest) while filtering by position is not (`tortuga_cdn.go:326-357`).

**Never put season/episode in the URL.** This CDN answers such a request with a
single episode instead of the tree, and the client loses the ability to switch
dubbing or episode. Season/episode filtering happens in memory only
(`tortuga_cdn.go:423-459`). Two consequences that look like bugs but are not:

* With no season/episode requested, "give me everything" must be narrowed to the
  first episode. Otherwise opening a details page reports "155 streams loaded" and
  155 identical rows.
* Quality expansion is deliberately done in `GetStreams`, not in the details path:
  details returns dozens of episodes and one master fetch each would turn opening
  a details page into 155 HTTP requests.

### 3.6 ashdi — HTTP 200 with a 404 master

This CDN serves a playlist URL that returns **200** but has no media behind it.
Measured 2026-10-05:

```
…/hls/480/index.m3u8   -> 200
…/hls/1080/index.m3u8  -> 200   the playlist exists
…/hls/480/segment1.ts  -> 200, 309 636 bytes
…/hls/1080/segment1.ts -> 404   no segments behind it
```

So the tempting shortcut — rewrite `/hls/index.m3u8` into
`/hls/1080/index.m3u8` — produces a phantom. The user picks "1080p" and playback
stalls on the first segment, which is worse than an honest "Auto".

**The rule: quality comes only from what the master playlist itself declared.
Never construct a quality path.** `ParseMasterPlaylist` returns variants sorted
best-first; if parsing fails, the source stays `Auto`. A source that could not be
expanded is left as-is rather than being given a guessed quality
(`hls_variants.go:12-37`, `hls_variants.go:143-177`).

Related: a trailer iframe on one site resolved to a URL with **no** "trailer"
word in it, only `?tr=1`, and its m3u8 pointed at a `/trailers/` path that was
otherwise indistinguishable from a real episode. So URL substring filters need an
explicit guard for that, not just a "trailer" keyword
(`resolve.go:266-274`, `resolve.go:486-498`).

### 3.7 DLE sites — `data-lebclicks` and the `data-soon-hash` flow

All four DLE-based providers share this, and it is the reason a static parse of a
DLE item page cannot be trusted to find the player.

**`data-lebclicks`.** DLE themes wrap ad/player containers in an element carrying
`data-lebclicks="yes"`. It marks a **click-counting ad wrapper**, not the player.
Observed on a real item page: the player-tab container
(`<div class="players-section">` with `UA #1` / `#torrent` tabs) carries the
attribute, while the actual media is fetched later by JavaScript. Treating this
element as "the player" and stopping there yields an empty result with no error.
It is a signal to **keep going**, not to stop.

**`data-soon-hash`.** DLE marks not-yet-published items with this attribute
instead of a player. Its presence means the item has no media yet and no amount of
parsing will produce one. Handle it explicitly — an empty season list here is a
correct answer, and returning it as a parse error causes pointless retry storms
against the site.

Both attributes are on this repository's forbidden-terms list
(`scripts/ci/forbidden.txt:46-48`), which is why they are named in prose here
rather than shown in markup.

---

## 4. Porting order

Goal for the community port: **reproduce one working series fastest**, then widen.
Each step below is independently useful and independently verifiable.

**1. `provider/testdata/` + `playerjs_scan.go` + `playlist_tree.go`**
Start here. Take the fixtures first — they are the ground truth and they are the
first thing lost if you start writing code. `playlist_tree.go` is pure logic with
no network and heavy test coverage (`playlist_tree_test.go`, 702 lines). Getting
tree traversal and season/episode numbering right before any HTTP exists means
every later failure is unambiguously a network or selector problem.

**2. `playerjs_scan.go` + `tortuga_format.go` + `tortuga_cdn.go`**
The CDN extractor is the narrowest end-to-end path and it is fully deterministic
offline: decrypt the fixture, parse the tree, prune trailers. Its cipher is 20
lines and its correctness is pinned by a real encrypted sample plus its plaintext
counterpart. This is where the "is my whole pipeline sane" answer comes from.

**3. `client.go` + `resolve.go`**
The HTTP client and the shared player resolver. Once these exist, every DLE
provider is reduced to catalogue parsing plus one call. Budget real time here:
the strategy ordering, iframe ranking and trailer guards in §3.6 all encode
failures that are invisible without live traffic.

**4. `uaserials.go`**
Most self-contained provider and the only one with a decryptor. The catalogue and
search selectors (§3.3) plus the AES path (§3.4) are independent of the shared
resolver. Its fixtures — catalogue, search, description page, and the tag test
vector — cover the whole surface offline.

**5. `studios.go` + `studios_data.go` + `dub_weight.go`**
Small, mechanical, and required before anything is presentable: without the studio
table and the ordering rules, streams come back labelled with CDN names instead of
dubbing studios. Port the table **verbatim** — client-side IDs depend on it.

**6. `hls_variants.go`**
Quality expansion, once there are streams to expand. Read §3.6 first; the
implementation looks trivial and the reason it is not naive is entirely in the
comment block.

**7. `eneyida.go`, `uakino.go`, `lavakino.go`**
Last, and each is small. Their real content is the live-selector knowledge in
§3.1, §3.2 and the section-slug tables — not their code, which is nearly
identical to each other. uakino is the weakest candidate to port first: its
playlist is AJAX-loaded (§3.1), so it is the site most likely to look broken
during early bring-up even when the port is correct.

**8. `search/` and `transport/http/`**
Independent of the providers and independent of each other. Port them when
several providers work, because clustering and relevance are only meaningful with
more than one source.

**9. `bandera.go` and friends**
The aggregator is a different shape entirely (JSON API, not HTML scraping) and
depends on `golang.org/x/sync/singleflight`. Lowest priority despite being the
largest single file.

---

## 5. Porting prerequisites

* **The `domain` types must be supplied by the community repository.** The
  exported code imports `internal/domain` for `MediaItem`, `MediaDetails`,
  `Season`, `Episode`, `Voiceover`, `StreamSource`, `SubtitleSource`,
  `ContentStreamsResponse`, `ProviderInfo`, `ProviderCatalog`,
  `ProviderCatalogEntry`, `ProviderHealth` and the `Provider` interface. None of
  them were under `plugins/`; all were in the app's `internal/domain`, which
  stays here. Define them before porting anything, and keep the JSON tags — the
  `Provider` interface method set is a contract, not an implementation detail.
* **There was no `go.mod` under `plugins/`.** The tree compiled as part of the app
  module. The community repository needs its own module and its own dependency set:
  `github.com/PuerkitoBio/goquery`, `github.com/bogdanfinn/tls-client`,
  `github.com/bogdanfinn/fhttp`, `golang.org/x/net/html/charset`,
  `golang.org/x/sync/singleflight`, `golang.org/x/text/unicode/norm`.
* **`crypto/pbkdf2`** is used for the AES key derivation
  (`uaserials.go:374-382`). It is a stdlib package in recent Go; on an older
  toolchain use `golang.org/x/crypto/pbkdf2` with the identical signature.

## 6. What this repository no longer contains

As of this commit:

* `plugins/` does not exist. 76 files, 24 884 lines.
* Nothing under `backend/`, `frontend/`, `contracts/` or `scripts/` imports or
  references it. Verified by build and test before and after deletion.
* `scripts/ci/check_forbidden.sh` gates the shipped paths and no longer needs a
  `plugins/` exclusion.
* The fixtures in §2 exist only in this document's inventory and in the community
  repository. If that repository is lost, the captured responses are lost.