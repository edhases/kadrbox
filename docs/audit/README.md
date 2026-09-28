# Підсумковий зведений звіт аудиту перенесення проєкту Oxide Film на Go-сервер

Аудит проведено командою з **5 незалежних субагентів-аудиторів** шляхом аналізу реального коду, структур даних, мережевих відповідей та конфігурацій (без синтетичних моків).

---

## 🗂️ Звіти субагентів-аудиторів

| № | Напрямок аудиту | Файл звіту | Ключовий висновок |
| :--- | :--- | :--- | :--- |
| 1 | **База даних та міграції** | [01_DATABASE_MIGRATION_AUDIT.md](01_DATABASE_MIGRATION_AUDIT.md) | Схема готова, але `season/episode = 0` ламає UI клієнта ("S0 E0" для фільмів). Потрібно використати стандарт **PostgreSQL 16 `UNIQUE NULLS NOT DISTINCT`** і повернути полям `NULL`. Утиліту `cmd/migrate_pb` треба доповнити імпортом favorites та history. |
| 2 | **Провайдери та парсери** | [02_PARSERS_AND_PROVIDERS_AUDIT.md](02_PARSERS_AND_PROVIDERS_AUDIT.md) | Виявлено застарілі селектори Eneyida (`.short-title` замість `.short_title`), блокування редиректів UAKino (`uakino.me` -> `uakino.best`), а також необхідність резолвінгу прямого HLS замість повернення iframe/HTML сторінок. |
| 3 | **Watch Party & WebSocket** | [03_WATCH_PARTY_REALTIME_AUDIT.md](03_WATCH_PARTY_REALTIME_AUDIT.md) | Конфлікт регістру дій (Go: `USER_JOINED` vs Flutter: `userJoined`) та ключів (`sender_id` vs `senderId`). Виявлено потенційний Data Race у `hub.go` при видаленні сокетів під `RLock()`. Стан кімнат треба реально писати в Redis. |
| 4 | **API Контракти клієнта** | [04_API_CONTRACTS_AND_CLIENT_COMPATIBILITY_AUDIT.md](04_API_CONTRACTS_AND_CLIENT_COMPATIBILITY_AUDIT.md) | Конфлікт у CORS (`AllowedOrigins: ["*"]` + `AllowCredentials: true` блокується браузерами). У `ApiClient.getJson` жорсткий каст `Map<String, dynamic>` падає при отриманні `List<dynamic>`. |
| 5 | **Інфраструктура та ресурси** | [05_INFRASTRUCTURE_AND_RESOURCE_AUDIT.md](05_INFRASTRUCTURE_AND_RESOURCE_AUDIT.md) | **Zero Media Traffic на 100% підтверджено** (< 40 МБ трафіку/добу, 150-350 МБ RAM на весь стек). Проте ліміт RAM для app у 128 MB ризикований через витрати пам'яті Argon2id (64 MB на хеш). Потрібно 256 MB. |

---

## 🎯 Пріоритетні виправлення для 100% стабільності продакшену

1. **База даних (`server/internal/repository/postgres/migrations/000001_init.up.sql`)**:
   - Змінити `season INT NOT NULL DEFAULT 0` -> `season INT NULL` та `episode INT NULL`.
   - Застосувати фічу PostgreSQL 16:
     ```sql
     CONSTRAINT uq_user_history UNIQUE NULLS NOT DISTINCT (user_id, media_id, provider_id, season, episode)
     ```
   - Додати поля `rating REAL` та `rating_source VARCHAR(100)` в таблиці `favorites` та `watch_history`.
2. **Селектори парсерів та редиректи**:
   - Виправити селектори Eneyida на `.short_title` та `.short_img`.
   - Дозволити `client.go` слідувати за 301/302 редиректами для дзеркал UAKino (`uakino.best`).
3. **CORS та WebSocket Hub**:
   - У `router.go` прибрати конфлікт `AllowCredentials` або задати точні Origin замість `*`.
   - У `hub.go` перенести видалення клієнтів `delete(clients, client)` під повне блокування `h.mu.Lock()` та синхронізувати camelCase іменування полів із Flutter-клієнтом (`senderId`, `senderName`, `userJoined`, `userLeft`).
4. **Ресурсний ліміт у `docker-compose.yml`**:
   - Підняти ліміт пам'яті для `app` до `256M`, щоб уникнути OOM Killer при кількох одночасних операціях хешування Argon2id.
