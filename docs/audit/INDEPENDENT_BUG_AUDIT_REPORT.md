# Звіт незалежного аудиту якості коду та безпеки: Oxide Film (Go Backend & Flutter Client)

**Дата аудиту:** 28 вересня 2026 року  
**Статус:** Виявлено критичні вразливості та архітектурні дефекти  
**Аудитор:** Principal Security & Reliability Auditor  
**Об'єкти перевірки:**
- Go Backend (`server/`)
- Flutter Client (`lib/`)

---

## 1. Загальна оцінка надійності та безпеки (Executive Summary)

Проєкт **Oxide Film** є амбітною мультиплатформенною системою онлайн-кінотеатру, що поєднує високоефективний бекенд на мові Go (архітектура Chi + PostgreSQL 16 + Redis) та повнофункціональний клієнт на Flutter з підтримкою Desktop/Mobile, локальної бази даних Drift, механізму синхронізації та Watch Party (спільного перегляду через WebSocket та P2P WebRTC).

Кодова база демонструє високу інженерну культуру: присутні юніт-тести, типізація, розділення на шари DAO/репозиторіїв та застосування патернів Offline-First. Проте ретельний аналіз виявив **низку критичних дефектів безпеки, конкурентності та сумісності контрактів**, які можуть призвести до повної компрометації облікових записів користувачів, взаємного блокування (deadlock) бекенду при навантаженні, витоків горутин та втрати користувацьких даних синхронізації:

1. **Критичні проблеми автентифікації та безпеки (Security):**
   - У хендлері `GoogleAuth` повністю відсутня перевірка `aud` (Client ID) та статусу верифікації email. Будь-який дійсний токен Google ID для будь-якого стороннього застосунку дозволяє захопити довільний акаунт на сервері (CWE-287 / CWE-347).
   - Конфігурація CORS динамічно повертає заголовок `Access-Control-Allow-Origin: <origin>` для будь-якого сайту разом із `Access-Control-Allow-Credentials: true` (CWE-942), що відкриває можливість викрадення приватних даних через браузер.
   - Ендпоінт роздачі завантажень `/uploads/` використовує дефолтний `http.FileServer`, що дозволяє Directory Listing (перегляд списку всіх файлів та UUID користувачів).
   - Завантаження аватарів перевіряє лише розширення файлу з імені клієнта, не валідуючи Magic Bytes / MIME-тип.

2. **Критичні дефекти конкурентності (Deadlocks & Goroutine Leaks):**
   - У `ws.Hub.Run()` виявлено потенційний self-deadlock: горутина `h.Run()` сама пише в канал `h.broadcast <- event` при реєстрації нових клієнтів. При заповненні буфера (256 подій) `h.Run()` блокується на записі, що призводить до перманентного зависання всього WebSocket хабу кімнат.
   - При завершенні роботи сервера (`GracefulStop`) `h.Run()` завершується, залишаючи відкритим канал `unregister`. Як наслідок, усі підключені клієнти при закритті назавжди зависають у горутині `readPump` на відправці в `unregister`.
   - Виявлено race condition при записі в Gorilla WebSocket під час зупинки сервера (`WriteMessage` викликається одночасно з `writePump`).

3. **Критичні невідповідності контрактів (API & Cloud Sync Mismatches):**
   - Клієнт `OxideServerService` надсилає дані збереження історії та обраного в `camelCase` (`mediaId`, `providerId`, `posterUrl`, `positionMs`), тоді як Go-сервер очікує `snake_case` (`media_id`, `provider_id`). Через це десеріалізація в Go обнуляє ці поля — прогрес перегляду та обране записуються в базу з порожніми ключами!
   - У `FavoritesService` видалення тайтла з обраного локально взагалі не викликає сервер, якщо `isFavorite == false`. Через це видалені фільми повторно завантажуються з сервера при кожному запуску застосунку ("Zombie Favorites").
   - У Flutter відсутній перехоплювач HTTP 401 для ротації JWT access-токена: через 15 хвилин після запуску всі запити синхронізації починають падати з помилкою Unauthorized.

---

## 2. Зведена таблиця виявлених дефектів

| ID | Компонент | Рівень | Назва проблеми | CWE / Категорія |
|---|---|---|---|---|
| **BUG-GO-01** | `server/auth` | **CRITICAL** | Google OAuth ID Token Impersonation (Відсутність валідації `aud` і `email_verified`) | CWE-287 / CWE-347 |
| **BUG-GO-02** | `server/ws` | **CRITICAL** | WebSocket Hub Deadlock через буферизацію каналу `h.broadcast` | CWE-833 (Deadlock) |
| **BUG-GO-03** | `server/ws` | **CRITICAL** | Goroutine Leak клієнтських горутин `readPump` при GracefulStop | CWE-400 (Resource Leak) |
| **BUG-GO-04** | `server/ws` | **HIGH** | Race Condition: одночасний запис у WebSocket з двох горутин у `GracefulStop` | CWE-362 (Race Condition) |
| **BUG-GO-05** | `server/router` | **HIGH** | Небезпечний динамічний CORS з підтримкою `AllowCredentials` | CWE-942 (Permissive CORS) |
| **BUG-GO-06** | `server/router` | **HIGH** | Directory Listing у роздачі завантажених файлів `/uploads/` | CWE-548 (Info Disclosure) |
| **BUG-GO-07** | `server/auth` | **HIGH** | Відсутність перевірки Magic Bytes / MIME-типу в `UploadAvatar` | CWE-434 (Unrestricted Upload) |
| **BUG-GO-08** | `server/docker` | **HIGH** | Відсутність Docker Volume для збереження завантажених аватарів | Data Loss |
| **BUG-GO-09** | `server/auth` | **HIGH** | Відсутність інвалідації Redis сесій при зміні/скиданні пароля та видаленні акаунту | CWE-613 (Session Expiration) |
| **BUG-GO-10** | `server/ws` | **HIGH** | Відсутність підписки на Redis Pub/Sub у хабі Watch Party (Dead Code) | Scalability / Architecture |
| **BUG-GO-11** | `server/config` | **MEDIUM** | Жорстко закодований резервний JWT секрет у конфігурації | CWE-798 (Hardcoded Secret) |
| **BUG-GO-12** | `server/provider`| **MEDIUM** | Скасування контексту першого клієнта перериває запити інших у `SingleFlightSearch` | Concurrency / Availability |
| **BUG-GO-13** | `server/http` | **MEDIUM** | Некоректні коди помилок (маскування збоїв БД під 409) та Content-Type `text/plain` | API Design |
| **BUG-GO-14** | `server/db` | **MEDIUM** | Небезпечне багаторазове виконання міграцій без блокування та таблиці обліку | Database Integrity |
| **BUG-GO-15** | `server/email` | **MEDIUM** | Синхронне блокування HTTP-запитів реєстрації зовнішнім Resend API | Latency / DoS |
| **BUG-GO-16** | `server/http` | **MEDIUM** | Мертвий код CacheRepository у `ContentHandler` (відсутність кешування) | Performance |
| **BUG-GO-18** | `server/postgres`| **LOW** | Неможливість скинути `bio` та `avatar_url` на порожнє значення у `UpdateProfile` | Logic Flaw |
| **BUG-FL-01** | `lib/services` | **CRITICAL** | Контрактний розрив регістру полів (camelCase vs snake_case) у Cloud Sync | Contract Mismatch |
| **BUG-FL-02** | `lib/services` | **CRITICAL** | Помилка синхронізації обраного: неможливо видалити фільм із сервера ("Zombie Favorites") | Data Integrity |
| **BUG-FL-03** | `lib/network` | **HIGH** | Відсутність автоматичного перехоплення 401 (Refresh Token Interceptor) у `ApiClient` | Authentication Flow |
| **BUG-FL-04** | `lib/network` | **HIGH** | Втрата тексту помилки бекенду у `ApiClient._handleDioError` | UX / Error Handling |
| **BUG-FL-05** | `lib/services` | **HIGH** | Ігнорування розриву WebSocket у `WatchPartyService` (UI зависає у стані `connected`) | Realtime Reliability |
| **BUG-FL-06** | `lib/services` | **HIGH** | Конфлікт блокування екрана (Wakelock) між `DownloadService` та `PlayerController` | State Race Condition |
| **BUG-FL-07** | `lib/services` | **MEDIUM** | Відсутність методу `dispose()` у `WatchPartyService` (витік `_heartbeatTimer`) | Memory / Resource Leak |
| **BUG-FL-08** | `lib/player` | **MEDIUM** | Небезпечне оновлення стану після асинхронного `open()` у `PlayerPage` | UI Crash / setState Leak |
| **BUG-FL-09** | `lib/services` | **MEDIUM** | Помилка видалення лісенера у `VideoPlayerService.close()` | Listener Leak |
| **BUG-FL-10** | `lib/database` | **MEDIUM** | `HistoryDao.saveProgress` ігнорує переданий `watchedAt` при вставці нових рядків | Data Sync Accuracy |
| **BUG-FL-11** | `lib/services` | **MEDIUM** | `EpisodeUpdateService` втрачає стан відомих епізодів при перезапуску додатка | Logic Flaw |
| **BUG-FL-12** | `lib/player` | **LOW** | Невивільнений `ValueNotifier<Duration>` у `PlayerController.dispose()` | Memory Leak |
| **BUG-FL-13** | `lib/auth` | **LOW** | Витік `TextEditingController` у модальних діалогах відновлення/зміни пароля | Memory Leak |
| **BUG-FL-14** | `lib/content` | **LOW** | Відсутність обробки `onCancel` у `UnifiedContentRepositoryImpl._streamProviders` | Background Waste |

---

## 3. Детальний опис знайдених дефектів

### Блок 1: Go Backend (`server/`)

#### BUG-GO-01 [CRITICAL]: Google OAuth ID Token Impersonation (CWE-287 / CWE-347)
- **Файл та рядки:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L498-L533)
- **У чому полягає проблема:**
  У функції `GoogleAuth` токен клієнта валідується через відправку запиту на `https://oauth2.googleapis.com/tokeninfo?id_token=...`. Отримана відповідь містить email користувача. Проте обробник:
  1. **Взагалі не перевіряє поле `aud`** (Audience / Client ID). Google підтверджує валідність токена для *будь-якого* застосунку в екосистемі Google. Зловмисник може створити власний Google OAuth проект, отримати токен на свій клієнт зі своєю поштою або чужою Gmail-поштою, надіслати його в Oxide Server і успішно увійти!
  2. **Не перевіряє статус підтвердження пошти (`email_verified`)**. Якщо в Google обліковому записі пошта не верифікована, вона все одно вважається валідною.
  3. Не використовує таймаут або `r.Context()` при виклику `http.Get(tokenURL)`, що може заблокувати горутину.
- **Вектор атаки:**
  Зловмисник бере будь-який валідний ID-токен для будь-якого свого додатку і авторизується від імені будь-якої пошти жертви. Якщо жертва зареєстрована під цією поштою — відбувається повне захоплення облікового запису (Account Takeover).
- **Спосіб виправлення:**
  Додати `GoogleClientID` до конфігурації сервера та суворо перевіряти `info.Aud == cfg.GoogleClientID` та `info.EmailVerified == "true"`:

```go
// У config/config.go додати:
GoogleClientID string // env: GOOGLE_CLIENT_ID

// У internal/transport/http/auth_handler.go:
var info struct {
    Aud           string `json:"aud"`
    Email         string `json:"email"`
    EmailVerified string `json:"email_verified"`
    Name          string `json:"name"`
    Picture       string `json:"picture"`
}
if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Email == "" {
    jsonError(w, "invalid google token payload", http.StatusUnauthorized)
    return
}
if h.googleClientID != "" && info.Aud != h.googleClientID {
    jsonError(w, "token was not issued for this client", http.StatusUnauthorized)
    return
}
if info.EmailVerified != "true" {
    jsonError(w, "google email is not verified", http.StatusForbidden)
    return
}
```

---

#### BUG-GO-02 [CRITICAL]: WebSocket Hub Permanent Self-Deadlock на каналі `h.broadcast`
- **Файл та рядки:** [hub.go](file:///e:/Github/oxide_film/server/internal/transport/ws/hub.go#L54-L138)
- **У чому полягає проблема:**
  У методі `Run()` горутина хабу одноосібно вичитує події з каналів `h.register`, `h.unregister`, `h.broadcast` та `h.stopChan`:
  ```go
  case client := <-h.register:
      // ...
      h.broadcast <- event // Рядок 90
  case client := <-h.unregister:
      // ...
      h.broadcast <- event // Рядок 115
  ```
  Канал `h.broadcast` ініціалізовано з ємністю 256 (`make(chan *domain.WatchPartyEvent, 256)`). Єдина горутина, яка споживає повідомлення з `h.broadcast` — це сама `h.Run()`. Коли буфер каналу заповнюється (наприклад, 256 користувачів швидко зайшли або надійшла серія повідомлень, яку клієнти не встигли розібрати), `h.broadcast <- event` **блокує виконання `h.Run()`**. Оскільки `h.Run()` заблокована, вона ніколи не перейде до гілки `case event := <-h.broadcast`, щоб вичитати з каналу хоча б одне повідомлення!
- **Наслідки:** Повний і безповоротний взаємний блокування (Deadlock) усього хабу кімнат. Жоден клієнт більше не зможе підключитися, надіслати повідомлення або синхронізуватися до повного перезапуску сервера.
- **Спосіб виправлення:**
  Безпосередньо викликати внутрішній метод розсилки `h.broadcastEvent(event)` або використовувати неблокуючу відправку `select { case h.broadcast <- event: default: }`, а найкраще — прибрати рекурсивний запис у власний канал:

```diff
--- a/server/internal/transport/ws/hub.go
+++ b/server/internal/transport/ws/hub.go
@@ -87,7 +87,7 @@ func (h *Hub) Run() {
 			if h.redisClient != nil {
 				_ = h.redisClient.PublishWatchPartyEvent(context.Background(), client.roomCode, event)
 			}
-			h.broadcast <- event
+			h.broadcastToRoom(event)

 		case client := <-h.unregister:
@@ -112,7 +112,7 @@ func (h *Hub) Run() {
 			if h.redisClient != nil {
 				_ = h.redisClient.PublishWatchPartyEvent(context.Background(), client.roomCode, event)
 			}
-			h.broadcast <- event
+			h.broadcastToRoom(event)
```

---

#### BUG-GO-03 [CRITICAL]: Goroutine Leak клієнтських горутин `readPump` при GracefulStop
- **Файл та рядки:** [hub.go](file:///e:/Github/oxide_film/server/internal/transport/ws/hub.go#L57-L70), [hub.go](file:///e:/Github/oxide_film/server/internal/transport/ws/hub.go#L180-L184)
- **У чому полягає проблема:**
  При надходженні сигналу завершення роботи `GracefulStop()` закриває `stopChan`. У `h.Run()` спрацьовує:
  ```go
  case <-h.stopChan:
      // ... закриває з'єднання ...
      return
  ```
  Після `return` головна горутина `h.Run()` припиняє роботу. Тепер подивимося на деструктор `readPump()` кожного клієнта:
  ```go
  func (c *Client) readPump() {
      defer func() {
          c.hub.unregister <- c // БЛОКУВАННЯ НАЗАВЖДИ!
          c.conn.Close()
      }()
  ```
  Оскільки `h.Run()` вже вийшла, канал `c.hub.unregister` більше ніхто не вичитує! Кожна горутина `readPump()` кожного підключеного користувача назавжди зависає на операції запису в небуферизований канал `unregister`.
- **Спосіб виправлення:**
  Зробити канал `unregister` захищеним від закриття або використовувати `select` з перевіркою `stopChan`:

```go
defer func() {
    select {
    case c.hub.unregister <- c:
    case <-c.hub.stopChan:
    }
    c.conn.Close()
}()
```

---

#### BUG-GO-04 [HIGH]: Race Condition та паніка при одночасному записі у WebSocket у `GracefulStop`
- **Файл та рядки:** [hub.go](file:///e:/Github/oxide_film/server/internal/transport/ws/hub.go#L61-L64)
- **У чому полягає проблема:**
  У `case <-h.stopChan:` горутина `h.Run()` викликає:
  ```go
  _ = client.conn.WriteMessage(
      websocket.CloseMessage,
      websocket.FormatCloseMessage(websocket.CloseGoingAway, "server restarting"),
  )
  ```
  Водночас для кожного клієнта працює горутина `writePump()`, яка за таймером кожні 20 секунд викликає `c.conn.WriteMessage(websocket.PingMessage, nil)`. Згідно з офіційною специфікацією Gorilla WebSocket: *"Connections support one concurrent reader and one concurrent writer"*. Одночасний виклик `WriteMessage` з двох горутин спричиняє паніку під час рестарту сервера (`concurrent write to websocket connection`).
- **Спосіб виправлення:**
  Передавати закриваючий сигнал через канал клієнта `client.send`, який обслуговується виключно горутиною `writePump`:

```go
// У h.Run() при stopChan замість прямого запису в conn:
close(client.send) // writePump побачить закриття і коректно надішле CloseMessage
```

---

#### BUG-GO-05 [HIGH]: Небезпечний динамічний CORS з підтримкою `AllowCredentials` (CWE-942)
- **Файл та рядки:** [router.go](file:///e:/Github/oxide_film/server/internal/transport/http/router.go#L29-L38)
- **У чому полягає проблема:**
  ```go
  r.Use(cors.Handler(cors.Options{
      AllowOriginFunc: func(r *http.Request, origin string) bool {
          return true // дозволяє будь-який origin динамічно зі збереженням credentials
      },
      AllowCredentials: true,
      // ...
  }))
  ```
  Повернення `true` для будь-якого Origin при ввімкненому `AllowCredentials: true` є грубим порушенням моделі безпеки Same-Origin Policy. Будь-який сторонній сайт (наприклад, зловмисний `evil-site.com`), відкритий користувачем у браузері, може виконувати фонові автентифіковані AJAX-запити до Oxide Server і читати історію, профіль або змінювати налаштування.
- **Спосіб виправлення:**
  Обмежити перелік дозволених origins білим списком (наприклад, домен застосунку, localhost для розробки):

```go
AllowedOrigins: []string{
    "https://film.oxideteam.pp.ua",
    "http://localhost:*",
    "tauri://localhost",
},
```

---

#### BUG-GO-06 [HIGH]: Directory Listing у роздачі завантажених файлів `/uploads/`
- **Файл та рядки:** [router.go](file:///e:/Github/oxide_film/server/internal/transport/http/router.go#L51)
- **У чому полягає проблема:**
  ```go
  r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./data/uploads"))))
  ```
  Стандартний `http.FileServer` у Go автоматично генерує HTML-сторінку зі списком усіх файлів та підпапок, якщо запит надходить на директорію (`GET /uploads/avatars/`). Оскільки назви аватарів містять UUID користувачів (`<user_id>.png`), будь-який неавторизований користувач може отримати повний перелік усіх зареєстрованих ID користувачів платформи.
- **Спосіб виправлення:**
  Огорнути `http.FileSystem` у структуру, що блокує відкриття директорій (повертає 404/403 для `os.FileInfo.IsDir()`).

---

#### BUG-GO-07 [HIGH]: Відсутність перевірки Magic Bytes / MIME-типу в `UploadAvatar`
- **Файл та рядки:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L458-L462)
- **У чому полягає проблема:**
  Перевірка розширення виконується виключно за рядком імені файлу клієнта:
  ```go
  ext := strings.ToLower(filepath.Ext(header.Filename))
  if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" { ... }
  ```
  Сервер не перевіряє сигнатуру файлу (Magic Bytes) через `http.DetectContentType` і не декодує зображення. Користувач може завантажити будь-який шкідливий бінарний файл або поліглот під розширенням `.png`.
- **Спосіб виправлення:**
  Зчитати перші 512 байтів і перевірити MIME-тип:
  ```go
  buf := make([]byte, 512)
  n, _ := file.Read(buf)
  mime := http.DetectContentType(buf[:n])
  if !strings.HasPrefix(mime, "image/") {
      jsonError(w, "file content is not a valid image", http.StatusBadRequest)
      return
  }
  file.Seek(0, io.SeekStart)
  ```

---

#### BUG-GO-08 [HIGH]: Відсутність Docker Volume для збереження завантажених аватарів
- **Файл та рядки:** [docker-compose.yml](file:///e:/Github/oxide_film/server/docker-compose.yml#L40-L71)
- **У чому полягає проблема:**
  Аватари користувачів зберігаються на диск у директорію `./data/uploads/avatars` (всередині контейнера сервісу `app`). Проте у `docker-compose.yml` у секції `volumes` змонтовано лише `pgdata` для PostgreSQL! Для контейнера `app` немає volume mount. При будь-якому перестворенні (`docker-compose down && docker-compose up`) або оновленні образу всі аватари користувачів видаляються назавжди.
- **Спосіб виправлення:**
  Додати том для зберігання завантажень:
  ```yaml
  app:
    volumes:
      - uploads_data:/app/data/uploads
  volumes:
    pgdata:
    uploads_data:
  ```

---

#### BUG-GO-09 [HIGH]: Відсутність інвалідації Redis сесій при зміні/скиданні пароля
- **Файл та рядки:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L313-L356), [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L402-L435)
- **У чому полягає проблема:**
  В API відсутній ендпоінт `POST /auth/logout`. Крім того, коли користувач змінює пароль (`ChangePassword`) або скидає його (`ResetPassword`), старі довготривалі `refresh_token` у Redis не інвалідуються. Зловмисник, який вкрав або перехопив сесію, може продовжувати оновлювати access-токени протягом 30 днів навіть після того, як власник змінив свій пароль.
- **Спосіб виправлення:**
  Зберігати в Redis зв'язку користувача `user:<id>:tokens` та інвалідувати всі активні токени користувача при зміні облікових даних. Додати маршрут `POST /api/v1/auth/logout`.

---

#### BUG-GO-10 [HIGH]: Відсутність підписки на Redis Pub/Sub у хабі Watch Party
- **Файл та рядки:** [hub.go](file:///e:/Github/oxide_film/server/internal/transport/ws/hub.go#L88), [redis.go](file:///e:/Github/oxide_film/server/internal/repository/redis/redis.go#L96)
- **У чому полягає проблема:**
  Метод `PublishWatchPartyEvent` викликається при кожній події у WebSocket, але метод `SubscribeWatchPartyEvents` ніколи не викликається в коді хабу (він присутній лише в тестах). Якщо сервер масштабується на кілька інстансів або контейнерів, повідомлення з Redis Pub/Sub не слухаються жодним екземпляром сервера. Крім того, стан кімнати (`SetWatchPartyState`) ніколи не зберігається в Redis при play/pause/seek.
- **Спосіб виправлення:**
  Підписати горутину хабу на Redis Pub/Sub кімнати для отримання подій від інших інстансів та актуалізувати `WatchPartyState` у Redis.

---

#### BUG-GO-11 [MEDIUM]: Жорстко закодований дефолтний JWT секрет
- **Файл та рядки:** [config.go](file:///e:/Github/oxide_film/server/config/config.go#L47)
- **У чому полягає проблема:**
  ```go
  JWTSecret: getEnv("JWT_SECRET", "super-secret-jwt-key-oxide-film-2026")
  ```
  Якщо адміністратор забуде задати змінну `JWT_SECRET` у бойовому середовищі, сервер мовчки стартує з відомим публічним секретом, що дозволяє підробляти токени довільних користувачів та отримувати права адміністратора.
- **Спосіб виправлення:**
  Якщо `JWT_SECRET` порожній у production-середовищі, викидати `log.Fatal("JWT_SECRET environment variable is required")`.

---

#### BUG-GO-12 [MEDIUM]: Singleflight скасування контексту першого клієнта
- **Файл та рядки:** [registry.go](file:///e:/Github/oxide_film/server/internal/provider/registry.go#L71-L81)
- **У чому полягає проблема:**
  ```go
  func (r *Registry) SingleFlightSearch(ctx context.Context, query string) ([]domain.MediaItem, error) {
      key := fmt.Sprintf("search:%s", query)
      val, err, _ := r.sf.Do(key, func() (interface{}, error) {
          return r.SearchAll(ctx, query), nil
      })
  ```
  `singleflight.Do` використовує `ctx` клієнта, який надіслав запит першим. Якщо перший клієнт розірве з'єднання або натисне "Стоп" у браузері, його `ctx.Done()` спрацює і перерве HTTP-запити до парсерів. Усі інші користувачі, які чекали на цей спільний результат, також отримають помилку `context canceled`.
- **Спосіб виправлення:**
  Передавати у `SearchAll` відокремлений контекст з фіксованим таймаутом (наприклад, `context.WithTimeout(context.Background(), 15*time.Second)`).

---

#### BUG-GO-13 [MEDIUM]: Некоректні коди помилок та неконсистентний Content-Type
- **Файл та рядки:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L91-L94), [content_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/content_handler.go#L27)
- **У чому полягає проблема:**
  1. У `Register` будь-яка помилка створення користувача (включно з відмовою підключення до PostgreSQL або таймаутом) повертає статус `409 Conflict: "email already registered"`. Допоміжна функція `isDuplicateKeyError` у репозиторії написана, але не використовується.
  2. У `content_handler.go` помилки повертаються через `http.Error()`, що примусово виставляє `Content-Type: text/plain; charset=utf-8`, тоді як клієнт очікує єдиний JSON-формат `application/json`.
- **Спосіб виправлення:**
  Перевіряти помилку через `isDuplicateKeyError(err)` і повертати 500 при системних помилках БД. Уніфікувати всі хендлери на використання `jsonError(w, msg, code)`.

---

#### BUG-GO-14 [MEDIUM]: Небезпечний механізм міграцій БД без блокування та обліку
- **Файл та рядки:** [db.go](file:///e:/Github/oxide_film/server/internal/repository/postgres/db.go#L43-L65)
- **У чому полягає проблема:**
  Функція `runMigrations` на кожному старті сервера виконує всі SQL-файли без перевірки таблиці `schema_migrations` та без блокування на рівні БД (`pg_advisory_lock`). При одночасному запуску кількох інстансів або додаванні неідемпотентних інструкцій виникне помилка запуску. Також відсутнє сортування файлів міграцій (`sort.Slice`), що може призвести до запуску міграцій у непередбачуваному порядку на різних ОС.
- **Спосіб виправлення:**
  Використовувати стабільне сортування імен файлів та PostgreSQL Advisory Lock на час виконання міграцій.

---

#### BUG-GO-15 [MEDIUM]: Синхронне блокування HTTP-запитів реєстрації зовнішнім Resend API
- **Файл та рядки:** [auth_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/auth_handler.go#L101), [email.go](file:///e:/Github/oxide_film/server/internal/email/email.go#L133-L144)
- **У чому полягає проблема:**
  Виклик `SendVerificationEmail` виконується синхронно у тілі запиту `Register`. Якщо Resend API має затримки, клієнт чекає до 10 секунд на завершення реєстрації. Також у запиті `http.NewRequest` не передається контекст.
- **Спосіб виправлення:**
  Відправляти листи асинхронно через робочу чергу або окрему горутину, а в `email.go` використовувати `http.NewRequestWithContext`.

---

#### BUG-GO-16 [MEDIUM]: Мертвий код CacheRepository у `ContentHandler`
- **Файл та рядки:** [content_handler.go](file:///e:/Github/oxide_film/server/internal/transport/http/content_handler.go#L12-L22)
- **У чому полягає проблема:**
  Поле `cacheRepo` передається в конструктор `ContentHandler`, але жодного разу не використовується в методах `Search`, `GetDetails` або `GetStreams`. Весь функціонал кешування метаданих фільмів у Postgres є "мертвим" кодом.
- **Спосіб виправлення:**
  Інтегрувати `cacheRepo.Get` та `cacheRepo.Set` перед зверненням до провайдерів.

---

#### BUG-GO-18 [LOW]: Неможливість скинути `bio` та `avatar_url` на порожнє значення
- **Файл та рядки:** [user_repo.go](file:///e:/Github/oxide_film/server/internal/repository/postgres/user_repo.go#L106-L115)
- **У чому полягає проблема:**
  У запиті `UpdateProfile` стоїть умова:
  `bio = CASE WHEN $3 <> '' THEN $3 ELSE bio END`.
  Якщо користувач хоче очистити поле "Про себе" (надіславши порожній рядок `""`), старе значення залишається незмінним.
- **Спосіб виправлення:**
  Приймати в репозиторій покажчики `*string` та перевіряти на `nil`.

---

### Блок 2: Flutter Client (`lib/`)

#### BUG-FL-01 [CRITICAL]: Контрактний розрив регістру полів (camelCase vs snake_case) у Cloud Sync
- **Файл та рядки:** [oxide_server_service.dart](file:///e:/Github/oxide_film/lib/data/services/oxide_server_service.dart#L379-L395), [oxide_server_service.dart](file:///e:/Github/oxide_film/lib/data/services/oxide_server_service.dart#L452-L462)
- **У чому полягає проблема:**
  У `saveHistoryProgress` клієнт відправляє на бекенд JSON з ключами в **camelCase**:
  ```dart
  final payload = {
    'mediaId': mediaId,
    'providerId': providerId,
    'title': title,
    'posterUrl': posterUrl,
    'mediaType': mediaType,
    'positionMs': positionMs,
    'durationMs': durationMs,
    // ...
  };
  ```
  Аналогічно у `toggleFavorite`: передаються `'mediaId'`, `'providerId'`, `'posterUrl'`.
  А на Go-сервері структури `domain.WatchHistory` та `domain.Favorite` мають теги:
  ```go
  MediaID    string `json:"media_id"`
  ProviderID string `json:"provider_id"`
  PosterURL  string `json:"poster_url,omitempty"`
  PositionMs int64  `json:"position_ms"`
  ```
  Стандартний десеріалізатор `json.Unmarshal` у Go **НЕ співставляє `mediaId` з `media_id`**! Усі ці поля залишаються нульовими значеннями (`""` та `0`).
- **Наслідки:**
  1. Усі записи історії записуються в базу з `media_id = ""` та `provider_id = ""`.
  2. Перший же доданий тайтл в обране створює запис з порожнім `media_id`. Спроба додати будь-який інший фільм розцінюється як той самий запис через унікальний індекс `(user_id, media_id, provider_id)`.
  3. Хмарна синхронізація повністю непрацездатна.
- **Спосіб виправлення:**
  Привести JSON-ключі у `OxideServerService` до єдиного формату `snake_case`:

```dart
final payload = {
  'media_id': mediaId,
  'provider_id': providerId,
  'title': title,
  'poster_url': posterUrl,
  'year': year,
  'media_type': mediaType,
  'position_ms': positionMs,
  'duration_ms': durationMs,
  'season': season,
  'episode': episode,
  'episode_title': episodeTitle,
  'last_stream_url': lastStreamUrl,
  'voiceover': voiceover,
  'watched_at': (watchedAt ?? DateTime.now()).toUtc().toIso8601String(),
};
```

---

#### BUG-FL-02 [CRITICAL]: Неможливість видалити фільм із сервера ("Zombie Favorites")
- **Файл та рядки:** [favorites_service.dart](file:///e:/Github/oxide_film/lib/data/services/favorites_service.dart#L329-L334)
- **У чому полягає проблема:**
  Погляньмо на метод синхронізації одиночного елемента `_syncSingleItemToCloud`:
  ```dart
  if (_server.isAuthenticated) {
    if (isFavorite) {
      await _server.toggleFavorite(...);
    }
    return;
  }
  ```
  Коли користувач прибирає фільм з обраного (`isFavorite == false`), код **нічого не робить і просто виходить (`return`)**! На Go-сервері фільм залишається в списку обраного.
  Коли користувач перезапускає додаток, спрацьовує `_pullFromCloud()`:
  ```dart
  final records = await _server.getFavorites();
  for (final cloudData in records) {
    final localExists = await _dao.isFavorite(mediaId, providerId);
    if (!localExists) {
      await _dao.add(...); // Відновлюємо видалений фільм назад!
    }
  }
  ```
- **Наслідки:** Користувач не може видалити жоден фільм з обраного. Будь-який видалений фільм "воскресає" при наступному запуску застосунку.
- **Спосіб виправлення:**
  Реалізувати на бекенді явний метод видалення або передавати статус у `toggleFavorite`. Якщо `!isFavorite`, викликати серверний маршрут видалення:

```dart
if (_server.isAuthenticated) {
  // Викликаємо сервер для синхронізації актуального стану
  await _server.toggleFavorite(
    mediaId: fav.mediaId,
    providerId: fav.providerId,
    title: fav.title,
    mediaType: fav.mediaType,
  );
  return;
}
```

---

#### BUG-FL-03 [HIGH]: Відсутність перехоплювача 401 (Refresh Token Interceptor) у `ApiClient`
- **Файл та рядки:** [api_client.dart](file:///e:/Github/oxide_film/lib/core/network/api_client.dart#L30-L48), [main.dart](file:///e:/Github/oxide_film/lib/main.dart#L79-L98)
- **У чому полягає проблема:**
  JWT Access Token на Go-сервері живе рівно 15 хвилин. `ApiClient` налаштований з `RetryInterceptor` для мережевих збоїв, але **не має жодного інтерцептора для обробки HTTP 401 Unauthorized**.
  Метод `refreshAuth()` викликається **лише один раз при холодному старті додатку** в `main.dart`.
  Якщо користувач дивиться серію тривалістю 45 хвилин, через 15 хвилин його токен прострочується. Наступні періодичні спроби зберегти прогрес перегляду (`saveHistoryProgress`) або додати в обране завершуються помилкою 401 і тихим відхиленням запиту.
- **Спосіб виправлення:**
  Додати `QueuedInterceptor` до Dio екземпляра в `ApiClient`, який при отриманні 401 ставить запити на паузу, викликає `OxideServerService.refreshAuth()`, оновлює заголовок `Authorization: Bearer <new_token>` та повторює оригінальний запит.

---

#### BUG-FL-04 [HIGH]: Втрата тексту помилки бекенду у `ApiClient._handleDioError`
- **Файл та рядки:** [api_client.dart](file:///e:/Github/oxide_film/lib/core/network/api_client.dart#L233-L241)
- **У чому полягає проблема:**
  ```dart
  case DioExceptionType.badResponse:
    final statusCode = e.response?.statusCode ?? 0;
    final statusMessage = e.response?.statusMessage ?? 'Unknown';
    return ServerException(
      message: 'Server error $statusCode: $statusMessage',
      statusCode: statusCode,
      code: 'HTTP_$statusCode',
    );
  ```
  Тіло відповіді `e.response?.data` повністю ігнорується! Коли бекенд повертає змістовне повідомлення, наприклад `{"error":"email already registered"}` або `{"error":"email not verified"}`, `ApiClient` викидає виключення з текстом `"Server error 409: Conflict"`. Функція `_translateError()` в `AuthService` не може знайти ключові слова і показує користувачеві сирий англомовний технічний текст замість підказки українською мовою.
- **Спосіб виправлення:**
  Витягувати поле `error` або `message` з `e.response?.data`:

```dart
case DioExceptionType.badResponse:
  final statusCode = e.response?.statusCode ?? 0;
  String message = e.response?.statusMessage ?? 'Unknown';
  if (e.response?.data is Map && e.response?.data['error'] != null) {
    message = e.response!.data['error'].toString();
  }
  return ServerException(
    message: message,
    statusCode: statusCode,
    code: 'HTTP_$statusCode',
  );
```

---

#### BUG-FL-05 [HIGH]: Ігнорування розриву WebSocket у `WatchPartyService`
- **Файл та рядки:** [watch_party_service.dart](file:///e:/Github/oxide_film/lib/data/services/watch_party_service.dart#L382-L388)
- **У чому полягає проблема:**
  У `_OxideServerBackend.connect` слухач потоку WebSocket:
  ```dart
  onError: (err) {
    Logger.e('WS error: $err', tag: _tag);
  },
  onDone: () {
    Logger.i('WS connection closed', tag: _tag);
  },
  ```
  При розриві сокета або перезавантаженні сервера події `onError` та `onDone` лише пишуть у лог. Вони не викликають `_setError()`, не перемикають стан у `WatchPartyState.error`, не сповіщають віджети (`notifyListeners`) та не ініціюють автоматичне перепідключення або fallback на PeerDart. Користувач залишається на екрані з ілюзією активного перегляду.
- **Спосіб виправлення:**
  Передавати колбек помилки або викликати зміну стану сервісу:

```dart
onDone: () {
  Logger.w('WS connection closed unexpectedly', tag: _tag);
  onErrorCallback?.call('З\'єднання з кімнатою розірвано');
},
```

---

#### BUG-FL-06 [HIGH]: Конфлікт блокування екрана (Wakelock) між `DownloadService` та `PlayerController`
- **Файл та рядки:** [download_service.dart](file:///e:/Github/oxide_film/lib/data/services/download_service.dart#L121-L127), [player_controller.dart](file:///e:/Github/oxide_film/lib/presentation/pages/player/player_controller.dart#L538)
- **У чому полягає проблема:**
  У `DownloadService`:
  ```dart
  void _updateActiveTasks(int id, DownloadTask? task) {
    // ...
    if (_activeTasks.isNotEmpty) {
      WakelockPlus.enable();
    } else {
      WakelockPlus.disable(); // ВИМИКАЄ БЛОКУВАННЯ ГЛОБАЛЬНО!
    }
  }
  ```
  Бібліотека `WakelockPlus` керує глобальним системним прапорцем операційної системи. Якщо користувач дивиться фільм у плеєрі і водночас у фоні завершується завантаження відеофайлу, `DownloadService` викликає `WakelockPlus.disable()`. Як наслідок, екран пристрою гасне посеред перегляду фільму!
- **Спосіб виправлення:**
  Використовувати підрахунок посилань (Reference Counting) або перевіряти, чи не активний зараз відеоплеєр:
  ```dart
  if (!GetIt.I<VideoPlayerService>().isPlaying) {
    WakelockPlus.disable();
  }
  ```

---

#### BUG-FL-07 [MEDIUM]: Відсутність методу `dispose()` у `WatchPartyService`
- **Файл та рядки:** [watch_party_service.dart](file:///e:/Github/oxide_film/lib/data/services/watch_party_service.dart#L740-L755), [watch_party_service.dart](file:///e:/Github/oxide_film/lib/data/services/watch_party_service.dart#L1270-L1275)
- **У чому полягає проблема:**
  Клас `WatchPartyService extends ChangeNotifier` запускає періодичний таймер `_heartbeatTimer = Timer.periodic(const Duration(seconds: 2), ...)` та відкриває з'єднання бекендів. При цьому в класі взагалі **відсутнє перевизначення методу `dispose()`**. Якщо сервіс перестворюється (наприклад, у тестах або при зміні конфігурації), таймер продовжує тікати у фоні, утримуючи весь сервіс у пам'яті.
- **Спосіб виправлення:**
  Реалізувати `dispose()` із скасуванням таймерів та відключенням бекенду:

```dart
@override
void dispose() {
  _stopHeartbeat();
  _backend?.disconnect();
  super.dispose();
}
```

---

#### BUG-FL-08 [MEDIUM]: Небезпечний виклик `_setupControllerCallbacks` після асинхронного `open()` у `PlayerPage`
- **Файл та рядки:** [player_page.dart](file:///e:/Github/oxide_film/lib/presentation/pages/player/player_page.dart#L80-L111)
- **У чому полягає проблема:**
  У `_PlayerPageState._initPlayer`:
  ```dart
  await _videoPlayerService.open(...);
  if (_videoPlayerService.controller != null) {
    _setupControllerCallbacks(_videoPlayerService.controller!);
  }
  ```
  Функція `_initPlayer` асинхронна і викликається в `initState()` без очікування. Якщо користувач швидко відкрив і закрив екран плеєра до завершення `_videoPlayerService.open()`, `PlayerPage.dispose()` виконується раніше. Після повернення з `await` віджет вже не змонтований (`mounted == false`), але `_setupControllerCallbacks` навішує слухачі на `watchPartyService`, колбеки яких викликають `setState()` на знищеному віджеті (`setState() called after dispose()`).
- **Спосіб виправлення:**
  Перевіряти `if (!mounted) return;` відразу після `await _videoPlayerService.open(...)`.

---

#### BUG-FL-09 [MEDIUM]: Помилка видалення лісенера у `VideoPlayerService.close()`
- **Файл та рядки:** [video_player_service.dart](file:///e:/Github/oxide_film/lib/data/services/video_player_service.dart#L70), [video_player_service.dart](file:///e:/Github/oxide_film/lib/data/services/video_player_service.dart#L133)
- **У чому полягає проблема:**
  У методі `open()` додається лісенер:
  `_controller!.addListener(_safeNotifyListeners);`
  А в методі `close()` видаляється:
  `_controller!.removeListener(notifyListeners);`
  Функція `_safeNotifyListeners` є окремим методом класу і не дорівнює `notifyListeners`! Через це `removeListener` не знаходить обробник, він залишається в пам'яті контролера, а при виклику `_controller.dispose()` генерується попередження або витік пам'яті.
- **Спосіб виправлення:**
  Викликати `_controller!.removeListener(_safeNotifyListeners)`.

---

#### BUG-FL-10 [MEDIUM]: `HistoryDao.saveProgress` ігнорує переданий `watchedAt` при нових записах
- **Файл та рядки:** [history_dao.dart](file:///e:/Github/oxide_film/lib/data/database/dao/history_dao.dart#L134)
- **У чому полягає проблема:**
  У параметрах методу є `DateTime? watchedAt` для коректної синхронізації з хмари. При оновленні запису використовується `Value(watchedAt ?? DateTime.now())`. Проте в гілці створення нового запису (`insert`) жорстко прописано:
  `watchedAt: Value(DateTime.now())`.
  Якщо клієнт завантажує з хмари історію трирічної давнини, локально запис створюється з поточною датою і часом, викривляючи реальний порядок переглядів користувача.
- **Спосіб виправлення:**
  Замінити рядок 134 на:
  `watchedAt: Value(watchedAt ?? DateTime.now())`.

---

#### BUG-FL-11 [MEDIUM]: `EpisodeUpdateService` втрачає стан відомих епізодів при перезапуску
- **Файл та рядки:** [episode_update_service.dart](file:///e:/Github/oxide_film/lib/data/services/episode_update_service.dart#L52), [episode_update_service.dart](file:///e:/Github/oxide_film/lib/data/services/episode_update_service.dart#L123-L152)
- **У чому полягає проблема:**
  Колекція `final Map<String, int> _lastKnownEpisodes = {};` існує виключно в оперативній пам'яті. При кожному закритті та повторному відкритті застосунку вона порожня. Під час першої перевірки `lastKnown == null`, сервіс лише заповнює карту і не виявляє нові епізоди. Таким чином, якщо додаток не тримати відкритим цілодобово, користувач ніколи не отримає сповіщення про вихід нових серій.
- **Спосіб виправлення:**
  Зберігати стан кількості серій у SharedPreferences або в таблиці налаштувань Drift SQLite.

---

#### BUG-FL-12 [LOW]: Невивільнений `ValueNotifier<Duration>` у `PlayerController.dispose()`
- **Файл та рядки:** [player_controller.dart](file:///e:/Github/oxide_film/lib/presentation/pages/player/player_controller.dart#L207), [player_controller.dart](file:///e:/Github/oxide_film/lib/presentation/pages/player/player_controller.dart#L1114-L1161)
- **У чому полягає проблема:**
  Поле `final ValueNotifier<Duration> positionNotifier = ValueNotifier(Duration.zero);` ніколи не закривається в `PlayerController.dispose()`.
- **Спосіб виправлення:**
  Додати `positionNotifier.dispose()` перед викликом `super.dispose()`.

---

#### BUG-FL-13 [LOW]: Витоки `TextEditingController` у модальних діалогах
- **Файл та рядки:** [login_page.dart](file:///e:/Github/oxide_film/lib/presentation/pages/auth/login_page.dart#L489), [profile_page.dart](file:///e:/Github/oxide_film/lib/presentation/pages/auth/profile_page.dart#L179-L181)
- **У чому полягає проблема:**
  У методах `_showForgotPasswordDialog` та `_showChangePasswordDialog` локально створюються контролери текстових полів (`TextEditingController()`), які передаються у віджети діалогів і ніколи не утилізуються через `.dispose()`.
- **Спосіб виправлення:**
  Обернути використання контролерів у StatefulWidget або викликати `dispose()` після закриття діалогу в блоці `finally`.

---

#### BUG-FL-14 [LOW]: Некеровані фонові запити при скасуванні підписки на `_streamProviders`
- **Файл та рядки:** [unified_content_repository_impl.dart](file:///e:/Github/oxide_film/lib/data/repositories/unified_content_repository_impl.dart#L394-L437)
- **У чому полягає проблема:**
  Створений `StreamController<List<MediaItem>>` не має обробника `onCancel`. Якщо користувач скасовує пошук або вводить новий запит, попередні паралельні мережеві запити до парсерів продовжують виконуватися і навантажувати мережу та процесор.
- **Спосіб виправлення:**
  Додати механізм скасування (наприклад, `CancelToken` у парсери) та прив'язати його до `controller.onCancel`.

---

## 4. План першочергових дій з виправлення (Remediation Roadmap)

1. **Фаза 1: Безпека та Автентифікація (Терміново):**
   - Виправити `GoogleAuth` у `auth_handler.go`: додати перевірку `aud` та `email_verified`.
   - Обмежити CORS у `router.go` білим списком доменів.
   - Закрити Directory Listing для `/uploads/` та додати валідацію Magic Bytes у `UploadAvatar`.
   - Додати том `uploads_data` у `docker-compose.yml`.

2. **Фаза 2: Стабільність WebSocket Hub (Терміново):**
   - Усунути deadlock у `hub.Run()`: заборонити рекурсивний запис у `h.broadcast` із самої горутини хабу.
   - Захистити `readPump` від блокування при зупинці сервера (`stopChan`).
   - Усунути конкурентний запис у сокет при `GracefulStop`.

3. **Фаза 3: Клієнт-серверні контракти та Синхронізація (Високий пріоритет):**
   - Перевести всі ключі в `OxideServerService` (`saveHistoryProgress`, `toggleFavorite`) у `snake_case`.
   - Виправити логіку `_syncSingleItemToCloud` у `FavoritesService`, щоб видалення фільму викликало сервер.
   - Додати в `ApiClient` обробку HTTP 401 з автоматичною ротацією токенів через `refreshAuth()`.
   - Зберегти вихідне повідомлення помилки бекенду в `_handleDioError`.

4. **Фаза 4: Очищення ресурсів та виправлення дрібних дефектів:**
   - Додати `dispose()` у `WatchPartyService`.
   - Виправити видалення слухача у `VideoPlayerService.close()`.
   - Зберігати стан `EpisodeUpdateService` у SharedPreferences.
   - Усунути конфлікт `WakelockPlus` між завантажувачем та відеоплеєром.

---

## 5. Резюме та висновок

Проведений незалежний аудит виявив **3 критичні вразливості та архітектурні дефекти на Go-сервері** та **2 критичні баги у Flutter-клієнті**, разом із 8 проблемами високого рівня важливості. 

Більшість виявлених дефектів мають точкову природу та піддаються швидкому виправленню без кардинального переписування архітектури проєкту. Виконання наданих у цьому звіті рекомендацій дозволить забезпечити повну безпеку автентифікації користувачів, гарантувати стабільність WebSocket хабу під навантаженням та відновити повноцінну хмарну синхронізацію між усіма пристроями платформи Oxide Film.
