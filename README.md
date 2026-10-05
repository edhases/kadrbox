# Kadrbox

**[English](#english) | [Українська](#українська)**

---

<a name="english"></a>
## 🇬🇧 English

**Kadrbox** is a cross-platform media player for Android, Windows and Linux. It plays video,
keeps your watch progress in sync across devices, and lets you watch together with friends in
real time.

The app contains no content catalogue of its own. It talks to a **catalog server** whose URL
you supply in Settings. Kadrbox ships no parsers and no hard-coded sources — change the URL,
change the catalogue.

### ✨ Features

- **Your catalog, your choice**: point the app at any catalog server URL. Nothing is bundled.
- **Unified Library**: Watch history, continue-watching and favourites, synced across devices.
- **Progress sync**: Sign in and your position, favourites and history follow you between
  devices.
- **Watch Party**: WebSocket rooms hosted by the backend — synchronised playback and chat.
- **Advanced Player**: `media_kit`-based player with quality selection, subtitles and gestures.
- **Cross-Platform**: Android, Linux and Windows (Material 3, light/dark/AMOLED).
- **Offline Support**: Local SQLite database with cached metadata and history.

### 🚀 Getting Started

#### Prerequisites

| Requirement | Version | Source of truth |
|---|---|---|
| Flutter SDK | **stable** channel (CI uses `channel: stable`) | `.github/workflows/flutter-ci.yml` |
| Dart SDK | **`^3.10.7`** | `frontend/pubspec.yaml` (`environment.sdk`) |
| Go (only to build/run the backend locally) | per `backend/go.mod` | `backend/go.mod` |
| Docker + Compose (only for the backend) | any recent | `backend/docker-compose.yml` |

There is **no** `web/` target — the app is not a web build. See
[Web is not supported](#-web-is-not-supported) below.

#### Installation

```bash
git clone https://github.com/edhases/kadrbox.git
cd kadrbox
cd frontend
flutter pub get
flutter run            # Android / Linux / Windows
```

#### Connecting a catalog

Open **Settings → Catalog** and enter the base URL of your catalog server, for example
`http://127.0.0.1:8089` for a server running on the same machine, or
`https://catalog.example` for one you host. Kadrbox validates the endpoint and shows the
available sources. Without a catalog the app still works as a local player: open a video file
from disk and start a watch party around it.

### 🛠 Tech Stack

Everything below is derived from `frontend/pubspec.yaml`, `backend/go.mod` and the directory
tree.

#### Client — Flutter (`frontend/lib/`)

| Concern | Actual technology |
|---|---|
| Framework | Flutter |
| Navigation | `go_router` |
| State management | Hand-rolled `ChangeNotifier` + `get_it` for DI |
| Local database | `drift` + `drift_flutter`, SQLite via `sqlite3_flutter_libs` |
| HTTP client | `dio` + `dio_smart_retry`, `dio_cookie_manager` |
| Video player | `media_kit` + `media_kit_video`; platform libs for Android, Windows, Linux |
| Metadata | `tmdb_api` |
| i18n | `lib/core/l10n/app_strings.dart` — **English** and **Ukrainian** |
| Desktop windowing | `window_manager` (custom title bar) |

#### Backend — Go (`backend/`)

The backend does authentication, sync and watch-party coordination. **It does not fetch
content and does not know what any source is.**

| Concern | Actual technology |
|---|---|
| Language | Go (per `backend/go.mod`) |
| HTTP router | `go-chi/chi/v5` |
| CORS | `go-chi/cors` with an `AllowOriginFunc` **allow-list** — not a wildcard |
| Database | **PostgreSQL 16** via `jackc/pgx/v5`, SQL migrations |
| Cache / sessions / pub-sub | **Redis 7** via `redis/go-redis/v9` |
| Auth | `golang-jwt/jwt/v5` (HS256) + Argon2id password hashing |
| OAuth2 | Google, Discord, Telegram — server-side `state` nonce + PKCE S256 |
| WebSocket (watch party) | `gorilla/websocket` + a Redis-backed hub |
| Containerisation | `backend/Dockerfile`, `backend/docker-compose.yml` |

#### CI/CD

GitHub Actions workflows run Dart formatting, analysis and tests; `go build` / `go vet` /
`go test`; and the forbidden-terms gate.

### 🖥 Running the Backend

The server is required for auth, sync and watch parties. It **fails closed** on
misconfiguration — it will refuse to start rather than run with a weak or default signing key.

#### With Docker Compose (recommended)

```bash
cd backend
export JWT_SECRET="$(openssl rand -base64 48)"
export POSTGRES_USER="kadrbox"
export POSTGRES_PASSWORD="$(openssl rand -base64 32)"
export REDIS_PASSWORD="$(openssl rand -base64 32)"
docker compose up -d
```

> Compose uses `${VAR:?message}` syntax, so it aborts immediately with a clear message if any
> required variable is missing — this is intentional.

#### Required environment variables

| Variable | Rule | Notes |
|---|---|---|
| `JWT_SECRET` | **≥ 32 bytes**, not a known placeholder | HMAC key for access tokens. Generate with `openssl rand -base64 48`. |
| `POSTGRES_USER` | non-empty | Mapped to `DB_USER`. |
| `POSTGRES_PASSWORD` | **≥ 16 bytes** | Mapped to `DB_PASSWORD`. |
| `REDIS_PASSWORD` | non-empty | Redis holds refresh tokens and watch-party state. |
| `SERVER_PORT` | non-empty | Container port. |

#### Recommended environment variables

| Variable | Purpose |
|---|---|
| `TRUSTED_PROXY_CIDRS` | Comma-separated CIDRs of reverse proxies whose `X-Forwarded-For` is trusted. Without it, rate limiting keys off the direct `RemoteAddr`. |
| `LOG_FORMAT` | `text` (default) or `json` for log aggregation. |
| `LOG_LEVEL` | `info` (default), `debug`, `warn`, `error`. |

#### Portainer deployment

Declare `JWT_SECRET`, `POSTGRES_USER`, `POSTGRES_PASSWORD` and `REDIS_PASSWORD` as **stack
environment variables** (Portainer's Secrets/Environment panel), not baked into the YAML. The
`${VAR:?...}` guards will stop the stack from deploying if any are absent.

> **Operator action:** if a stack was ever deployed with a weak `JWT_SECRET`, rotate it. The old
> key must be treated as compromised and the stack restarted.

#### Health endpoints

- `GET /health`, `GET /healthz` — liveness
- `GET /readyz` — readiness (pings PostgreSQL and Redis)

### 🚫 Web is not supported

There is **no `web/` directory** in this repository and no web target is configured. The app is
not a web build and cannot be made into one cheaply: files under `frontend/lib/` import
`dart:io`, and the plugin set (`window_manager`, `media_kit_libs_*`, `permission_handler`,
`open_filex`) has no web implementation.

Any documentation claiming web support is stale.

### 🤝 Contributing

Contributions are welcome! Please read [`docs/audit/`](docs/audit/) for the audit history and
[`docs/README.md`](docs/README.md) for the architecture overview. Note that this repository
must never name a content source — a catalog server is chosen by the user, not by us.

### 📄 License

MIT — see [`LICENSE`](LICENSE).

---

<a name="українська"></a>
## 🇺🇦 Українська

**Kadrbox** — кросплатформний медіаплеєр для Android, Windows та Linux. Він відтворює відео,
синхронізує прогрес перегляду між пристроями та дозволяє дивитися разом із друзями в реальному
часі.

У застосунку немає власного каталогу контенту. Він звертається до **каталогового сервера**, URL
якого ви вказуєте в налаштуваннях. Kadrbox не містить парсерів і жодних жорстко заданих
джерел — змінили URL, змінився каталог.

### ✨ Можливості

- **Ваш каталог, ваш вибір**: вкажіть URL каталогового сервера. Нічого не вбудовано.
- **Єдина бібліотека**: історія переглядів, «продовжити перегляд» та обране, синхронізовані між
  пристроями.
- **Синхронізація прогресу**: увійдіть — і ваша позиція, обране та історія переходять між
  пристроями.
- **Watch Party**: WebSocket-кімнати на бекенді — синхронізоване відтворення та чат.
- **Просунутий плеєр**: на базі `media_kit` — вибір якості, субтитри, керування жестами.
- **Кросплатформність**: Android, Linux та Windows (Material 3, світла/темна/AMOLED-тема).
- **Офлайн режим**: локальна база SQLite з кешем метаданих та історії.

### 🚀 Початок роботи

| Вимога | Версія |
|---|---|
| Flutter SDK | **`stable`** (CI використовує `channel: stable`) |
| Dart SDK | **`^3.10.7`** (`frontend/pubspec.yaml`) |
| Go (лише для локальної збірки бекенду) | згідно `backend/go.mod` |
| Docker + Compose (лише для бекенду) | будь-яка сучасна версія |

**Веб-платформа не підтримується**: каталогу `web/` у репозиторії немає, а файли в
`frontend/lib/` імпортують `dart:io`.

```bash
git clone https://github.com/edhases/kadrbox.git
cd kadrbox/frontend
flutter pub get
flutter run
```

### 🖥 Запуск бекенду

Сервер **не стартує** при некоректній конфігурації.

```bash
cd backend
export JWT_SECRET="$(openssl rand -base64 48)"
export POSTGRES_USER="kadrbox"
export POSTGRES_PASSWORD="$(openssl rand -base64 32)"
export REDIS_PASSWORD="$(openssl rand -base64 32)"
docker compose up -d
```

**Обов'язкові змінні:** `JWT_SECRET` (≥ 32 байти, не заглушка), `POSTGRES_USER`,
`POSTGRES_PASSWORD` (≥ 16 байтів), `REDIS_PASSWORD`.

**Бажані змінні:** `TRUSTED_PROXY_CIDRS` (без нього rate-limit бачить усіх клієнтів за одним
IP), `LOG_FORMAT`, `LOG_LEVEL`.

**Portainer:** обов'язкові змінні задаються як **stack environment variables**; compose
використовує `${VAR:?...}` і не даде розгорнути стек без них.

**Health:** `/health`, `/healthz` (liveness), `/readyz` (PostgreSQL + Redis).

### 🤝 Участь у розробці

Ми вітаємо будь-яку допомогу! Будь ласка, створюйте Pull Request. Актуальний стан роботи —
у [`docs/README.md`](docs/README.md) та [`docs/audit/`](docs/audit/). Зверніть увагу: цей
репозиторій не має права згадувати жодне джерело контенту — каталоговий сервер обирає
користувач, а не ми.

### 📄 Ліцензія

MIT — див. [`LICENSE`](LICENSE).