> ## Статус документа
>
> - **Status:** superseded by `docs/REMEDIATION_PLAN.md`
> - **Verified against:** `3ab45ef`
> - **Актуальність:** аудит Telegram/Discord/Google OAuth2. Знахідки перенесені в
>   `docs/REMEDIATION_PLAN.md` (Wave 1D — Auth & OAuth security).
> - **Вибірково перевірено на `3ab45ef`:** серверний `state` nonce з атомарним
>   одноразовим споживанням та PKCE S256 (`transport/http/oauth_state.go`,
>   сховище — `redis.go:300`); точний allow-list redirect
>   (`transport/http/oauth_redirect.go`); екранування HTML-відповіді
>   (`transport/http/html_render.go`).
> - **Не переписано:** історичні знахідки залишено як є.
# Незалежний експертний аудит безпеки та надійності: Telegram & Discord OAuth2
**Проєкт:** Oxide Film (Go Backend & Flutter Client)  
**Роль:** Головний інженер з безпеки та аудиту якості коду (Principal Security & Reliability Auditor)  
**Дата аудиту:** 29 вересня 2026 р.  
**Статус:** ЗАВЕРШЕНО  

---

## 1. Загальна оцінка (Executive Summary)

Було проведено поглиблений, неупереджений та прискіпливий аналіз нової підсистеми зовнішньої авторизації через **Telegram** (Login Widget / HMAC-SHA256) та **Discord** (OAuth2 Authorization Code Grant), реалізованої на стороні Go-сервера (`server/`) та Flutter-клієнта (`lib/`).

### Результати перевірки ключових напрямків:
| Секція аудиту | Вердикт | Короткий підсумок |
|---|:---:|---|
| **1. Криптографічна валідація підпису Telegram** | **PASS** *(з зауваженнями)* | Відповідає офіційному стандарту Telegram (SHA256(bot_token), HMAC-SHA256, constant-time `hmac.Equal`, перевірка вікна застарівання `auth_date`). Виявлено відсутність перевірки конфігурації у Web Login. |
| **2. Обмін токенами Discord OAuth2** | **PASS** | Коректна реалізація Authorization Code Grant: POST запит на токен із `application/x-www-form-urlencoded`, отримання профілю через `/api/users/@me`, обробка скасування користувачем та помилок API. |
| **3. Захист від Open Redirect та безпека `state`** | **WARN** | Функція `isSafeRedirectURL` надійно запобігає перенаправленню на зовнішні зловмисні домени (дозволено лише loopback, custom scheme `oxide://` та `*.oxideteam.pp.ua`). Проте параметр `state` не містить криптографічного CSRF nonce. |
| **4. База даних, міграції та зв'язування акаунтів** | **PASS** *(з зауваженнями)* | Міграція `000004_oauth_providers.up.sql` створює унікальні поля `telegram_id` та `discord_id`. Забезпечено безпечне збереження профілів без конфліктів. Виявлено надлишкові індекси та потенційний оверфлоу довжини імені. |
| **5. Flutter-клієнт та Loopback HTTP-сервер** | **WARN** | Прив'язка до порту `0` та `InternetAddress.loopbackIPv4`, надійне закриття у `finally`, 5-хвилинний таймаут. Проте використання `server.first` вразливе до сторонніх шумових запитів браузера (наприклад, `/favicon.ico`), що може призводити до зриву входу. |

---

## 2. Зведена таблиця знайдених проблем

| ID | Рівень | Категорія | Опис проблеми | Файл / Рядки |
|---|:---:|---|---|---|
| **SEC-OAUTH-01** | **HIGH** | Reliability / Flow | Використання `server.first` у Flutter Loopback сервері без фільтрації URI (падіння при запиті `/favicon.ico`) | `lib/data/services/oxide_server_service.dart:146-149` |
| **SEC-OAUTH-02** | **HIGH** | Security / CSRF | Відсутність валідації CSRF Nonce у параметрі `state` (Login CSRF ризик) | `server/internal/transport/http/auth_handler.go:1037-1041, 1079-1087` |
| **SEC-OAUTH-03** | **MEDIUM** | UX / Reliability | Зависання Flutter-клієнта на 5 хвилин при відмові користувача в Discord/Telegram | `server/internal/transport/http/auth_handler.go:1052-1060`, `oxide_server_service.dart:146` |
| **SEC-OAUTH-04** | **MEDIUM** | UX / Error Handling | `TelegramLoginWeb` не перевіряє наявність `TELEGRAM_BOT_TOKEN` перед генерацією сторінки віджета | `server/internal/transport/http/auth_handler.go:765-770` |
| **SEC-OAUTH-05** | **MEDIUM** | Data Integrity | Ризик помилки SQL `value too long for type character varying(100)` для довгих імен Telegram | `server/internal/transport/http/auth_handler.go:720-726` |
| **SEC-OAUTH-06** | **LOW** | Performance / DB | Дублювання індексів у міграції `000004_oauth_providers.up.sql` (UNIQUE вже створює B-tree індекс) | `server/internal/repository/postgres/migrations/000004_oauth_providers.up.sql:8-9` |
| **SEC-OAUTH-07** | **LOW** | Feature / API | Відсутній публічний/захищений ендпоінт для зв'язування існуючого акаунта з Telegram | `server/internal/transport/http/router.go`, `user_repo.go:201` |
| **SEC-OAUTH-08** | **LOW** | Security / Defense | Відсутність перевірки безпечної схеми `http`/`https` у `isSafeRedirectURL` для хостових URL | `server/internal/transport/http/auth_handler.go:1412-1417` |

---

## 3. Детальний аналіз реалізації

### 3.1. Валідація криптографічного підпису Telegram
**Вердикт: PASS (з зауваженнями)**  
**Файли:** `server/internal/transport/http/auth_handler.go:664-707, 742-762, 828-880`

1. **Відповідність офіційному стандарту Telegram:**
   - **Генерація Secret Key:** За специфікацією Telegram `secret_key = SHA256(bot_token)`. У коді: `sha := sha256.Sum256([]byte(h.telegramBotToken))` та використання зрізу `sha[:]` (32 байти) як ключа HMAC. Це на 100% відповідає специфікації.
   - **Складання `data_check_string`:** Ключі та значення формуються у вигляді `key=<value>`, додаються тільки непорожні параметри (`auth_date`, `first_name`, `id`, `last_name`, `photo_url`, `username`), після чого сортуються за алфавітом через `sort.Strings(parts)` і поєднуються символом `\n`.
   - **Константний час порівняння (Constant-Time):** Застосовано `hmac.Equal([]byte(strings.ToLower(expectedHash)), []byte(strings.ToLower(req.Hash)))`, що виключає ризик Timing Attacks.
   - **Захист від атак повторного відтворення (Replay Attacks):** Перевіряється `now - req.AuthDate > 86400 || req.AuthDate > now + 300`. Вікно валідності — 24 години, з допуском 5 хвилин на розсинхронізацію серверних годинників.
2. **Обробка запитів:**
   - Реалізовано обидва методи: API (`POST /api/v1/auth/telegram`) з декодуванням JSON і веб-віджет (`GET /api/v1/auth/telegram/login` + `GET /api/v1/auth/telegram/callback`).
   - У `TelegramCallbackWeb` параметри `id` та `auth_date` безпечно конвертуються через `strconv.ParseInt(..., 10, 64)`. У разі помилки парсингу встановлюється `0`, що гарантовано бракується у `verifyTelegramAuth`.

---

### 3.2. Обмін токенами Discord OAuth2
**Вердикт: PASS**  
**Файли:** `server/internal/transport/http/auth_handler.go:882-1019, 1021-1117`

1. **Обмін `code` на `access_token`:**
   - Виконується POST-запит на `https://discord.com/api/oauth2/token` із заголовком `Content-Type: application/x-www-form-urlencoded` та тілом `grant_type=authorization_code`, `client_id`, `client_secret`, `code`, `redirect_uri`.
   - Таймаут клієнта обмежено до 10 секунд через `client := &http.Client{Timeout: 10 * time.Second}`, з передачею контексту запиту `r.Context()`.
2. **Отримання профілю користувача:**
   - Виконується GET-запит на `https://discord.com/api/users/@me` з заголовком `Authorization: Bearer <access_token>`.
   - Отримуються `id`, `username`, `global_name`, `avatar`, `email`, `verified`.
   - Коректно формується посилання на аватар через офіційний CDN Discord: `https://cdn.discordapp.com/avatars/{id}/{avatar}.png`.
3. **Обробка помилок:**
   - При відмові користувача в авторизації перевіряється відсутність `code` та вичитується `error_description`, після чого користувачеві виводиться інформативна сторінка помилки (400 Bad Request).
   - Якщо на сервері не вказано `DISCORD_CLIENT_ID`, ендпоінт відразу повертає `503 Service Unavailable`.
   - Підтримується як веб-флоу, так і прямий API-ендпоінт `POST /api/v1/auth/discord` для нативних клієнтів.

---

### 3.3. Безпека та захист від Open Redirect
**Вердикт: WARN**  
**Файли:** `server/internal/transport/http/auth_handler.go:1401-1419`

1. **Функція `isSafeRedirectURL`:**
   ```go
   func isSafeRedirectURL(rawURL string) bool {
       if rawURL == "" { return false }
       u, err := url.Parse(rawURL)
       if err != nil { return false }
       if u.Scheme == "oxide" { return true }
       if u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" { return true }
       if u.Hostname() == "film.oxideteam.pp.ua" || u.Hostname() == "oxideteam.pp.ua" || strings.HasSuffix(u.Hostname(), ".oxideteam.pp.ua") {
           return true
       }
       return false
   }
   ```
   - Захист надійно запобігає класичним атакам Open Redirect: спроби передати `https://evil.com` або `https://film.oxideteam.pp.ua.evil.com` відхиляються.
   - Специфікація deep-link схеми `oxide://` дозволена для мобільного та десктопного додатків.
   - Хости `127.0.0.1` та `localhost` дозволені для локального loopback-сервера Flutter.
2. **Слабке місце (SEC-OAUTH-02):**
   - У поточному коді параметр `state` використовується **виключно** як носій URL редиректу (`redirect_to`).
   - Відповідно до стандарту OAuth 2.0 (RFC 6749, Sec. 10.12), `state` зобов'язаний містити непередбачуваний криптографічний токен проти CSRF (Login CSRF Attack). Зловмисник може згенерувати посилання на авторизацію з власним кодом та підсунути його жертві.

---

### 3.4. База даних, міграції та зв'язування акаунтів
**Вердикт: PASS (з зауваженнями)**  
**Файли:** `server/internal/repository/postgres/migrations/000004_oauth_providers.up.sql`, `server/internal/repository/postgres/user_repo.go`

1. **Міграції:**
   - Додано стовпці `telegram_id BIGINT UNIQUE` та `discord_id VARCHAR(64) UNIQUE`.
   - Завдяки `IF NOT EXISTS` міграція успішно накатується автоматично через вбудований механізм `embed.FS` на старті бекенду без падінь.
2. **Методи репозиторію:**
   - `GetUserByTelegramID`, `GetUserByDiscordID`, `LinkTelegram`, `LinkDiscord`, `CreateOAuthUser` використовують параметризовані SQL-запити (`$1, $2`), що виключає SQL Injection.
   - Усі запити повертають повний набір із 12 полів моделі `User`, що збігається з сигнатурою методу `scanUser`.
   - Значення `telegram_id` та `discord_id` коректно зчитуються у покажчики `*int64` та `*string`, підтримуючи значення `NULL` для звичайних користувачів.
3. **Логіка зв'язування акаунтів:**
   - **Discord:** Якщо користувач з таким email уже зареєстрований, система автоматично прив'язує `discord_id` через `LinkDiscord`, оновлює аватар (якщо був пустий) і, якщо пошта підтверджена в Discord (`discordUser.Verified`), автоматично позначає `is_verified = true`. Це зручно та безпечно.
   - **Telegram:** Оскільки Telegram Login Widget не надає email, для нових користувачів генерується плейсхолдер `tg_<id>@telegram.oxide`. Проте метод `LinkTelegram` не виведений у жоден HTTP-ендпоінт, тому користувач з існуючим акаунтом не має змоги прив'язати свій Telegram у профілі (створюється окремий новий акаунт).

---

### 3.5. Flutter-клієнт та Loopback HTTP-сервер
**Вердикт: WARN**  
**Файли:** `lib/data/services/oxide_server_service.dart:130-195`, `lib/data/services/auth_service.dart:156-175`, `lib/presentation/pages/auth/login_page.dart:405-498`

1. **Loopback Server (`signInWithOAuthLoopback`):**
   - `HttpServer.bind(InternetAddress.loopbackIPv4, 0)`: прив'язка строго до локального інтерфейсу на випадково виділений ОС вільний порт — повністю відповідає вимогам безпеки RFC 8252 (OAuth 2.0 for Native Apps).
   - Блок `finally { await server?.close(force: true); }` гарантує, що локальний сокет буде звільнено за будь-яких обставин (успіх, таймаут чи виняток).
   - Ендпоінт формується з `AppConfig.serverApiUrl` як `${AppConfig.serverApiUrl}/auth/$provider/login` (`/api/v1/auth/discord/login` та `/api/v1/auth/telegram/login`), що відповідає зареєстрованим маршрутам у Chi.
2. **Слабке місце (SEC-OAUTH-01):**
   - Рядок `final request = await server.first.timeout(...)` бере **найперший** HTTP-запит, який надійде на порт. Якщо браузер або розширення перед переходом або одночасно виконає запит на `/favicon.ico` або перевірку порту, `server.first` перехопить його. Параметри `access_token` будуть відсутні, сервер викине помилку, виконає `finally` і закриє порт. Реальний редирет з токенами отримає `Connection Refused`.
3. **UI / UX:**
   - У `LoginPage` додано стильні кнопки для Discord та Telegram з фірмовими кольорами (`#5865F2` та `#229ED9`), векторними іконками з `flutter_vector_icons`.
   - На час запиту кнопки блокуються (`_isLoading`), що унеможливлює паралельне відкриття кількох сокетів.
   - Після успішної авторизації викликається `_checkLocalDataAndSync()` для пропозиції синхронізувати локальні закладки/історію перегляду з серверною БД Drift.

---

## 4. Детальний опис виявлених дефектів та вразливостей

---

### [HIGH] SEC-OAUTH-01: Використання `server.first` у Loopback сервері без валідації шляху
- **Файл:** [oxide_server_service.dart](file:///e:/Github/oxide_film/lib/data/services/oxide_server_service.dart#L146-L154) (рядки 146–154)
- **Суть проблеми:**  
  `HttpServer` у Dart транслює потік усіх запитів, що надходять на сокет. Виклик `await server.first` очікує рівно один будь-який запит. Сучасні десктопні браузери (Google Chrome, MS Edge, Brave) при відкритті вкладки або редиректі часто паралельно надсилають фоновий запит на `/favicon.ico`. Якщо запит favicon прийде раніше або першим, `server.first` вважатиме його за відповідь авторизації, `params['access_token']` буде `null`, метод кине виняток і закриє сервер. Користувач побачить помилку авторизації.
- **Спосіб виправлення:**  
  Замінити `server.first` на цикл очікування запиту за цільовим шляхом `/callback`, а на всі інші запити повертати 404:

```dart
// Було:
final request = await server.first.timeout(
  const Duration(minutes: 5),
  onTimeout: () => throw TimeoutException('Час очікування авторизації вичерпано'),
);

// Рекомендовано:
HttpRequest? authRequest;
await for (final req in server.timeout(
  const Duration(minutes: 5),
  onTimeout: (sink) => sink.close(),
)) {
  if (req.uri.path == '/callback') {
    authRequest = req;
    break;
  } else {
    req.response.statusCode = HttpStatus.notFound;
    await req.response.close();
  }
}
if (authRequest == null) {
  throw TimeoutException('Час очікування авторизації вичерпано');
}
final request = authRequest;
```

---

### [HIGH] SEC-OAUTH-02: Відсутність валідації CSRF Nonce у параметрі `state`
- **Файл:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L1037-L1041) (рядки 1037–1041, 1079–1087)
- **Суть проблеми:**  
  Параметр `state` передається авторизаційному серверу Discord без криптографічного зв'язування з сесією клієнта. Значення `redirect_to` напряму кладеться в `state`:
  ```go
  if state := r.URL.Query().Get("state"); state != "" {
      params.Set("state", state)
  } else if redirect := r.URL.Query().Get("redirect_to"); redirect != "" {
      params.Set("state", redirect)
  }
  ```
  Зловмисник може згенерувати посилання на вхід через власний акаунт Discord і спровокувати жертву відкрити callback-посилання, що призведе до прив'язки акаунта зловмисника до сесії жертви (Login CSRF).
- **Спосіб виправлення:**  
  Зберігати або підписувати `state` за допомогою HMAC-SHA256 від випадкового `nonce` та `redirect_to`:
  ```go
  // Формат state: base64(json({nonce, redirect_to, signature}))
  // або збереження nonce у Redis з TTL 10 хвилин перед редиректом
  ```

---

### [MEDIUM] SEC-OAUTH-03: Зависання клієнта на 5 хвилин при помилці авторизації
- **Файл:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L1052-L1066) (рядки 1052–1066)
- **Суть проблеми:**  
  Якщо користувач у вікні Discord натискає «Скасувати» (Cancel), Discord повертає помилку `?error=access_denied`. Хендлер `DiscordCallback` перехоплює це і рендерить HTML-помилку в браузері:
  ```go
  if code == "" {
      w.WriteHeader(http.StatusBadRequest)
      _, _ = w.Write([]byte(renderOAuthStatusHTML(false, "Помилка Discord", errDesc, "", "", "")))
      return
  }
  ```
  При цьому редирект на `state` (локальний HTTP-сервер додатка `http://127.0.0.1:<port>/callback`) **не виконується**. Додаток продовжує висіти у стані очікування впродовж 5 хвилин, доки не спрацює загальний таймаут.
- **Спосіб виправлення:**  
  Якщо `state` валідний за `isSafeRedirectURL(state)`, перенаправляти на нього з параметром помилки `?error=...`:
  ```go
  if code == "" {
      errDesc := r.URL.Query().Get("error_description")
      if errDesc == "" {
          errDesc = "Авторизацію скасовано"
      }
      state := r.URL.Query().Get("state")
      if isSafeRedirectURL(state) {
          sep := "?"
          if strings.Contains(state, "?") { sep = "&" }
          http.Redirect(w, r, fmt.Sprintf("%s%serror=%s", state, sep, url.QueryEscape(errDesc)), http.StatusTemporaryRedirect)
          return
      }
      w.WriteHeader(http.StatusBadRequest)
      _, _ = w.Write([]byte(renderOAuthStatusHTML(false, "Помилка Discord", errDesc, "", "", "")))
      return
  }
  ```
  А в Dart-клієнті перевіряти:
  ```dart
  if (params['error'] != null) {
    throw Exception('Авторизацію відхилено: ${params['error']}');
  }
  ```

---

### [MEDIUM] SEC-OAUTH-04: `TelegramLoginWeb` рендерить віджет навіть якщо бот не налаштований
- **Файл:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L765-L770) (рядки 765–770)
- **Суть проблеми:**  
  У `DiscordLogin` є перевірка `if h.discordClientID == ""`, яка повертає 503 Service Unavailable, якщо змінні оточення не задані. У `TelegramLoginWeb` такої перевірки немає: якщо `TELEGRAM_BOT_TOKEN` не налаштовано, сторінка з віджетом все одно віддається користувачеві з дефолтним ботом `oxidefilmbot`. Коли користувач тисне на віджет і повертається у `TelegramCallbackWeb`, валідація підпису падає з 401 Unauthorized замість зрозумілого попередження про неналаштований сервіс.
- **Спосіб виправлення:**  
  Додати перевірку токена бота на початку `TelegramLoginWeb`:
  ```go
  if h.telegramBotToken == "" {
      w.Header().Set("Content-Type", "text/html; charset=utf-8")
      w.WriteHeader(http.StatusServiceUnavailable)
      _, _ = w.Write([]byte(renderOAuthStatusHTML(false, "Telegram недоступний", "TELEGRAM_BOT_TOKEN не налаштований на сервері", "", "", "")))
      return
  }
  ```

---

### [MEDIUM] SEC-OAUTH-05: Ризик переповнення довжини імені користувача в PostgreSQL
- **Файл:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L720-L726) (рядки 720–726)
- **Суть проблеми:**  
  У базі даних стовпець `username` має обмеження `VARCHAR(100) NOT NULL`. У Telegram `first_name` може бути до 64 символів і `last_name` до 64 символів. Конкатенація `strings.TrimSpace(req.FirstName + " " + req.LastName)` може досягати 129 символів. При спробі створення такого користувача через `CreateOAuthUser` Postgres поверне фатальну помилку `pq: value too long for type character varying(100)` (HTTP 500).
- **Спосіб виправлення:**  
  Обрізати `displayName` перед збереженням:
  ```go
  if len(displayName) > 100 {
      displayName = displayName[:100]
  }
  ```

---

### [LOW] SEC-OAUTH-06: Надлишкові індекси у міграції 000004
- **Файл:** [000004_oauth_providers.up.sql](file:///e:/Github/oxide_film/server/internal/repository/postgres/migrations/000004_oauth_providers.up.sql#L4-L9) (рядки 4–9)
- **Суть проблеми:**  
  Коли стовпці оголошуються з атрибутом `UNIQUE`:
  ```sql
  ADD COLUMN IF NOT EXISTS telegram_id BIGINT UNIQUE,
  ADD COLUMN IF NOT EXISTS discord_id VARCHAR(64) UNIQUE;
  ```
  PostgreSQL автоматично створює унікальні B-tree індекси. Наступні команди:
  ```sql
  CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
  CREATE INDEX IF NOT EXISTS idx_users_discord_id ON users(discord_id);
  ```
  створюють дублюючі звичайні індекси на ті самі стовпці, що даремно витрачає дисковий простір та сповільнює операції `INSERT`/`UPDATE`.
- **Спосіб виправлення:**  
  Видалити окремі `CREATE INDEX` або замінити на створення одного унікального індексу без атрибута в `ADD COLUMN`.

---

### [LOW] SEC-OAUTH-07: Відсутній HTTP-ендпоінт для прив'язки існуючого акаунта до Telegram
- **Файл:** `server/internal/transport/http/router.go`, `server/internal/repository/postgres/user_repo.go:201`
- **Суть проблеми:**  
  У репозиторії реалізовано метод `LinkTelegram(ctx, userID, telegramID)`, проте в `router.go` немає маршруту на кшталт `POST /api/v1/auth/telegram/link`. Оскільки Telegram не передає email, користувач, який уже має акаунт в Oxide Film і входить через Telegram, щоразу потрапляє у новий відокремлений акаунт `tg_<id>@telegram.oxide`.
- **Спосіб виправлення:**  
  Додати захищений ендпоінт у групу `AuthMiddleware`, який приймає перевірений віджет Telegram і прив'язує його до поточного `userID`.

---

### [LOW] SEC-OAUTH-08: `isSafeRedirectURL` не перевіряє схему протоколу для мережевих адрес
- **Файл:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L1412-L1417) (рядки 1412–1417)
- **Суть проблеми:**  
  Для хостів `127.0.0.1`, `localhost` та `*.oxideteam.pp.ua` перевіряється лише ім'я хоста, але не перевіряється схема (`http` чи `https`).
- **Спосіб виправлення:**  
  Додати перевірку: для `127.0.0.1` та `localhost` схема має бути строго `http`, для доменів `oxideteam.pp.ua` — строго `https`.

---

## 5. Резюме та висновок аудитора

Реалізація зовнішньої авторизації через **Telegram** та **Discord** виконана на високому інженерному рівні:
1. Криптографія Telegram повністю відповідає офіційним специфікаціям Telegram Login Widget (включно з захистом від Timing Attacks та Replay Attacks).
2. Інтеграція Discord суворо слідує стандарту OAuth 2.0 Authorization Code Grant.
3. Додатковий захист від Open Redirect успішно блокує витік токенів на сторонні хости.
4. Взаємодія з базою даних pgxpool побудована надійно, без SQL-ін'єкцій та з повною підтримкою сканування полів.
5. Flutter-клієнт використовує стандартний сучасний механізм локального Loopback HTTP-сервера на порту 0 з обов'язковим закриттям сокетів у `finally`.

Для досягнення найвищого стандарту безпеки та стабільності (Enterprise-grade) рекомендується внести виправлення згідно з наведеними code diff, насамперед усунувши вразливість до шумових запитів у Flutter Loopback сервері (**SEC-OAUTH-01**) та забезпечивши захист від Login CSRF (**SEC-OAUTH-02**).
