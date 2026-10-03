# oxide_film — Remediation Plan (виконавчий документ)

Дата: 2026-10-01 · База: `dev` @ `0bd7d00` · Аудит: 6 субагентів, 112 унікальних знахідок

---

## Статус виконання

| Хвиля | Область | Статус |
|---|---|---|
| Wave 0 | JWT/секрети, Portainer env | **DONE** |
| Wave 1 | Platform/lifecycle, observability, WebSocket hub, auth & OAuth | **DONE** |
| Wave 2 | SSRF + rate limit, persistence + міграції, API-контракт + кеш, Dart sync | **DONE** |
| Wave 3 | Provider performance, Dart runtime, тести/CI, гігієна, build-конфіг | **DONE** |
| Інтеграція | Зєднання між хвилями, wire-контракт, аналізатор, форматування | **DONE** |

**Верифікація:** `go build`/`go vet` exit 0, `go test ./...` 13/13 пакетів, `gofmt` чистий,
`flutter analyze` — **No issues found!**, `flutter test` — **374 passed** (1 skipped),
`dart format --set-exit-if-changed lib test` exit 0.

### Поза цим планом

- **Операторська дія:** якщо стек уже працював — змінити `JWT_SECRET` у Portainer і
  перезапустити. Старий ключ вважається скомпрометованим.
- **BREAKING:** клієнт мусить перейти на тікети Watch Party
  (`POST /api/v1/watch-party/tickets`) і парсити конверт `{"data": [...]}`. Розгортання
  конверта вже реалізовано в `ApiClient._asJsonMapList`, але нативний WebSocket клієнта ще
  надсилає identity у query.
- **Відкладено:** `strict-casts`/`strict-raw-types` (58 помилок у несвоєних файлах), повний
  поділ `auth_handler.go`, DNS-rebinding через dial-time перевірку (закривається
  `UPSTREAM_HOST_ALLOWLIST`).
- **Не перевірено локально:** `go test -race` (немає C-компілятора). CI запускає з `-race`.

---

## Статус виконання

Оновлено під час Wave 4 (гігієна репозиторію, агент **L**). Перевірено на `3ab45ef`.

### DONE

| Хвиля | Пункт | Доказ |
|---|---|---|
| 0 | `JWT_SECRET` без дефолту + fail-closed `Validate()` | `config/config.go:96-132`, 11 регресійних кейсів у `config_validate_test.go` |
| 0 | Portainer-обов'язкові env + Redis `--requirepass` | `docker-compose.yml:14-19,72` |
| 1A | HTTP-таймаути + graceful shutdown | `cmd/api/main.go:256-259` |
| 1B | `slog`, `trace_id`, `/healthz` + `/readyz`, body limit | `internal/logging/`, `middleware/{logging,requestctx,bodylimit,health}.go` |
| 1C | Watch party: handshake-токен, `CheckOrigin`, `dropLocked`, Redis поза lock | `transport/ws/{ticket,origin,protocol,options}.go`, `hub.go:421-436` |
| 1D | OAuth `state` + PKCE S256, `GETDEL` refresh, `RevokeAllForUser` | `transport/http/oauth_state.go`, `redis.go:126,180-184,300` |
| 2F | `NULLS NOT DISTINCT`, `season/episode NULL`, `schema_migrations` | `migrations/000001_init.up.sql:44-45`, `postgres/db.go` |
| 2G | Єдиний JSON error/list-envelope, `CountUserFavorites`, кеш+singleflight | `transport/http/api_envelope.go`, `favorites_repo.go:112` |
| 2H | In-flight guard `_saveProgress`, 401 refresh-and-replay | `history_service.dart`, `api_client.dart` |
| 3I | Provider: singleflight, пакетні regexp, обмеження редиректів | `provider/client.go:42,71,82` |
| 4L | Гігієна репозиторію | `.gitignore` переписано; 28 файлів / ~9.7k рядків сміття видалено з `test/`; `Audite.md` видалено; README переписано; 8 звітів аудиту анотовано |

### REMAINING / OPEN

| Пункт | Статус | Власник |
|---|---|---|
| **BREAKING: list-envelope** — сервер віддає `{"data":[...],"meta":{...}}`, клієнт має розбирати це синхронно | ⚠️ відкрито | G + H |
| `POST /auth/logout` та `POST /watch-party/tickets` — **зареєстровані** в `router.go:123,128`, але клієнт має їх викликати | ⚠️ частково | D + клієнт |
| `flutter_bloc` (нуль посилань) та `injectable` — мертві залежності | відкрито | J (`pubspec.yaml`) |
| `update_service.dart` вказує на гілку `master`, репозиторій на `main` | відкрито | J |
| Версії розсинхронізовані: `pubspec.yaml` `2026.9.30+2` / `msix_version: 2026.9.30.1` / `assets/version.json` `2026.9.30`+2 / `update.json` `20260208` / `AppConfig.appVersion '1.0.0'` | відкрито | J |
| `docs/01`-`05` історичні, містять PocketBase/Web/6-провайдерів | позначено як історичні | L (зроблено) |
| `dart format` — 12+ файлів під `test/` не форматовані (pre-existing) | відкрито | K |
| Операторська дія: ротація `JWT_SECRET` у Portainer, якщо стек уже працював | поза кодом | DevOps |

### Скасовані або зміщені пункти

- ~~`hub.go`: перенести `delete(clients, client)` під `h.mu.Lock()`~~ — **виконано** у Wave 1C,
  але реалізовано ширше: єдина точка виведення `dropLocked` (`hub.go:428`).
- ~~CORS: прибрати конфлікт `AllowCredentials`~~ — **виконано** точково через
  `AllowOriginFunc` з allow-list (`router.go:42-50`), не послабленням `AllowCredentials`.
- ~~Eneyida `.short-title` → `.short_title`~~ — **виконано** (`eneyida.go:131,138`).
  У Lavakino роздільники справді `.short-title` (`lavakino.go:144`) — це інша розмітка.

---

## Правило виконання

Кожен субагент має **ексклюзивне право власності** на перелік файлів. Заборонено редагувати
будь-який файл позначеним списком. Якщо потрібно змінити чужий файл — описати вимогу в
звіті, а не редагувати.

Маршрутизація (`server/internal/transport/http/router.go`) належить інтегратору (Tech Lead)
і не редагується субагентами після Wave 1.

---

## Wave 0 — виконано

| # | Що | Файли | Статус |
|---|---|---|---|
| 0.1 | `JWT_SECRET` без дефолту + fail-closed `Validate()` | `server/config/config.go`, `server/cmd/api/main.go` | DONE |
| 0.2 | Portainer-обов'язкові env: `JWT_SECRET`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `REDIS_PASSWORD` | `server/docker-compose.yml` | DONE |
| 0.3 | Redis `--requirepass` + healthcheck з паролем | `server/docker-compose.yml` | DONE |
| 0.4 | Регресійні тести валідації (11 кейсів) | `server/config/config_validate_test.go` | DONE |

**Операторська дія (обов'язково, поза кодом):** якщо стек уже працював — змінити `JWT_SECRET`
в Portainer і перезапустити. Старий ключ вважається скомпрометованим.

---

## Wave 1 — P0/P1 безпека (паралельно, 4 агента)

### A. Platform: bootstrap, lifecycle, HTTP-сервер
**Власність:** `server/cmd/api/main.go`, `server/Dockerfile`, `.github/workflows/docker-publish.yml`
- `ReadHeaderTimeout: 5s`, `MaxHeaderBytes: 1<<20`
- Graceful shutdown: idempotent-зупинка, порядок WS→HTTP, `os.Exit(1)` при невдалому drain
- Join горутин перед закриттям пулів
- `HEALTHCHECK` у Dockerfile; `mem_limit`/`cpus` (Compose, не Swarm)
- `EXTRACT main()` в `run(ctx, cfg) error` для тестованості

### B. Observability + HTTP middleware
**Власність:** `server/internal/transport/http/router.go`,
`server/internal/transport/http/middleware/{logging,requestctx,bodylimit,health}.go` (нові),
`server/internal/logging/` (новий)
- `slog` JSON, рівні, без PII
- `trace_id`: chi RequestID → context → `X-Request-Id` у відповіді
- `/healthz` (live) + `/readyz` (PG ping + Redis ping)
- Глобальний `http.MaxBytesReader` (1 MiB) + `DisallowUnknownFields` там, де безпечно
- Не додавати доменні маршрути

### C. Watch Party WebSocket
**Власність:** `server/internal/transport/ws/*.go`
- Аутентифікація handshake (підписний короткочасний токен), `user_id` з claims
- `CheckOrigin` → allow-list
- Host-роль: тільки хост керує `play/pause/seek/speed`
- Ліквідація витоку `hub.rooms`; ліміт кімнат і клієнтів
- `readPump` з `select` на `stopChan`; idempotent `GracefulStop`
- Redis publish поза lock, з timeout-контекстом
- Власний `EventPublisher` інтерфейс у пакеті `ws`

### D. Auth & OAuth security
**Власність:** `server/internal/transport/http/auth_handler.go`,
`server/internal/transport/http/stores.go`,
`server/internal/auth/*.go`, `server/internal/repository/redis/redis.go`
- OAuth: серверний `state` nonce + PKCE S256, точний allow-list redirect
- `renderOAuthStatusHTML` — екранування (XSS)
- Зміна пароля/видалення → revoke всіх сесій; додати `RevokeAllForUser`
- `Refresh` → атомарний `GETDEL` + виявлення повторного використання
- `RequireVerifiedEmail` → fail-closed
- Анти-енумерація email; Argon2 off-request-path
- Не ігнорувати помилки `MarkEmailVerified`

---

## Wave 2 — P1/P2 цілісність даних (4 агента)

### E. SSRF + rate limiting
**Власність:** `server/internal/transport/http/security.go`,
`server/internal/provider/client.go`, `server/internal/transport/http/middleware/ratelimit.go`
- Resolve-then-check IP; блокувати redirect; `io.LimitReader` на upstream
- Валідація JSON-гілки Bandera (зараз валідує 0 полів)
- Trusted-proxy для `extractIP`; окремий ліміт для `/auth/*`

### F. Persistence: репозиторії + міграції
**Власність:** `server/internal/repository/postgres/{user_repo.go,favorites_repo.go,history_repo.go,db.go}`,
`server/internal/repository/postgres/migrations/*.sql`
- `schema_migrations` + advisory lock + транзакції на міграцію
- Транзакції: `MarkEmailVerified`, `ResetPassword`, `CreateVerificationToken`
- `EXCLUDED.watched_at` у merge історії
- `LOWER(email)` унікальний індекс
- CHECK-обмеження; індекси на FK; прибрати дублікати індексів
- `isDuplicateKeyError` → `pgconn.PgError.ConstraintName`

### G. Sync/Content контракт + кеш
**Власність:** `server/internal/transport/http/{sync_handler.go,content_handler.go}`,
`server/internal/repository/postgres/cache_repo.go`,
`server/internal/provider/registry.go`
- Єдиний JSON error-envelope (замість `text/plain`)
- Ідемпотентний toggle (або `DELETE...RETURNING` + `INSERT`)
- `limit/offset` для favourites; `meta.total`
- Кеш: помилки ≠ miss; singleflight; negative-cache; TTL-koreкція
- Кеш `GetDetails`; `Popular`/`Category`
- `Search` винести з concrete-type assertion; `recover()` в усіх fan-out
- `registry.go`: `singleflight` не має успадковувати чужий `ctx`

### H. Dart: sync-цілісність та auth-відновлення
**Власність:** `lib/data/services/{history_service.dart,oxide_server_service.dart}`,
`lib/data/database/dao/history_dao.dart`, `lib/core/network/api_client.dart`
- In-flight guard для `_saveProgress`; атомарний DAO-upsert
- `history_dao`: прибрати read-then-write, обробити дублікати без `getSingleOrNull` throw
- 401-interceptor з refresh-and-replay (замість `signOut()` на першому 401)
- Retry-політика Dio: не ретраїти неідемпотентні POST без ключа
- `_isUnauthorized` — тільки `ServerException.statusCode == 401`

---

## Wave 3 — P2/P3 борг (4 агента)

### I. Provider performance
**Власність:** `server/internal/provider/{bandera_client.go,uakino.go,lavakino.go,eneyida.go,resolve.go}`
- Прибрати data race + не тримати mutex через HTTP (singleflight)
- `regexp` на рівні пакету (було в циклі на кожну картку)
- Паралельний fan-out iframe з per-candidate budget
- Прибрати надлишкові копії map; `io.LimitReader`

### J. Dart: якість застосунку
**Власність:** `analysis_options.yaml`, `pubspec.yaml`,
`lib/core/utils/logger.dart`, `lib/core/l10n/app_strings.dart`,
`lib/presentation/app.dart`,
`lib/presentation/pages/home/home_page.dart`,
`lib/presentation/pages/search/search_page.dart`,
`lib/data/services/update_service.dart`, `update.json`
- Увімкнути `use_build_context_synchronously`, `unused_field`, прибрати глобальні `ignore`
- Прибрати `fsync` на рядок логу; асинхронний batched flush
- Leak `Connectivity` subscription у HomePage
- `mounted` guard після `await` перед `setState`
- `update_service`: гілка `master` → `main`; обов'язковий хеш
- Додати `build_runner` + `drift_dev`

### K. Тести й CI
**Власність:** `server/go.mod`, `server/go.sum`,
`server/internal/repository/postgres/pg_container_test.go` (новий),
`.github/workflows/{server-test.yml,flutter-ci.yml}`, `test/**`
- `testcontainers-go` для Postgres/Redis; прибрати env-gating
- Тести на `UpsertWatchHistory` ідемпотентність + пагінація + OAuth-функції з 0 %
- `dart format` зробити зеленим (12 файлів)
- CI: перевірка що `.g.dart` не застарілий

### L. Гігієна репозиторію
**Власність:** `README.md`, `Audite.md`, `.gitignore`, `docs/**`, `test/*.html`,
root-скрипти (`analyze_*.py`, `find_*.py`, `test_uaflix_*.py`, `debug_provider_html*.dart`,
`test_search.dart`, `test_avatar.html`, `test_eneyida.html`, `app_run.log`)
- `git rm --cached` для проігнорованих, але закомічених файлів
- Видалити 2.85 MB сміття з `test/`
- README: реальний стек замість PocketBase/Hive/Web
- Прибрати мертві залежності (`flutter_bloc`, `injectable`)

---

## Контракти між агентами (зафіксувати, щоб не ламали компіляцію)

1. **F → G**: `GetUserFavorites(ctx, userID uuid.UUID, limit, offset int) ([]domain.Favorite, error)`
   + окремий `CountUserFavorites(ctx, userID) (int, error)`.
2. **D → B**: `logging.New(level, format)` повертає `*slog.Logger`; A викликає його в `run()`.
3. **E → G**: сигнатура `ValidateSafeURL(raw string) error` **не змінюється** (G не чіпає security.go).
4. **G**: `cache_repo.go` належить G; F його **не** редагує.
5. **C**: не редагує `stores.go` (належить D); власний інтерфейс оголошує всередині пакету `ws`.
6. **Маршрути**: `POST /auth/logout` та `POST /watch-party/tickets` — зареєстровані
   інтегратором у Wave 1/2 (`router.go:123,128`). Клієнт ще має їх викликати.

---

## Критерії приймання (DoD)

- `go build ./...` exit 0
- `go vet ./...` exit 0
- `go test ./...` — усі тести зелені, `go test -race ./internal/transport/...` без data race
- `go test -race` не виявляє race у `provider/` та `ws/`
- `flutter analyze` — 0 issues
- `flutter test` — зелено
- `dart format --set-exit-if-changed lib test` — exit 0
- `docker compose config` валідний
