# 📋 Звіт з архітектурної міграції Oxide Film (Kadrbox v2)

**Дата створення:** 2026-10-05  
**Гілка:** `dev` (архівна копія v1 зафіксована в `release-v1`)  
**Автор реструктуризації:** Antigravity AI  

---

## 1. Мета та стратегія міграції

Проєкт переходить від монолітного сервера зі вбудованими веб-скраперами до **трьохкомпонентної модульної архітектури**:

```
                       ┌──────────────────────────────────────────────┐
                       │          1. BACKEND (Хмарний сервер)         │
                       │  • Чистий Go REST API + WebSocket Hub        │
                       │  • PostgreSQL (Users, History, Favorites)    │
                       │  • Redis (Сесії, Pub/Sub, Токени)            │
                       │  • 100% легальний, нуль піратських парсерів  │
                       └──────────────────────┬───────────────────────┘
                                              │ Auth, Sync, Watch Together
                                              ▼
                       ┌──────────────────────────────────────────────┐
                       │          2. FRONTEND (Клієнтський UI)        │
                       │  • Flutter (Windows, Android, Linux, iOS)    │
                       │  • Медіаплеєр (media_kit)                    │
                       │  • Спільний перегляд (Watch Party)           │
                       │  • Плагінний завантажувач (Plugin Engine)    │
                       │  • Придатний для публікації в Microsoft Store│
                       └──────────────────────┬───────────────────────┘
                                              │ Опціональне підключення джерел
                                              ▼
                       ┌──────────────────────────────────────────────┐
                       │          3. PLUGINS (Джерела контенту)       │
                       │  • sidecar-scrapers (локальний Go-демон)     │
                       │  • lampac-bridge (клієнт до Lampac API / JS) │
                       │  • Запити до сайтів йдуть з IP користувача   │
                       └──────────────────────────────────────────────┘
```

---

## 2. Поточний стан репозиторію

### 2.1. Що вже зроблено:
1. Поточний робочий стан зафіксовано й запушено в гілку **`release-v1`**.
2. У гілці **`dev`** файли фізично рознесено через `git mv` (зі збереженням повної історії комітів):
   - **`frontend/`**: весь Flutter-проєкт (`lib/`, `test/`, `windows/`, `android/`, `assets/`, `pubspec.yaml` тощо).
   - **`backend/`**: серверний Go-код (`cmd/`, `internal/auth/`, `internal/repository/`, `internal/transport/`, `docker-compose.yml`, `go.mod` тощо).
   - **`plugins/sidecar-scrapers/`**: відокремлені парсери сайтів (`internal/provider/` та `internal/search/`).

### 2.2. Що наразі зламано або потребує виправлення:
1. **`backend/`**:
   - `backend/cmd/api/main.go` намагається імпортувати видалені `internal/provider` та `internal/search`.
   - `backend/internal/transport/http/router.go` все ще оголошує маршрути `/api/v1/content/*` (`contentH.Search`, `contentH.GetDetails`, `contentH.GetStreams`, `contentH.Popular`, `contentH.Category`, `contentH.Providers`).
   - `backend/internal/transport/http/content_handler.go` та пов'язані файли валідації/кешування контенту все ще лежать у `backend`, хоча мають бути перенесені в `plugins/sidecar-scrapers` або видалені з бекенду.
   - Тести `backend/internal/transport/http/...` падають через відсутність контент-хендлера та прив'язок провайдерів.
2. **`plugins/sidecar-scrapers/`**:
   - Папка містить тільки `internal/provider` та `internal/search`.
   - **Немає власного `go.mod`**.
   - **Немає власної точки входу (`cmd/sidecar/main.go`)**, яка запускатиме локальний веб-сервер на `127.0.0.1:8089` і обслуговуватиме `/api/v1/content/*`.
3. **`frontend/`**:
   - `frontend/lib/data/providers/server_backed_provider.dart` наразі жорстко прив'язаний до ендпоінтів сервера `/api/v1/content`. Потрібно переписати його на адаптер, який підключається до локального sidecar або Lampac.
   - Скрипти запуску та CI очікували `pubspec.yaml` у корені проєкту, тепер корінь Flutter — це `frontend/`.
4. **Сміття в корені проєкту**:
   - Потрібно видалити або заігнорити: `.flutter-plugins-dependencies`, `app_run.log`, `update.json`, `oxide_film.iml`, `build/`, `coverage/`.

---

## 3. Завдання для субагентів за компонентами

### 🔵 Блок 1: Очищення та стабілізація `backend/`
**Мета:** Перетворити бекенд на ультралегкий, повністю легальний сервіс авторизації, синхронізації та WebSocket-кімнат.

1. **Видалити залежності від парсерів у `backend/cmd/api/main.go`**:
   - Прибрати імпорти `github.com/edhases/oxide-server/internal/provider`.
   - Прибрати реєстрацію `provider.NewRegistry()`, `clientOpts`, створення скраперів (`UakinoProvider`, `EneyidaProvider`, тощо).
   - Прибрати фоновий кеш-воркер `cacheRepo.DeleteExpired` для контенту (якщо кешування контенту більше не ведеться на бекенді).
   - Прибрати виклик `transporthttp.NewContentHandler(...)`.
2. **Очистити HTTP роутер `backend/internal/transport/http/router.go`**:
   - Видалити секцію маршрутів `r.Route("/content", ...)`.
   - Залишити тільки:
     - `/healthz`, `/readyz`
     - `/api/v1/auth/*` (реєстрація, логін, refresh, oauth)
     - `/api/v1/watch-party/tickets`
     - `/api/v1/sync/*` (історія, улюблене, прогрес перегляду)
     - `/ws` (Watch Party WebSocket Hub)
3. **Видалити застарілі файли з `backend/internal/transport/http/`**:
   - `content_handler.go`, `content_cache.go`, `content_validation.go`, `catalogue_handler.go`, `search_pipeline.go`.
   - Їх відповідні тести (`content_handler_coverage_test.go`, `content_validation_coverage_test.go`, тощо) перенести у `plugins/sidecar-scrapers` або очистити.
4. **Перевірка**:
   - Запустити `cd backend && go test ./...` — всі тести повинні проходити на 100%.
   - Запустити `cd backend && go build ./cmd/api` — бінарник має збиратися без помилок.

---

### 🟢 Блок 2: Формування `plugins/sidecar-scrapers/`
**Мета:** Зібрати існуючі Go-парсери в автономний мікросервіс (sidecar daemon), який може запускатися на пристрої клієнта.

1. **Ініціалізація Go-модуля**:
   - `cd plugins/sidecar-scrapers && go mod init github.com/edhases/oxide-sidecar`
   - Налаштувати залежності (`goquery`, `tls-client` тощо).
2. **Створення точки входу `plugins/sidecar-scrapers/cmd/sidecar/main.go`**:
   - Мінімальний HTTP-сервер, який слухає `127.0.0.1:8089` (порт за замовчуванням або з прапорця `--port`).
   - Ініціалізує `provider.NewRegistry()` з провайдерами UAKino, Eneyida, Lavakino, UASerials, Bandera.
   - Експортує маршрути:
     - `GET /status` (healthcheck для перевірки з Flutter)
     - `GET /api/v1/content/providers`
     - `GET /api/v1/content/search?q=...`
     - `GET /api/v1/content/popular`
     - `GET /api/v1/content/category`
     - `GET /api/v1/content/details?provider=...&url=...`
     - `GET /api/v1/content/streams?provider=...&url=...&season=...&episode=...`
3. **Перевірка**:
   - `go test ./...` всередині `plugins/sidecar-scrapers` проходить без помилок.
   - Бінарник збирається у `sidecar.exe`.

---

### 🟡 Блок 3: Адаптація `frontend/`
**Мета:** Очистити Flutter-застосунок для проходження в Microsoft Store та реалізувати підтримку плагінів.

1. **Рефакторинг джерел даних (`frontend/lib/data/`)**:
   - Створити концепцію `ContentSource` / `PluginEngine`:
     - **Default mode:** "Local Player & Watch Together". Застосунок показує бібліотеку локальних відео, можливість відкрити мережевий потік/файл, та підключитися до кімнати друзів.
     - **Sidecar mode:** Якщо в налаштуваннях увімкнено Sidecar (`http://127.0.0.1:8089`), або додаток сам запускає `sidecar.exe`, з'являється розділ пошуку та каталогу.
     - **Lampac mode:** Поле для вводу URL Lampac-сервера (або вибору публічного дзеркала).
2. **Оновлення конфігурації та шляхів**:
   - Перевірити коректність роботи `frontend/pubspec.yaml`, `flutter test`.
   - Налаштувати збірку Windows MSIX у `frontend/windows/`.
3. **Microsoft Store Compliance**:
   - Переконатися, що в коді `frontend/lib` немає захардкодних назв нелегальних ресурсів у текстах інтерфейсу за замовчуванням. Всі назви провайдерів мають приходити динамічно від підключеного плагіна (`GET /api/v1/content/providers`).

---

### ⚪ Блок 4: Очищення кореня репозиторію
1. Оновити `.gitignore` у корені для підтримки `frontend/`, `backend/`, `plugins/`.
2. Видалити застарілі тимчасові файли:
   - `app_run.log`
   - `.flutter-plugins-dependencies`
   - `oxide_film.iml`
   - `update.json` (або перенести до `frontend/`)
   - `build/` та `coverage/` у корені (вони мають створюватись у відповідних папках).

---

## 4. Контракт API між Frontend та Backend

### Хмарний сервер (`backend`)
- **Базовий URL:** `https://api.oxide.example` (або локально `:8080`)
- **Маршрути:**
  - `POST /api/v1/auth/register` — реєстрація
  - `POST /api/v1/auth/login` — вхід
  - `POST /api/v1/auth/refresh` — оновлення токенів
  - `POST /api/v1/auth/google`, `telegram`, `discord` — OAuth
  - `GET /api/v1/sync/history`, `POST /api/v1/sync/history` — історія та таймкоди
  - `GET /api/v1/sync/favorites`, `POST /api/v1/sync/favorites/toggle` — улюблене
  - `POST /api/v1/watch-party/tickets` — квитки для WebSockets
  - `GET /ws` — WebSocket кімнати спільного перегляду

### Локальний плагін (`plugins/sidecar-scrapers` або `Lampac`)
- **Базовий URL:** `http://127.0.0.1:8089` (або власний Lampac URL)
- **Маршрути:**
  - `GET /api/v1/content/providers`
  - `GET /api/v1/content/search?q=...`
  - `GET /api/v1/content/popular?provider=...&type=...&page=...`
  - `GET /api/v1/content/category?provider=...&category=...&type=...&page=...`
  - `GET /api/v1/content/details?provider=...&url=...`
  - `GET /api/v1/content/streams?provider=...&url=...&season=...&episode=...`
