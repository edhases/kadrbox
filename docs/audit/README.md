# Аудит Kadrbox — зведений покажчик

Цей каталог містить **7 звітів** (8 файлів, враховуючи цей покажчик). Усі вони — історичні
записи стану коду на момент проведення аудиту.

> Звіт №2 (аналіз парсерів і джерел контенту) видалено: його зміст описував механіку
> видобування контенту з веб-сайтів і не піддається безпечному редагуванню.

> ⚠️ **Як читати цей каталог**
>
> Знахідки всередині звітів **не переписуються** — вони зафіксовують те, що було відкрито тоді.
> Кожен файл має заголовок `## Статус документа` з полями `Status` та `Verified against: <sha>`.
> Актуальний стан виконання — у [`../../MIGRATION_REPORT.md`](../../MIGRATION_REPORT.md).
>
> `Status: fixed` = усі ключові знахідки звіту перевірено на вказаному коміті та виправлено.
> `Status: superseded` = звіт є вхідним документом для плану;
> його знахідки розподілені між агентами і простежуються там.

---

## 🗂️ Основний аудит (5 субагентів)

Проведено аналізом реального коду, структур даних, мережевих відповідей та конфігурацій
(без синтетичних моків).

| № | Напрямок | Звіт | Status | Ключовий висновок |
| :--- | :--- | :--- | :--- | :--- |
| 1 | База даних та міграції | [01_DATABASE_MIGRATION_AUDIT.md](01_DATABASE_MIGRATION_AUDIT.md) | `fixed` | `season/episode = 0` ламало UI («S0 E0» для фільмів). Потрібні `NULL` + `UNIQUE NULLS NOT DISTINCT` (PostgreSQL 16), імпорт favorites/history. **Виправлено.** |
| 3 | Watch Party & WebSocket | [03_WATCH_PARTY_REALTIME_AUDIT.md](03_WATCH_PARTY_REALTIME_AUDIT.md) | `fixed` | Конфлікт регістру/ключів (`USER_JOINED` vs `userJoined`, `sender_id` vs `senderId`), data race у `hub.go`, стан кімнат не писався в Redis. **Виправлено.** |
| 4 | API-контракти клієнта | [04_API_CONTRACTS_AND_CLIENT_COMPATIBILITY_AUDIT.md](04_API_CONTRACTS_AND_CLIENT_COMPATIBILITY_AUDIT.md) | `fixed` | CORS `AllowedOrigins: ["*"]` + `AllowCredentials` блокується браузерами; жорсткий каст `Map<String, dynamic>` падає на `List<dynamic>`. **Виправлено.** |
| 5 | Інфраструктура та ресурси | [05_INFRASTRUCTURE_AND_RESOURCE_AUDIT.md](05_INFRASTRUCTURE_AND_RESOURCE_AUDIT.md) | `fixed` | Zero media traffic підтверджено, але ліміт RAM 128 MB ризикований через Argon2id (64 MB на хеш) — потрібно 256 MB. **Виправлено.** |

---

## 🗂️ Незалежні аудити (3 звіти)

Ці звіти **не входили** до основного п'ятичленного аудиту, але без них вихідний план
(`MIGRATION_REPORT.md`) не виник би.

| # | Звіт | Рядків | Status | Про що |
| :--- | :--- | :--- | :--- | :--- |
| 6 | [INDEPENDENT_BUG_AUDIT_REPORT.md](INDEPENDENT_BUG_AUDIT_REPORT.md) | 602 | `superseded` | Повний перелік багів Go-бекенду та Flutter-клієнту з пріоритетами P0-P3. |
| 7 | [OAUTH_INDEPENDENT_AUDIT_REPORT.md](OAUTH_INDEPENDENT_AUDIT_REPORT.md) | 267 | `superseded` | OAuth2 Telegram / Discord / Google: CSRF `state`, PKCE, redirect allow-list, екранування HTML. |
| 8 | [SILENT_FAILURES_AND_CRASHES_AUDIT_REPORT.md](SILENT_FAILURES_AND_CRASHES_AUDIT_REPORT.md) | 474 | `superseded` | «Тихі» помилки, зависання та падіння — клієнт і бекенд. |

---

## 🎯 Пріоритетні виправлення (зведено)

Усі нижче перелічені пункти станом на поточний коміт **виконано**. Деталі — у
[`../../MIGRATION_REPORT.md`](../../MIGRATION_REPORT.md).

1. **База даних** (`migrations/000001_init.up.sql`)
   - ✅ `season INT NOT NULL DEFAULT 0` → `season INT NULL`, `episode INT NULL` (рядки 44-45)
   - ✅ `UNIQUE NULLS NOT DISTINCT (user_id, media_id, provider_id, season, episode)`
   - ✅ Поля `rating` та `rating_source` додано
   - ✅ `schema_migrations` ведеться (`repository/postgres/db.go`)

2. **Джерела контенту**
   - ✅ Пункт 2 зведено до нуля: механізм видобування контенту винесено з репозиторію
     повністю. Актуальна модель — див. [`../../MIGRATION_REPORT.md`](../../MIGRATION_REPORT.md).

3. **CORS та WebSocket Hub**
   - ✅ `AllowedOrigins: ["*"]` → `AllowOriginFunc` з allow-list (`router.go:42-50`)
   - ✅ Видалення клієнтів перенесено під `h.mu.Lock()` через єдину точку `dropLocked`
     (`ws/hub.go:421-436`); іменування вирівняно з клієнтом

4. **Ресурсні ліміти**
   - ✅ RAM для `app` піднято до `256M` (`docker-compose.yml:41`)

5. **Додатково (після аудиту)**
   - ✅ Обов'язкові env-змінні через `${VAR:?...}`; `JWT_SECRET` ≥ 32 байти з fail-closed валідацією
   - ✅ `Redis --requirepass` + healthcheck з паролем
   - ✅ `readHeaderTimeout` / `maxHeaderBytes` (`cmd/api/main.go:256-259`)
   - ✅ OAuth `state` + PKCE S256, `GETDEL` для refresh-токенів, `RevokeAllForUser`

---

## ⚠️ Незакритий ризик: breaking change у wire-форматі

Після цих аудитів запроваджено єдиний response-envelope
(`backend/internal/transport/http/api_envelope.go`):

```
success, list    {"data":[ … ],"meta":{"limit":200,"offset":0,"count":2,"total":5,"has_more":true}}
success, object  {"data":{ … }}
error            {"error":"…"}
```

Списки більше не повертаються голим масивом. **Клієнт має бути оновлено синхронно** —
див. [`../../MIGRATION_REPORT.md`](../../MIGRATION_REPORT.md), Wave 2G/2H.
