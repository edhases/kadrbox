> ## Статус документа
>
> - **Status:** fixed
> - **Verified against:** `3ab45ef`
> - **Актуальність:** основні рекомендації підтверджено як реалізовані:
>   `season`/`episode` тепер `INT NULL` (`migrations/000001_init.up.sql:44-45`),
>   `UNIQUE NULLS NOT DISTINCT` застосовано, `rating`/`rating_source` додано,
>   `schema_migrations` ведеться (`repository/postgres/db.go`),
>   `CountUserFavorites` існує (`favorites_repo.go:112`).
> - **Див. також:** `MIGRATION_REPORT.md` (Wave 2, агенти F та G).
>
> Історичні знахідки нижче **не переписані** — вони залишено як запис стану на момент аудиту.
# Технічний аудит міграції бази даних (PostgreSQL vs PocketBase & Drift)

**Дата аудиту:** 28 вересня 2026 року  
**Роль:** Аудитор 1 — Валідатор міграції бази даних  
**Об'єкти перевірки:**
- Локальна схема Drift: [`lib/data/database/app_database.dart`](../../frontend/lib/data/database/app_database.dart), [`lib/data/database/dao/history_dao.dart`](../../frontend/lib/data/database/dao/history_dao.dart), [`lib/data/database/dao/favorites_dao.dart`](../../frontend/lib/data/database/dao/favorites_dao.dart)
- Хмарні сервіси PocketBase: [`lib/data/services/pocketbase_service.dart`](../../frontend/lib/data/services/pocketbase_service.dart), [`lib/data/services/history_service.dart`](../../frontend/lib/data/services/history_service.dart), [`lib/data/services/favorites_service.dart`](../../frontend/lib/data/services/favorites_service.dart), [`lib/data/services/auth_service.dart`](../../frontend/lib/data/services/auth_service.dart)
- Серверна схема PostgreSQL та репозиторії Go: [`backend/internal/repository/postgres/migrations/000001_init.up.sql`](../../backend/internal/repository/postgres/migrations/000001_init.up.sql), [`user_repo.go`](../../backend/internal/repository/postgres/user_repo.go), [`history_repo.go`](../../backend/internal/repository/postgres/history_repo.go), [`favorites_repo.go`](../../backend/internal/repository/postgres/favorites_repo.go), [`cache_repo.go`](../../backend/internal/repository/postgres/cache_repo.go), [`db.go`](../../backend/internal/repository/postgres/db.go)
- Утиліта міграції: [`backend/cmd/migrate_pb/main.go`](../../backend/cmd/migrate_pb/main.go)
- Контейнеризація: [`backend/docker-compose.yml`](../../backend/docker-compose.yml), [`backend/Dockerfile`](../../backend/Dockerfile)

---

## 1. Виконавче резюме (Executive Summary)

Перехід із комбінації **PocketBase (хмара) + Drift (локально)** на власний сервер **Go + PostgreSQL 16 + Redis** є стратегічно правильним кроком для підвищення продуктивності, масштабованості та контролю над бізнес-логікою.

### Загальний статус готовності:
- **Базова схема БД (PostgreSQL):** Створена та підготовлена до автоматичного накату через `embed.FS` при старті сервера.
- **Відповідність збереження полів:** **88%** (основні поля збережено, але втрачені `rating` і `rating_source` в обраному та історії; розбіжність імен у профілі користувача).
- **Обробка фільмів (`season`/`episode = 0` vs `NULL`):** Проблема унікальності в SQL вирішена через `DEFAULT 0`, проте це створює **критичну колізію в клієнтському інтерфейсі Flutter** (відображення "S0 E0" для фільмів та збій пошуку в `HistoryDao`).
- **Скрипт міграції (`cmd/migrate_pb/main.go`):** **НЕ ГОТОВИЙ ДО ПРОДАКШЕНУ (Критичний ризик)**. Скрипт переносить лише користувачів із фіктивним паролем, повністю ігноруючи історію (`watch_history`) та закладки (`favorites`), а також генерує нові UUID без збереження зв'язків.

---

## 2. Порівняльний аналіз моделей даних (Data Schemas Comparison)

### 2.1. Таблиця користувачів (`users`)

| Параметр / Поле | PocketBase (`users`) | Drift (клієнт) | PostgreSQL (`000001_init.up.sql`) | Go Модель (`domain.User`) | Статус відповідності |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Ідентифікатор** | `id` (15-char string) | Відсутній (Auth) | `id` (UUIDv4) | `ID uuid.UUID` | ⚠️ Потребує мапінгу при міграції |
| **Email** | `email` (string, unique) | Відсутній | `email` (VARCHAR(255), unique) | `Email string` | ✅ Повна відповідність |
| **Пароль** | `passwordHash` (bcrypt) | Відсутній | `password_hash` (VARCHAR(255)) | `PasswordHash string` | ✅ Хешування bcrypt |
| **Ім'я** | `name` (string) | `displayName` | `username` (VARCHAR(100)) | `Username string` | ⚠️ Розбіжність ключів (`name` vs `username`) |
| **Аватар** | `avatar` (ім'я файлу) | URL генератор | `avatar_url` (TEXT) | `AvatarURL string` | ⚠️ Різні формати зберігання |
| **Біографія** | `bio` (string) | `bio` (string) | `bio` (TEXT) | `Bio string` | ✅ Повна відповідність |
| **Роль** | `role` / адмін-флаг | Відсутній | `role` (VARCHAR(20) DEFAULT 'user') | `Role string` | ✅ Додано контроль доступу |
| **OAuth2** | Підтримується (Google, Discord) | `authWithOAuth2` | Відсутній | Відсутній | ❌ Зовнішні провайдери не реалізовані |

> [!WARNING]
> **Невідповідність імен полів у профілі користувача:**
> Клієнт Flutter (`PocketBaseService.signUp` та `AuthService.profile`) оперує полем `name`. Натомість бекенд Go очікує і повертає `username`. При реєстрації з Flutter або парсингу профілю це призведе до порожнього імені, якщо клієнт не адаптовано під новий DTO.

---

### 2.2. Таблиця закладок / обраного (`favorites`)

| Поле | Drift (`Favorites`) | PocketBase (`favorites`) | PostgreSQL (`000001_init.up.sql`) | Go Репозиторій (`favorites_repo.go`) | Статус |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`id`** | `Int` autoIncrement | `string` (15) | `UUID` (PK) | `ID uuid.UUID` | ✅ Коректно |
| **`user_id`** | Відсутнє (локальна БД) | `relation` / string | `UUID` (FK -> users.id) | `UserID uuid.UUID` | ✅ Каскадне видалення |
| **`media_id`** | `Text` | `string` | `VARCHAR(255)` | `string` | ✅ Повна відповідність |
| **`provider_id`**| `Text` | `string` | `VARCHAR(100)` | `string` | ✅ Повна відповідність |
| **`title`** | `Text` | `string` | `VARCHAR(500)` | `string` | ✅ Повна відповідність |
| **`poster_url`** | `Text` nullable | `string` nullable | `TEXT` nullable | `string` (pointer) | ✅ Повна відповідність |
| **`year`** | `Int` nullable | `int` nullable | `INT` nullable | `int` (pointer) | ✅ Повна відповідність |
| **`media_type`** | `Text` | `string` | `VARCHAR(50)` | `string` | ✅ Повна відповідність |
| **`rating`** | `Real` nullable | `num` nullable | **ВІДСУТНЄ** | **ВІДСУТНЄ** | ❌ **Втрата даних рейтингу** |
| **`rating_source`**| `Text` nullable | `string` nullable | **ВІДСУТНЄ** | **ВІДСУТНЄ** | ❌ **Втрата джерела рейтингу** |
| **`added_at`** | `DateTime` | ISO8601 string | `TIMESTAMPTZ DEFAULT NOW()` | `time.Time` | ✅ Повна відповідність |
| **Обмеження** | `{mediaId, providerId}` | `user_id + media_id + provider_id` | `UNIQUE(user_id, media_id, provider_id)` | `ON CONFLICT DO NOTHING` | ✅ Ідеальна відповідність |

> [!NOTE]
> **Аналіз полів рейтингу в `favorites`:**
> У Drift таблиці [`Favorites`](../../frontend/lib/data/database/app_database.dart#L65-L81) наявні колонки `rating` та `ratingSource`. У PostgreSQL таблиці `favorites` ці стовпці відсутні. Якщо користувач додає фільм в обране в офлайні з рейтингом (наприклад, IMDB 8.5), після синхронізації з бекендом рейтинг не збережеться на сервері.

---

### 2.3. Таблиця історії переглядів (`watch_history`)

| Поле | Drift (`WatchHistory`) | PocketBase (`watch_history`) | PostgreSQL (`000001_init.up.sql`) | Go Репозиторій (`history_repo.go`) | Статус |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`id`** | `Int` autoIncrement | `string` (15) | `UUID` (PK) | `ID uuid.UUID` | ✅ Коректно |
| **`user_id`** | Відсутнє (локально) | `relation` / string | `UUID` (FK -> users.id) | `UserID uuid.UUID` | ✅ Каскадне видалення |
| **`media_id`** | `Text` | `string` | `VARCHAR(255)` | `string` | ✅ Повна відповідність |
| **`provider_id`**| `Text` | `string` | `VARCHAR(100)` | `string` | ✅ Повна відповідність |
| **`title`** | `Text` | `string` | `VARCHAR(500)` | `string` | ✅ Повна відповідність |
| **`poster_url`** | `Text` nullable | `string` nullable | `TEXT` nullable | `string` (pointer) | ✅ Повна відповідність |
| **`year`** | `Int` nullable | `int` nullable | `INT` nullable | `int` (pointer) | ✅ Повна відповідність |
| **`media_type`** | `Text` | `string` | `VARCHAR(50)` | `string` | ✅ Повна відповідність |
| **`position_ms`**| `Int` (default 0) | `int` | `BIGINT NOT NULL DEFAULT 0` | `int64` | ✅ Чудовий вибір для мілісекунд |
| **`duration_ms`**| `Int` (default 0) | `int` | `BIGINT NOT NULL DEFAULT 0` | `int64` | ✅ Чудовий вибір для мілісекунд |
| **`season`** | `Int` nullable (null для фільмів) | `int` nullable | `INT NOT NULL DEFAULT 0` | `int` (0 для фільмів) | ⚠️ **Колізія UI та DAO** (деталі нижче) |
| **`episode`** | `Int` nullable (null для фільмів) | `int` nullable | `INT NOT NULL DEFAULT 0` | `int` (0 для фільмів) | ⚠️ **Колізія UI та DAO** (деталі нижче) |
| **`episode_title`**| `Text` nullable | `string` nullable | `VARCHAR(500)` | `string` (pointer) | ✅ Повна відповідність |
| **`last_stream_url`**| `Text` nullable | `string` nullable | `TEXT` nullable | `string` (pointer) | ✅ Повна відповідність |
| **`voiceover`** | `Text` nullable | `string` nullable | `VARCHAR(255)` | `string` (pointer) | ✅ Повна відповідність |
| **`watched_at`** | `DateTime` | ISO8601 string | `TIMESTAMPTZ DEFAULT NOW()` | `time.Time` | ✅ Повна відповідність |
| **`rating`** / **`rating_source`** | `Real` / `Text` | `num` / `string` (опціонально) | **ВІДСУТНІ** | **ВІДСУТНІ** | ⚠️ Не перенесені в схему |
| **Обмеження** | `{mediaId, providerId, season, episode}` | Складений фільтр | `UNIQUE(user_id, media_id, provider_id, season, episode)` | `ON CONFLICT DO UPDATE` | ✅ Індексна підтримка upsert |

---

### 2.4. Серверний кеш контенту (`content_cache`)

У схемі PostgreSQL реалізовано таблицю серверного кешу:
```sql
CREATE TABLE IF NOT EXISTS content_cache (
    cache_key VARCHAR(255) PRIMARY KEY,
    provider_id VARCHAR(100) NOT NULL,
    content_type VARCHAR(50) NOT NULL,
    data JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_content_cache_expires ON content_cache(expires_at);
```
- **Оцінка рішення:** **Відмінно**. Використання типу `JSONB` дає гнучкість для збереження структури деталей фільмів, сезонів, серій та стрім-посилань будь-якого провайдера.
- **Продуктивність:** Відмова від важких повнотекстових GIN індексів на користь простого `PRIMARY KEY (cache_key)` та B-Tree індексу на `expires_at` забезпечує блискавичне читання за ключем та швидке видалення застарілих даних воркером `DeleteExpired`.

---

### 2.5. Кімнати Watch Party (`watch_party_rooms`)

```sql
CREATE TABLE IF NOT EXISTS watch_party_rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_code VARCHAR(10) NOT NULL,
    host_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_active_room_code ON watch_party_rooms(room_code) WHERE is_active = TRUE;
```
- **Оцінка рішення:** **Відмінно**. Використання часткового індексу (partial index) `WHERE is_active = TRUE` дозволяє повторно використовувати короткі 6-значні коди кімнат після завершення сесій, уникаючи колізій із закритими кімнатами.

---

### 2.6. Локальні таблиці Drift, що залишаються на клієнті

Наступні таблиці з [`app_database.dart`](../../frontend/lib/data/database/app_database.dart) обґрунтовано не потребують прямого дублювання в PostgreSQL:
1. **`Downloads`:** Зберігає стан завантаження офлайн-відеофайлів (шляхи на диску пристрою, розміри, прогрес). Це специфіка локального сховища пристрою.
2. **`SearchHistoryTable`:** Кеш пошукових запитів для миттєвих підказок в UI (offline-first).
3. **`StoredMediaItems`:** Локальний SQLite-кеш раніше переглянутих карток для роботи без мережі.
4. **`AppSettings` & `EnabledProviders`:** Налаштування плеєра та активні парсери на конкретному девайсі.

---

## 3. Глибокий аналіз проблеми фільмів: `season/episode = 0` vs `NULL`

### 3.1. Суть проблеми в SQL та Drift
У реляційних базах даних стандарт SQL передбачає, що значення `NULL` не дорівнює іншому `NULL` (`NULL != NULL`).
- У Drift та PocketBase для фільмів поля `season` та `episode` мали значення `NULL` (або були відсутні в JSON).
- Якби в PostgreSQL таблиці `watch_history` поля були `season INT NULL` та `episode INT NULL`, обмеження `UNIQUE (user_id, media_id, provider_id, season, episode)` **НЕ СПРАЦЬОВУВАЛО б для фільмів**! Кожен новий запуск того самого фільму створював би новий рядок у таблиці, засмічуючи базу та ламаючи `ON CONFLICT DO UPDATE`.

### 3.2. Рішення розробників бекенду Go
У міграції `000001_init.up.sql` встановлено:
```sql
season INT NOT NULL DEFAULT 0,
episode INT NOT NULL DEFAULT 0,
CONSTRAINT uq_user_history UNIQUE (user_id, media_id, provider_id, season, episode)
```
Це гарантує, що для фільмів ключ унікальності завжди чітко визначений: `(user_id, media_id, provider_id, 0, 0)`.

### 3.3. Виявлена колізія в клієнтському коді Flutter
Незважаючи на коректність у SQL, передача `0` замість `null` у клієнт Flutter спричиняє декілька серйозних багів:

1. **Баг у віджеті "Продовжити перегляд" ([`continue_watching_section.dart:185`](../../frontend/lib/presentation/widgets/home/continue_watching_section.dart#L185)):**
   ```dart
   if (item.season != null && item.episode != null) ...[
     Text('S${item.season} E${item.episode}'),
   ]
   ```
   Якщо фільм синхронізовано з бекенду зі значеннями `season: 0, episode: 0`, перевірка `!= null` поверне `true`. На картці фільму з'явиться некоректний бейдж: **`S0 E0`**.
2. **Баг на сторінці деталей ([`details_page.dart:1103`](../../frontend/lib/presentation/pages/details/details_page.dart#L1103)):**
   Параметр `'initialSeason': item.season` передасть значення `0`. Оскільки в серіалах сезони починаються з 1, логіка вибору епізоду може впасти з помилкою або вибрати пустий список.
3. **Баг у DAO пошуку ([`history_dao.dart:32`](../../frontend/lib/data/database/dao/history_dao.dart#L32)):**
   ```dart
   (season != null ? t.season.equals(season) : t.season.isNull()) &
   (episode != null ? t.episode.equals(episode) : t.episode.isNull())
   ```
   Коли Flutter додаток запитує прогрес для фільму, він передає `season: null`. Запит Drift шукає рядок де `season IS NULL`. Але з бекенду прийшло значення `0`. Результат: **додаток вважає, що фільм не переглядався, і починає відтворення з 00:00!**

### 3.4. Архітектурні шляхи вирішення колізії

#### Варіант 1 (Рекомендований): Використання `UNIQUE NULLS NOT DISTINCT` у PostgreSQL 16
У `docker-compose.yml` використовується образ `postgres:16-alpine`. Починаючи з PostgreSQL 15, з'явилася нативна підтримка стандарту SQL:2023:
```sql
ALTER TABLE watch_history ALTER COLUMN season DROP NOT NULL;
ALTER TABLE watch_history ALTER COLUMN season DROP DEFAULT;
ALTER TABLE watch_history ALTER COLUMN episode DROP NOT NULL;
ALTER TABLE watch_history ALTER COLUMN episode DROP DEFAULT;

ALTER TABLE watch_history DROP CONSTRAINT uq_user_history;
ALTER TABLE watch_history ADD CONSTRAINT uq_user_history 
    UNIQUE NULLS NOT DISTINCT (user_id, media_id, provider_id, season, episode);
```
При такому синтаксисі PostgreSQL трактує всі `NULL` як ідентичні значення для індексу унікальності! Це повністю зберігає чистий контракт `NULL` як на бекенді, так і у Flutter, усуваючи необхідність будь-яких хаків із нулями.

#### Варіант 2: Мапінг на рівні Go DTO (Серіалізатор)
Якщо залишати `DEFAULT 0` в БД, то в структурі `domain.WatchHistory`:
- Додати поля `Season *int json:"season"` і при читанні з БД конвертувати `0 -> nil`, а при записі `nil -> 0`.

---

## 4. Валідація унікальних обмежень та логіки Upsert

### 4.1. Обмеження та конфлікти у `favorites_repo.go`
```sql
INSERT INTO favorites (user_id, media_id, provider_id, title, poster_url, year, media_type, added_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT (user_id, media_id, provider_id) DO NOTHING
```
- **Статус:** **Працює ідеально**. Ідемпотентна операція, запобігає дублікатам одного й того самого тайтлу в обраному користувача.

### 4.2. Обмеження та конфлікти у `history_repo.go`
```sql
INSERT INTO watch_history (
    user_id, media_id, provider_id, title, poster_url, year, media_type,
    season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, watched_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW()
)
ON CONFLICT (user_id, media_id, provider_id, season, episode)
DO UPDATE SET
    title = EXCLUDED.title,
    poster_url = COALESCE(EXCLUDED.poster_url, watch_history.poster_url),
    episode_title = COALESCE(EXCLUDED.episode_title, watch_history.episode_title),
    position_ms = EXCLUDED.position_ms,
    duration_ms = EXCLUDED.duration_ms,
    last_stream_url = COALESCE(EXCLUDED.last_stream_url, watch_history.last_stream_url),
    voiceover = COALESCE(EXCLUDED.voiceover, watch_history.voiceover),
    watched_at = NOW()
```

#### Зауваження щодо стратегії "Newer Timestamp Wins":
- Зверніть увагу: `watched_at` завжди оновлюється на `NOW()`.
- Якщо користувач дивився серію офлайн у літаку вчора, а підключився до мережі сьогодні після того, як на іншому девайсі було відкрито ту саму серію раніше, клієнт надсилає `watched_at` зі своїм локальним часом перегляду.
- Сервер перезаписує `watched_at` поточним часом сервера `NOW()`, ігноруючи клієнтський таймстемп. Для строгої реалізації стратегії "новіший виграє" слід враховувати:
  `watched_at = CASE WHEN EXCLUDED.watched_at > watch_history.watched_at THEN EXCLUDED.watched_at ELSE watch_history.watched_at END`.

---

## 5. Аудит скрипта імпорту даних `cmd/migrate_pb/main.go`

Було проведено ретельний рядок-за-рядком аналіз коду [`backend/cmd/migrate_pb/main.go`](../../backend/cmd/migrate_pb/main.go).

```go
// Фрагмент із migrate_pb/main.go:
users, err := fetchPBRecords[PBUserRecord](pbURL, "users", pbAdminToken)
...
for _, u := range users {
    dummyHash, _ := auth.HashPassword("DefaultMigratedPassword2026!")
    _, err := userRepo.CreateUser(ctx, u.Email, dummyHash, u.Username)
    ...
}
```

### Виявлені критичні дефекти:

1. ❌ **Повна відсутність міграції `favorites` та `watch_history`:**
   Скрипт звертається виключно до колекції `users`. Колекції `favorites` та `watch_history` не зчитуються взагалі. Усі накопичені закладки та історія переглядів користувачів при такій міграції будуть втрачені!
2. ❌ **Скидання користувацьких паролів:**
   Усім користувачам встановлюється однаковий пароль `"DefaultMigratedPassword2026!"`. Користувачі не зможуть увійти зі своїми звичними паролями без процедури скидання пароля (яка на даний момент у Go-сервері ще не реалізована).
3. ❌ **Розрив зовнішніх ключів (Foreign Key Mismatch):**
   PocketBase генерує 15-символьні рядкові ідентифікатори (наприклад, `r8e9f2a4b1c7d3e`). Postgres використовує `UUIDv4`. Метод `CreateUser` генерує новий випадковий `UUID`. Старий `PB ID` не зберігається. Як результат: навіть якщо додати читання `favorites` з PocketBase, їх неможливо прив'язати до нового `user_id` без побудови мапи відповідності `map[string]uuid.UUID` (`pb_user_id -> new_pg_uuid`).
4. ❌ **Втрата аватара та біографії:**
   Хоча структура `PBUserRecord` парсить поля `Avatar` та `Bio`, метод `userRepo.CreateUser(ctx, u.Email, dummyHash, u.Username)` взагалі не приймає цих параметрів, і вони відкидаються.
5. ❌ **Ігнорування пагінації PocketBase:**
   Запит використовує `perPage=500`. Якщо в базі PocketBase більше 500 користувачів, скрипт завантажить лише першу сторінку, проігнорувавши решту через відсутність циклу за `page=1, 2, ...`.

---

## 6. Інфраструктура та розгортання (Docker & Portainer)

### 6.1. Автоматичний накат міграцій
У файлі [`backend/internal/repository/postgres/db.go`](../../backend/internal/repository/postgres/db.go#L13-L65):
```go
//go:embed migrations/*.sql
var MigrationsFS embed.FS
```
При виклику `postgres.InitDB(ctx, dsn)` функція `runMigrations` автоматично зчитує всі файли з суфіксом `.up.sql` та застосовує їх до бази.
- **Оцінка:** **Відмінно**. Додатку не потрібні сторонні утиліти (`migrate-cli`) під час старту в Docker/Portainer. При першому піднятті стека всі таблиці та індекси створюються автономно.

### 6.2. Docker Compose та ресурсні ліміти
У [`backend/docker-compose.yml`](../../backend/docker-compose.yml):
- Образ `postgres:16-alpine` з томом `pgdata:/var/lib/postgresql/data`.
- Healthcheck налаштовано коректно через `pg_isready`.
- Ліміти пам'яті:
  - `app`: Limit 128MB, Reservation 32MB.
  - `postgres`: Limit 256MB, Reservation 64MB.
  - `redis`: Limit 64MB.
- **Оцінка:** Конфігурація готова для деплою на VDS з 1GB+ RAM через Portainer.

---

## 7. Матриця виявлених дефектів та план виправлення

| № | Дефект / Невідповідність | Рівень критичності | Компонент | Необхідна дія |
| :- | :--- | :--- | :--- | :--- |
| **1** | Скрипт `migrate_pb/main.go` не імпортує `favorites` та `watch_history` | 🔴 **CRITICAL** | `backend/cmd/migrate_pb` | Реалізувати повноцінний експорт усіх колекцій з мапінгом `pb_id -> pg_uuid` за email. |
| **2** | Колізія `season/episode = 0` у клієнтському UI ("S0 E0" для фільмів) та збій пошуку в `HistoryDao` | 🔴 **CRITICAL** | `migrations` / `history_repo` / Flutter UI | Використати `UNIQUE NULLS NOT DISTINCT` у Postgres 16 або мапити `0 <-> null` у DTO. |
| **3** | Втрата полів `rating` та `rating_source` в таблицях `favorites` та `watch_history` | 🟡 **MEDIUM** | `migrations/000001_init.up.sql` | Додати `rating REAL`, `rating_source VARCHAR(100)` до міграцій PostgreSQL. |
| **4** | Розбіжність полів профілю користувача (`name` vs `username`, `avatar` vs `avatar_url`) | 🟡 **MEDIUM** | `domain/user.go`, `auth_handler.go` | Додати псевдоніми або уніфікувати JSON-контракт між Flutter та Go. |
| **5** | Відсутність підтримки OAuth2 (Google/Discord) у новому Go-сервері | 🟡 **MEDIUM** | `backend/internal/auth` | Розробити модуль OAuth2 обміну кодами на JWT або зафіксувати це обмеження в документації. |
| **6** | Скидання паролів у `migrate_pb` на фіктивне значення без механізму скидання | 🟡 **MEDIUM** | `backend/cmd/migrate_pb` | Додати генерацію одноразових токенів скидання або розсилку повідомлень користувачам. |

---

## 8. Висновок аудитора

Архітектура перенесення бази даних на PostgreSQL спроєктована надійно з точки зору типів даних, індексації та автоматизації накату міграцій через Go `embed.FS`.

Проте, перед введенням системи в промислову експлуатацію необхідно:
1. **Усунути колізію `season/episode = 0`** (рекомендовано через нативний для PG 16 `UNIQUE NULLS NOT DISTINCT`).
2. **Переписати утиліту `migrate_pb`**, щоб вона гарантувала цілісний перенос історії та закладок користувачів зі збереженням зв'язків.
3. **Узгодити DTO полів користувача та рейтингів** між Flutter-провайдерами та Go HTTP-хендлерами.
