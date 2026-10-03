# Oxide Film

![Oxide Film Banner](https://raw.githubusercontent.com/edhases/oxide_film/main/assets/branding/banner.png)

**[English](#english) | [Українська](#українська)**

---

<a name="english"></a>
## 🇬🇧 English

**Oxide Film** is a media aggregation application for movies, series, cartoons and anime, built
with Flutter for desktop and Android. Parsing and streaming of upstream providers happens in a
companion **Go** backend, so no provider traffic ever originates from the client.

### ✨ Features

- **Multi-Provider Support**: Aggregates content from **uakino**, **lavakino**, **eneyida** and
  **bandera** (`server/internal/provider/`, `lib/data/services/provider_catalog_service.dart`).
- **Unified Library**: Watch history, continue-watching and favourites, synced across devices.
- **Server-side proxies**: Content is fetched by the Go backend; the client receives metadata and
  stream URLs only.
- **Cross-Platform**: Android, Linux and Windows (Material 3, light/dark, dynamic theming).
- **Advanced Player**: `media_kit`-based player with quality selection, subtitles and gestures.
- **Offline Support**: Local SQLite database with cached metadata and history.
- **Account Sync**: JWT auth (email/password plus Google, Discord and Telegram OAuth2) against the
  Go backend.
- **Watch Party**: WebSocket rooms hosted by the backend, synchronised playback and chat.

### 🚀 Getting Started

#### Prerequisites

| Requirement | Version | Source of truth |
|---|---|---|
| Flutter SDK | **stable** channel (CI uses `channel: stable`) | `.github/workflows/flutter-ci.yml` |
| Dart SDK | **`^3.10.7`** | `pubspec.yaml` (`environment.sdk`) |
| Go (only to build/run the backend locally) | **1.23** | `server/go.mod` |
| Docker + Compose (only for the backend) | any recent | `server/docker-compose.yml` |

There is **no** `web/` target — the app is not a web build. See
[Web is not supported](#-web-is-not-supported) below.

#### Installation

```bash
git clone https://github.com/edhases/oxide_film.git
cd oxide_film
flutter pub get
flutter run            # Android / Linux / Windows
```

### 🛠 Tech Stack

Everything below is derived from `pubspec.yaml`, `server/go.mod` and the directory tree.

#### Client — Flutter (`lib/`, ~Flutter stable / Dart 3.10.7)

| Concern | Actual technology |
|---|---|
| Framework | Flutter |
| Navigation | `go_router` ^17.1.0 |
| State management | Hand-rolled `ChangeNotifier` (13 classes in `lib/`) + `get_it` ^9.2.0 / `injectable` ^2.5.0 for DI. **`provider` and `flutter_bloc` are NOT used** — `flutter_bloc` is declared but has zero references. |
| Local database | **`drift` ^2.31.0** + `drift_flutter`, SQLite via `sqlite3_flutter_libs` |
| HTTP client | `dio` ^5.8.0 + `dio_smart_retry`, `dio_cookie_manager`, `connectivity_plus` |
| HTML parsing (client-side) | `beautiful_soup_dart` ^0.3.0 |
| Video player | `media_kit` ^1.1.11 + `media_kit_video`; platform libs for Android, Windows, Linux |
| Cloudflare bypass | `flutter_inappwebview` ^6.1.5 (headless cookie extraction) |
| Metadata | `tmdb_api` ^2.1.7 |
| i18n | Hand-rolled `lib/core/l10n/app_strings.dart` — **English** and **Ukrainian** |
| Desktop windowing | `window_manager` ^0.5.1 (custom title bar) |
| Misc | `peerdart`, `fuzzywuzzy`, `shared_preferences`, `wakelock_plus`, `package_info_plus`, `permission_handler`, `image_picker`, `file_picker`, `share_plus`, `open_filex`, `crypto`, `cached_network_image`, `flutter_cache_manager`, `shimmer`, `skeletonizer`, `semaphore`, `collection`, `intl`, `url_launcher`, `path_provider` |

#### Backend — Go (`server/`, ~33k LOC including tests)

| Concern | Actual technology |
|---|---|
| Language | **Go 1.23** (`server/go.mod`) |
| HTTP router | **`go-chi/chi/v5`** ^5.1.0 |
| CORS | `go-chi/cors` with an `AllowOriginFunc` **allow-list** (`server/internal/transport/http/allowed_origins.go`) — not a wildcard |
| Database | **PostgreSQL 16** via `jackc/pgx/v5` ^5.6.0, SQL migrations in `server/internal/repository/postgres/migrations/` |
| Cache / sessions / pub-sub | **Redis 7** via `redis/go-redis/v9` ^9.6.1 |
| Auth | `golang-jwt/jwt/v5` ^5.2.1 (HS256) + `golang.org/x/crypto` (Argon2id password hashing) |
| OAuth2 | Google, Discord, Telegram — server-side `state` nonce + PKCE S256 |
| WebSocket (watch party) | `gorilla/websocket` ^1.5.3 + a Redis-backed hub (`server/internal/transport/ws/`) |
| Scraping / parsing | `PuerkitoBio/goquery` ^1.9.2 |
| TLS fingerprinting | `bogdanfinn/tls-client` ^1.7.5 + `fhttp` ^0.5.28 |
| Containerisation | `server/Dockerfile`, `server/docker-compose.yml` |

#### CI/CD

Three GitHub Actions workflows:

- `.github/workflows/flutter-ci.yml` — Dart formatting, analysis and tests
- `.github/workflows/server-test.yml` — `go build` / `go vet` / `go test`
- `.github/workflows/docker-publish.yml` — builds and publishes `ghcr.io/edhases/oxide-server`

### 🖥 Running the Backend

The server is required for provider search, streams, auth and sync. It **fails closed** on
misconfiguration — it will refuse to start rather than run with a weak or default signing key.

#### With Docker Compose (recommended)

```bash
cd server
export JWT_SECRET="$(openssl rand -base64 48)"
export POSTGRES_USER="oxide"
export POSTGRES_PASSWORD="$(openssl rand -base64 32)"
export REDIS_PASSWORD="$(openssl rand -base64 32)"
docker compose up -d
```

The API is then served on `http://127.0.0.1:8089`. `docker-compose.yml` publishes the container's
`8080` as host port `8089`. PostgreSQL data persists in the `pgdata` volume.

> Compose uses `${VAR:?message}` syntax, so it aborts immediately with a clear message if any
> required variable is missing — this is intentional.

#### Required environment variables

The server **refuses to start** without these (`server/config/config.go` → `Validate()`):

| Variable | Rule | Notes |
|---|---|---|
| `JWT_SECRET` | **≥ 32 bytes**, not a known placeholder | HMAC key for access tokens. If it is weak or guessable, anyone can forge a token. Generate with `openssl rand -base64 48`. Placeholders such as `super-secret-jwt-key-2026`, `secret`, `changeme`, `jwt-secret`, `test` are explicitly rejected. |
| `POSTGRES_USER` | non-empty | Mapped to `DB_USER`. |
| `POSTGRES_PASSWORD` | **≥ 16 bytes** | Mapped to `DB_PASSWORD`. Generate with `openssl rand -base64 32`. |
| `REDIS_PASSWORD` | non-empty | Redis holds refresh tokens and watch-party state; an open Redis leaks every session. |
| `SERVER_PORT` | non-empty | Defaults to `8080`. |
| `DB_HOST` / `DB_NAME` / `APP_URL` | non-empty | Default to `postgres` / `oxide_film` / `https://film.oxideteam.pp.ua`. |

#### Recommended environment variables

Not enforced at startup, but strongly recommended:

| Variable | Purpose |
|---|---|
| `UPSTREAM_HOST_ALLOWLIST` | Comma-separated allow-list of upstream hosts. When empty, **any public host is reachable** and the server logs an explicit `[SSRF]` warning (`server/cmd/api/main.go`). When set, DNS resolution is skipped entirely. |
| `TRUSTED_PROXY_CIDRS` | Comma-separated CIDRs of reverse proxies whose `X-Forwarded-For` is trusted, e.g. `172.18.0.0/16` in Compose. Without it, rate limiting keys off the direct `RemoteAddr`, so all clients can appear as one IP behind a proxy (`server/internal/transport/http/middleware/ratelimit.go`). |
| `LOG_FORMAT` | `text` (default) or `json` for log aggregation. |
| `LOG_LEVEL` | `info` (default), `debug`, `warn`, `error`. |

#### Optional variables

`DB_SSLMODE`, `REDIS_ADDR`, `BASE_PROXY_URL`, `DISABLED_PROVIDERS` (e.g. `uakino,lavakino`),
`GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / `GOOGLE_REDIRECT_URI`,
`DISCORD_CLIENT_ID` / `DISCORD_CLIENT_SECRET` / `DISCORD_REDIRECT_URI`,
`TELEGRAM_BOT_TOKEN` / `TELEGRAM_BOT_USERNAME`, `RESEND_API`, `SMTP_FROM`.

#### Portainer deployment

The published image is `ghcr.io/edhases/oxide-server:latest`. The compose file is written for a
Portainer stack: declare `JWT_SECRET`, `POSTGRES_USER`, `POSTGRES_PASSWORD` and `REDIS_PASSWORD`
as **stack environment variables** (Portainer's Secrets/Environment panel), not baked into the
YAML. The `${VAR:?...}` guards will stop the stack from deploying if any are absent.

> **Operator action:** if a stack was ever deployed with a weak `JWT_SECRET`, rotate it. The old
> key must be treated as compromised and the stack restarted.

#### Health endpoints

- `GET /health`, `GET /healthz` — liveness
- `GET /readyz` — readiness (pings PostgreSQL and Redis)

### 🚫 Web is not supported

There is **no `web/` directory** in this repository and no web target is configured. The app is
not a web build and cannot be made into one cheaply: 15 files under `lib/` import `dart:io`
(`Platform`, `File`, `Directory`), and the plugin set (`window_manager`, `media_kit_libs_*`,
`permission_handler`, `open_filex`) has no web implementation.

Any documentation claiming web support is stale.

### 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request. Please read
[`docs/REMEDIATION_PLAN.md`](docs/REMEDIATION_PLAN.md) for the current work-in-flight status, and
[`docs/audit/`](docs/audit/) for the security and correctness audit history.

### 📄 License

See [`LICENSE`](LICENSE).

---

<a name="українська"></a>
## 🇺🇦 Українська

**Oxide Film** — медіазастосунок для фільмів, серіалів, мультфільмів та аніме, створений на
Flutter для Android та десктопу. Розбір і стрімінг джерел виконує супутній **Go**-бекенд, тому
трафік до провайдерів ніколи не йде з клієнта.

### ✨ Можливості

- **Мульти-провайдерність**: **uakino**, **lavakino**, **eneyida** та **bandera**.
- **Єдина бібліотека**: історія переглядів, «продовжити перегляд» та обране, синхронізовані між
  пристроями.
- **Проксі на сервері**: контент завантажує Go-бекенд; клієнт отримує лише метадані та посилання
  на потік.
- **Кросплатформність**: Android, Linux та Windows (Material 3, світла/темна тема).
- **Просунутий плеєр**: на базі `media_kit` — вибір якості, субтитри, керування жестами.
- **Офлайн режим**: локальна база SQLite з кешем метаданих та історії.
- **Синхронізація**: JWT-авторизація (email/пароль та OAuth2 Google, Discord, Telegram).
- **Watch Party**: WebSocket-кімнати на бекенді — синхронізоване відтворення та чат.

### 🚀 Початок роботи

#### Вимоги

| Вимога | Версія |
|---|---|
| Flutter SDK | **`stable`** (CI використовує `channel: stable`) |
| Dart SDK | **`^3.10.7`** (`pubspec.yaml`) |
| Go (лише для локальної збірки бекенду) | **1.23** (`server/go.mod`) |
| Docker + Compose (лише для бекенду) | будь-яка сучасна версія |

**Веб-платформа не підтримується**: каталогу `web/` у репозиторії немає, а 15 файлів у `lib/`
імпортують `dart:io`.

```bash
git clone https://github.com/edhases/oxide_film.git
cd oxide_film
flutter pub get
flutter run
```

### 🛠 Технології

#### Клієнт — Flutter

- **Навігація**: `go_router`
- **Стан**: власні `ChangeNotifier` + `get_it` / `injectable`. Бібліотеки `provider` та
  `flutter_bloc` **не використовуються** (`flutter_bloc` оголошена, але має нуль посилань).
- **Локальна база**: **`drift` / SQLite**
- **HTTP**: `dio` + `dio_smart_retry`, `dio_cookie_manager`
- **Плеєр**: `media_kit`
- **Обхід Cloudflare**: `flutter_inappwebview`
- **Метадані**: `tmdb_api`
- **Локалізація**: `lib/core/l10n/app_strings.dart` — **англійська** та **українська**

#### Бекенд — Go (`server/`, ~33 тис. рядків)

- **Маршрутизатор**: `go-chi/chi/v5`
- **CORS**: `go-chi/cors` з `AllowOriginFunc` — **allow-list**, не `*`
- **База**: **PostgreSQL 16** (`pgx/v5`) + SQL-міграції
- **Кеш і сесії**: **Redis 7**
- **Авторизація**: `golang-jwt/jwt/v5` + Argon2id (`golang.org/x/crypto`)
- **OAuth2**: Google, Discord, Telegram — серверний `state` + PKCE S256
- **Watch Party**: `gorilla/websocket` + Redis-hub
- **Парсинг**: `goquery`; TLS-відпечатки: `tls-client`

#### CI/CD

`flutter-ci.yml`, `server-test.yml`, `docker-publish.yml` (публікація
`ghcr.io/edhases/oxide-server`).

### 🖥 Запуск бекенду

Сервер **не стартує** при некоректній конфігурації.

```bash
cd server
export JWT_SECRET="$(openssl rand -base64 48)"
export POSTGRES_USER="oxide"
export POSTGRES_PASSWORD="$(openssl rand -base64 32)"
export REDIS_PASSWORD="$(openssl rand -base64 32)"
docker compose up -d
```

API доступний на `http://127.0.0.1:8089`.

**Обов'язкові змінні:** `JWT_SECRET` (≥ 32 байти, не заглушка), `POSTGRES_USER`,
`POSTGRES_PASSWORD` (≥ 16 байтів), `REDIS_PASSWORD`.

**Бажані змінні:** `UPSTREAM_HOST_ALLOWLIST` (без нього доступний будь-який публічний хост і
сервер пише попередження `[SSRF]`), `TRUSTED_PROXY_CIDRS` (без нього rate-limit бачить усіх
клієнтів за одним IP), `LOG_FORMAT`, `LOG_LEVEL`.

**Portainer:** образ `ghcr.io/edhases/oxide-server:latest`. Обов'язкові змінні задаються як
**stack environment variables**; compose використовує `${VAR:?...}` і не даде розгорнути стек
без них.

**Health:** `/health`, `/healthz` (liveness), `/readyz` (PostgreSQL + Redis).

### 🤝 Участь у розробці

Ми вітаємо будь-яку допомогу! Будь ласка, створюйте Pull Request. Актуальний стан роботи —
у [`docs/REMEDIATION_PLAN.md`](docs/REMEDIATION_PLAN.md).
