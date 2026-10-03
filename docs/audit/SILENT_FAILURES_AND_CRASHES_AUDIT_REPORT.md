> ## Статус документа
>
> - **Status:** superseded by `docs/REMEDIATION_PLAN.md`
> - **Verified against:** `3ab45ef`
> - **Актуальність:** аудит «тихих» помилок, зависань і падінь у клієнті та бекенді.
>   Знахідки перенесені в `docs/REMEDIATION_PLAN.md` (Wave 2H, Wave 3J — Dart).
> - **Вибірково перевірено на `3ab45ef`:** 401-interceptor з refresh-and-replay замість
>   миттєвого `signOut()` та in-flight guard для `_saveProgress`
>   (`lib/core/network/api_client.dart`, `lib/data/services/history_service.dart`).
> - **Не переписано:** історичні знахідки залишено як є.
# Комплексний аудит тихих помилок, зависань та аварійних завершень (Silent Failures, Freezes & Crashes Audit Report)

**Проєкт:** Oxide Film (Flutter Client + Go Backend)  
**Дата аудиту:** 28 вересня 2026 року  
**Роль:** Site Reliability & Code Quality Auditor  
**Статус:** Завершено  

---

## 1. Резюме та матриця ризиків

В ході глибокого аудиту кодової бази **Oxide Film** (клієнтська частина у `lib/` та серверна частина у `server/`) було проведено комплексне сканування на наявність дефектів двох фундаментальних категорій:
1. **Тихі помилки (Silent Errors / Swallowed Exceptions):** ковтання винятків блоками `catch (_) {}`, ігнорування помилок операцій вводу-виводу/БД (`_ = ...`), відсутність обробки статус-кодів upstream-провайдерів (Cloudflare 403/502), через що користувач бачить порожній екран замість інформативної причини збою.
2. **Падіння без повідомлень, зависання та дедлоки (Silent Crashes, Freezes & Deadlocks):** незахищені фонові горутини в Go без `recover()` (що призводять до миттєвого завершення процесу всієї програми), блокування небуферизованих каналів при завершенні роботи, вічні спінери (`isLoading = true`) через відсутність блоків `finally`, безкінечні зависання мережевих викликів без таймаутів та вічна буферизація у відеоплеєрі.

### Зведена таблиця знайдених проблем

| ID | Компонент | Рівень | Назва дефекту | Наслідок для системи |
|---|---|---|---|---|
| **CRIT-01** | `server/internal/provider/registry.go`, `server/cmd/api/main.go` | **Критичний (P0)** | Goroutine Panics Without Recover у фонових воркерах і пошуку | Повне падіння процесу сервера при будь-якій помилці стороннього провайдера |
| **CRIT-02** | `server/internal/transport/ws/hub.go`, `main.go` | **Критичний (P0)** | Дедлок на небуферизованому каналі `register` при зупинці сервера | Зависання HTTP-хендлерів та блокування процесу Graceful Shutdown |
| **CRIT-03** | `lib/presentation/pages/player/player_controller.dart` | **Критичний (P0)** | Вічна буферизація плеєра при помилці потоку (`position > Duration.zero`) | Зависання інтерфейсу користувача у вічному спінері без відображення помилки |
| **CRIT-04** | `lib/data/services/watch_party_service.dart` | **Критичний (P0)** | Відсутність таймауту ініціалізації брокера PeerDart / WebSocket | Безкінечний спінер створення або підключення до кімнати Watch Party |
| **HIGH-01** | `server/internal/provider/client.go` | **Високий (P1)** | Ігнорування HTTP StatusCode у TLSClient (Cloudflare 403/502/429) | Маскування блокувань та помилок серверів під порожній результат пошуку |
| **HIGH-02** | `server/internal/transport/http/sync_handler.go` | **Високий (P1)** | Ігнорування помилок бази даних при зміні статусу обраного (ToggleFavorite) | Розсинхронізація клієнта: успішна відповідь 200 OK при фейлі запису в БД |
| **HIGH-03** | `lib/presentation/pages/details/details_page.dart` | **Високий (P1)** | Вічне зависання вибору серії при відсутності провайдера | Елемент вибору серії блокується у стані `_isLoadingEpisode = true` назавжди |
| **HIGH-04** | `lib/data/services/search_service.dart`, `search_page.dart` | **Високий (P1)** | Послідовне очікування результатів у стрімі пошуку та незавершуваний прогрес | Повільні провайдери блокують швидких; вічний індикатор пошуку |
| **HIGH-05** | `lib/presentation/pages/home/home_page.dart` | **Високий (P1)** | Ковтання помилок усіх провайдерів на головному екрані | Каталог відображає "Немає контенту" замість повідомлення про відсутність мережі |
| **MED-01** | `lib/data/services/sync_service.dart` | **Середній (P2)** | Ковтання помилок схеми/типів при імпорті бекапу | Успішний статус імпорту при реальній втраті даних користувача |
| **MED-02** | `server/internal/repository/postgres/user_repo.go` | **Середній (P2)** | Ігнорування помилок видалення використаних токенів скидання пароля | Можливість повторного використання токенів скидання пароля |
| **MED-03** | `lib/data/services/update_service.dart` | **Середній (P2)** | Відсутність таймаутів з'єднання у клієнті оновлень Dio | Зависання діалогу перевірки та завантаження оновлень |
| **MED-04** | `server/internal/transport/http/auth_handler.go` | **Середній (P2)** | Відсутність очищення тимчасових файлів при завантаженні аватарів | Потенційний витік дискового простору при обробці великих завантажень |
| **MED-05** | `lib/data/services/oxide_server_service.dart` | **Середній (P2)** | Відсутність таймауту на виклику `WebSocket.connect` | Зависання клієнта при недоступності вебсокет-сервера |
| **LOW-01** | `lib/presentation/pages/auth/profile_page.dart`, `login_page.dart` | **Низький (P3)** | Відсутність блоків finally та небезпечне розіменування полів помилок | Текст "Помилка: null" та потенційне залипання індикатора прогресу |
| **LOW-02** | `lib/presentation/pages/player/player_controls.dart` | **Низький (P3)** | Використання `try-catch` як штатного flow-control у селекторах | Неохайне перехоплення винятків замість безпечних методів колекцій |
| **LOW-03** | `lib/presentation/pages/provider/provider_page.dart` | **Низький (P3)** | Падіння пагінації без інформування користувача | Раптове припинення довантаження списку без повідомлень у UI |

---

## 2. Критичні дефекти (Critical / Severity P0)

### CRIT-01: Go Goroutine Panics Without Recover у фонових воркерах та паралельному пошуку

- **Файли та рядки:**
  - `server/internal/provider/registry.go:56-65`
  - `server/cmd/api/main.go:51-65`
  - `server/internal/transport/ws/hub.go:54-122`, `178-180`
- **Сценарій виникнення:**
  1. Користувач здійснює пошуковий запит через `SingleFlightSearch` / `SearchAll`. Сервер запускає паралельні горутини для кожного провайдера: `go func(prov domain.Provider)`.
  2. Якщо сторонній сайт змінив розмітку або повернув неочікуваний контент, і DOM-парсер (або маніпуляція зі зрізами/стрінгами) викликає `runtime error: index out of range` або `nil pointer dereference`.
  3. Оскільки горутина створена вручну через `go func()`, стандартний Gin middleware `gin.Recovery()` **НЕ МОЖЕ** її перехопити (в Go паніка в іншій горутині, не обгорнутій `recover()`, негайно кладе весь OS-процес).
  4. Аналогічно: фоновий кеш-воркер у `main.go:51` та воркери `writePump` / `readPump` у `hub.go` не мають захисту від панік.
- **Чому це критично:**
  Один некоректний рядок HTML від будь-якого стороннього піратського онлайн-кінотеатру повністю припиняє роботу бекенда для всіх активних користувачів сервісу (процес падає з кодом 2, перериваючи синхронізацію, аутентифікацію та Watch Party).
- **Код для виправлення:**

```go
// Файл: server/internal/provider/registry.go
func (r *Registry) SearchAll(ctx context.Context, query string) []domain.MediaItem {
	providers := r.List()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var aggregated []domain.MediaItem

	for _, p := range providers {
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[PANIC RECOVER] Provider %s crashed on query %q: %v", prov.Name(), query, rec)
				}
			}()

			items, err := prov.Search(ctx, query)
			if err != nil {
				log.Printf("[Provider Error] %s search failed: %v", prov.Name(), err)
				return
			}
			if len(items) > 0 {
				mu.Lock()
				aggregated = append(aggregated, items...)
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()
	return aggregated
}
```

---

### CRIT-02: Дедлок на небуферизованому каналі `register` при зупинці сервера

- **Файли та рядки:**
  - `server/internal/transport/ws/hub.go:48-49`, `176-177`
  - `server/cmd/api/main.go:121-127`
- **Сценарій виникнення:**
  1. Під час штатного завершення роботи сервера (`SIGTERM`/`SIGINT`), `main.go` викликає:
     ```go
     wsHub.GracefulStop()
     ```
  2. `GracefulStop()` закриває `stopChan`, і цикл `h.Run()` завершується (`return`).
  3. Тільки **після** цього викликається `server.Shutdown(shutdownCtx)`.
  4. У проміжку між виходом з `h.Run()` та зупинкою HTTP-лістенера надходить новий запит на підключення клієнта до WebSocket (`/api/v1/ws/watch-party`).
  5. Викликається `h.HandleWebSocket`, де виконується:
     ```go
     h.register <- client
     ```
  6. Оскільки канал `register` є **небуферизованим** (`make(chan *Client)`), а горутина-читач `h.Run()` вже мертва, цей запис блокує горутину HTTP-обробника **назавжди**.
  7. `server.Shutdown` не може коректно завершитися до закінчення 10-секундного таймауту і аварійно примусово закриває з'єднання.
- **Чому це критично:**
  Викликає дедлок горутини, зависання клієнтів при рестарті та неможливість швидкого Graceful Shutdown у контейнерах (Kubernetes/Docker вимушений посилати `SIGKILL`).
- **Код для виправлення:**

```go
// Файл: server/internal/transport/ws/hub.go
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	roomCode := r.URL.Query().Get("room")
	userID := r.URL.Query().Get("user_id")
	userName := r.URL.Query().Get("user_name")

	if roomCode == "" || userID == "" {
		http.Error(w, "missing room or user_id query parameters", http.StatusBadRequest)
		return
	}
	if userName == "" {
		userName = "Гість"
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		roomCode: roomCode,
		userID:   userID,
		userName: userName,
	}

	// Безпечна відправка з перевіркою контексту запиту та каналу зупинки хабу
	select {
	case h.register <- client:
		go client.writePump()
		go client.readPump()
	case <-h.stopChan:
		conn.Close()
		return
	case <-r.Context().Done():
		conn.Close()
		return
	}
}
```

Також змінити порядок у `server/cmd/api/main.go`: спочатку `server.Shutdown(shutdownCtx)`, щоб припинити прийом нових HTTP/WS запитів, і лише потім `wsHub.GracefulStop()`.

---

### CRIT-03: Вічна буферизація плеєра при помилці потоку (`position > Duration.zero`)

- **Файли та рядки:**
  - `lib/presentation/pages/player/player_controller.dart:651-665`
- **Сценарій виникнення:**
  1. Користувач дивиться фільм. На 15-й хвилині відео потік обривається (прострочився тимчасовий токен CDN, провайдер розірвав з'єднання або виникла помилка декодування mpv).
  2. Відео перестає відтворюватися, плеєр переходить у внутрішній стан буферизації (`isBuffering: true`), і через потік `_player.stream.error` надходить повідомлення про помилку від плеєра MPV.
  3. Контролер перевіряє умову:
     ```dart
     if (!_player.state.playing && _state.position == Duration.zero) {
       _state = _state.copyWith(
         hasError: true,
         errorMessage: error,
         isBuffering: false,
       );
       notifyListeners();
     }
     ```
  4. Оскільки `_state.position` дорівнює 15 хвилинам (а **не** `Duration.zero`), блок коду **ігнорується**.
  5. Помилка безслідно ковтається. Стан `hasError` залишається `false`, а `isBuffering` залишається `true`.
- **Чому це критично:**
  На екрані користувача з'являється нескінченний спінер завантаження. Відео більше не відтворюється, інтерфейс не пропонує перемкнути джерело або повторити спробу (`Retry`), користувач вважає, що додаток "намертво завис".
- **Код для виправлення:**

```dart
// Файл: lib/presentation/pages/player/player_controller.dart
_subscriptions.add(
  _player.stream.error.listen((error) {
    if (_isDisposed) return;
    if (error.isNotEmpty) {
      Logger.w('Player stream error encountered: $error', tag: _tag);
      // Помилка повинна відображатися незалежно від того, на якій хвилині виник збій
      _state = _state.copyWith(
        hasError: true,
        errorMessage: error,
        isBuffering: false,
      );
      notifyListeners();
    }
  }),
);
```

---

### CRIT-04: Відсутність таймауту ініціалізації брокера PeerDart / WebSocket у WatchParty

- **Файли та рядки:**
  - `lib/data/services/watch_party_service.dart:460-484`, `512-514`
- **Сценарій виникнення:**
  1. Користувач намагається створити спільну кімнату або підключитися як гість через P2P бекенд PeerDart.
  2. Створюється `Completer<void>()` для очікування підключення до сигнального сервера PeerJS:
     ```dart
     _peer!.on('open').listen((id) { if (!completer.isCompleted) completer.complete(); });
     ```
  3. У коді для хоста (рядок 513) та клієнта (рядок 483):
     ```dart
     await completer.future;
     ```
  4. Якщо публічний сигнальний сервер (peerjs.com) заблокований фаєрволом, не відповідає або дропнув TLS handshake без TCP Reset, `completer.future` **ніколи не завершується**, оскільки для нього взагалі не передбачено таймауту (таймаут був заданий лише для подальшого з'єднання клієнт-хост).
- **Чому це критично:**
  Виклик `hostRoom()` або `joinRoom()` зависає назавжди в стані `WatchPartyState.hosting` / `WatchPartyState.joining`. Вікно діалогу або екран залишається у вічному індикаторі прогресу, а користувач не може ні скасувати дію, ні побачити помилку підключення.
- **Код для виправлення:**

```dart
// Файл: lib/data/services/watch_party_service.dart
// Обмежуємо очікування відкриття брокера жорстким таймаутом
try {
  await completer.future.timeout(
    const Duration(seconds: 10),
    onTimeout: () {
      throw TimeoutException('Не вдалося з\'єднатися з сигнальним сервером PeerDart');
    },
  );
} catch (e) {
  Logger.e('PeerDart signaling handshake timeout/failed', tag: _tag, error: e);
  _peer?.dispose();
  _peer = null;
  rethrow;
}
```

---

## 3. Дефекти високого рівня ризику (High / Severity P1)

### HIGH-01: Ігнорування HTTP StatusCode у TLSClient (Cloudflare 403/502/429)

- **Файли та рядки:**
  - `server/internal/provider/client.go:49-60`, `77-88`
  - `server/internal/provider/uakino.go:40-86`
  - `server/internal/provider/eneyida.go:38-75`
- **Сценарій виникнення:**
  1. `TLSClient.Get()` та `TLSClient.PostForm()` виконують запит до провайдера.
  2. Якщо провайдер повертає `403 Forbidden` (капча/блокування Cloudflare), `502 Bad Gateway` або `429 Too Many Requests`, клієнт **НЕ перевіряє** `resp.StatusCode`.
  3. Клієнт повертає вміст сторінки помилки Cloudflare у вигляді рядка з `err == nil`.
  4. Парсер (наприклад, `goquery.NewDocumentFromReader`) парсить цей HTML. Селектори `.movie-item`, `.short-story` не знаходять жодного збігу.
  5. Метод `Search()` повертає порожній зріз `[]domain.MediaItem` з `err == nil`.
- **Чому це "тиха помилка":**
  Клієнт і сервер вважають, що запит виконано успішно, і що фільм просто "відсутній у каталозі". Адміністратор не бачить блокування провайдера, а користувач отримує хибне враження, що контенту немає, хоча сервіс насправді заблокований Cloudflare або лежить.
- **Код для виправлення:**

```go
// Файл: server/internal/provider/client.go
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
	bodySnippet := string(bodyBytes)
	if len(bodySnippet) > 200 {
		bodySnippet = bodySnippet[:200]
	}
	return "", fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, bodySnippet)
}
```

---

### HIGH-02: Ігнорування помилок бази даних при зміні статусу обраного (ToggleFavorite)

- **Файли та рядки:**
  - `server/internal/transport/http/sync_handler.go:270-278`
- **Сценарій виникнення:**
  1. Клієнт надсилає запит `POST /api/v1/sync/favorites/toggle`.
  2. Хендлер перевіряє статус і викликає:
     ```go
     if isFav {
         _ = h.favoritesRepo.RemoveFavorite(r.Context(), userID, fav.MediaID, fav.ProviderID)
         _, _ = w.Write([]byte(`{"is_favorite":false}`))
     } else {
         _ = h.favoritesRepo.AddFavorite(r.Context(), &fav)
         _, _ = w.Write([]byte(`{"is_favorite":true}`))
     }
     ```
  3. Якщо операція `AddFavorite` або `RemoveFavorite` падає з помилкою (розрив з'єднання з PostgreSQL, конфлікт блокувань транзакцій, тайм-аут запиту), оператор `_ =` повністю ігнорує її.
  4. Сервер повертає клієнту `HTTP 200 OK` з протилежним булевим статусом.
- **Чому це "тиха помилка":**
  Клієнт перемикає іконку сердечка в активний стан і показує снекбар "Додано до обраного". Проте в базі даних запис **не збережено**. Після перезапуску додатку фільм зникає з обраного. Користувач стикається з незрозумілою втратою даних.
- **Код для виправлення:**

```go
// Файл: server/internal/transport/http/sync_handler.go
if isFav {
	if err := h.favoritesRepo.RemoveFavorite(r.Context(), userID, fav.MediaID, fav.ProviderID); err != nil {
		http.Error(w, `{"error":"failed to remove favorite"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"is_favorite":false}`))
} else {
	if err := h.favoritesRepo.AddFavorite(r.Context(), &fav); err != nil {
		http.Error(w, `{"error":"failed to add favorite"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"is_favorite":true}`))
}
```

---

### HIGH-03: Вічне зависання вибору серії при відсутності провайдера у DetailsPage

- **Файли та рядки:**
  - `lib/presentation/pages/details/details_page.dart:1245-1256`
- **Сценарій виникнення:**
  1. Користувач переглядає сторінку серіалу та натискає на конкретну серію.
  2. Метод `_selectEpisode` встановлює `_isLoadingEpisode = true` і очищує поточні стріми:
     ```dart
     setState(() {
       _selectedSeason = season;
       _selectedEpisode = episode;
       _isLoadingEpisode = true;
       _streams = [];
     });
     ```
  3. Далі запитується провайдер:
     ```dart
     final provider = _registry.getById(widget.providerId);
     if (provider == null) return;
     ```
  4. Якщо провайдер з якоїсь причини відключений у налаштуваннях або повернув `null`, виконується `return;`.
- **Чому це "зависання":**
  Прапорець `_isLoadingEpisode` так і залишається рівним `true`. Плитка серії перетворюється на нескінченний круглий спінер `CircularProgressIndicator`. Оскільки на плитці стоїть перевірка `onTap: isLoading ? null : ...`, користувач більше не може натиснути на жодну іншу серію чи повторити дію.
- **Код для виправлення:**

```dart
// Файл: lib/presentation/pages/details/details_page.dart
Future<void> _selectEpisode(int season, int episode) async {
  setState(() {
    _selectedSeason = season;
    _selectedEpisode = episode;
    _isLoadingEpisode = true;
    _streams = [];
  });

  try {
    final provider = _registry.getById(widget.providerId);
    if (provider == null) {
      throw Exception('Провайдер ${widget.providerId} не активний');
    }
    // ... завантаження потоків ...
  } catch (e) {
    debugPrint('Failed to load episode streams: $e');
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Помилка завантаження: $e')),
      );
    }
  } finally {
    if (mounted) {
      setState(() => _isLoadingEpisode = false);
    }
  }
}
```

---

### HIGH-04: Послідовне очікування результатів у стрімі пошуку та незавершуваний прогрес

- **Файли та рядки:**
  - `lib/data/services/search_service.dart:196-210`
  - `lib/presentation/pages/search/search_page.dart:224`
- **Сценарій виникнення:**
  1. `SearchService.searchStream()` ініціалізує список паралельних ф'ючерсів `futures` для кожного провайдера.
  2. Далі в циклі виконується:
     ```dart
     for (final future in futures) {
       try {
         final result = await future;
         results.add(result);
         yield AggregatedSearchResult(
           ...
           isComplete: results.length == providers.length,
         );
       } catch (e) { ... }
     }
     ```
  3. Цикл очікує ф'ючерси не по мірі їх готовності, а в жорсткому порядку індексів списку `providers`. Якщо перший провайдер завис на 15 секунд, результати решти провайдерів, які відповіли за 300 мс, не надсилаються в UI до закінчення таймауту першого!
  4. Якщо один із провайдерів завершується помилкою через виняток, `results.add(result)` пропускається, тому умова `results.length == providers.length` **НІКОЛИ не виконується**.
  5. У `search_page.dart`:
     ```dart
     _isLoading = !result.aggregatedResult.isComplete;
     ```
     Індикатор завантаження пошуку залишається активним навіть після завершення обробки всіх провайдерів.
- **Код для виправлення:**
  Використовувати підписку на кожен Future через StreamController або лічильник завершених операцій `completedCount`, де `isComplete = completedCount == providers.length`.

---

### HIGH-05: Ковтання помилок усіх провайдерів на головному екрані

- **Файли та рядки:**
  - `lib/presentation/pages/home/home_page.dart:129-170`
- **Сценарій виникнення:**
  1. При завантаженні головного екрана запускається `Future.wait`:
     ```dart
     final results = await Future.wait(
       providers.map((provider) async {
         try {
           return await provider.getPopular(type: selectedType, page: 1);
         } catch (e) {
           debugPrint('Failed to load from ${provider.name}: $e');
           return <MediaItem>[];
         }
       }),
     );
     ```
  2. Якщо відсутній інтернет або всі сервери провайдерів недоступні, кожен виклик перехоплюється блоком `catch` і повертає порожній список `[]`.
  3. Після циклу `_error` залишається `null`, `_allItems` порожній.
  4. Інтерфейс відмальовує `_filteredItems.isEmpty` -> "Немає контенту. Спробуйте пошукати щось".
- **Чому це "тиха помилка":**
  Користувач бачить повідомлення про порожній каталог, а не про мережеву помилку. Кнопка "Спробувати знову" відсутня, виникає оманливе враження, що сервер видалив весь каталог.
- **Код для виправлення:**
  Зберігати помилки провайдерів і, якщо `allItems.isEmpty` та всі провайдери кинули винятки, виставляти `_error = 'Не вдалося завантажити контент. Перевірте з\'єднання.'`.

---

## 4. Дефекти середнього рівня ризику (Medium / Severity P2)

### MED-01: Ковтання помилок схеми/типів при імпорті бекапу в SyncService

- **Файли та рядки:**
  - `lib/data/services/sync_service.dart:278-314`
- **Опис проблеми:**
  У методах `_importFavorite` та `_importHistory` стоїть блок `catch (e)` з коментарем `// Ignore duplicate errors`. Проте цей блок перехоплює не лише помилки дублікатів первинного ключа, а й будь-які `TypeError` (наприклад, якщо поле `title` у JSON має тип `null` або числовий тип). У результаті запис не імпортується, а метод повертає користувачу `importData() -> true` ("Успішно імпортовано").
- **Рекомендація щодо виправлення:**
  Перевіряти наявність та тип обов'язкових полів перед викликом DAO і логувати помилки парсингу конкретних сутностей окремо від дублікатів.

---

### MED-02: Ігнорування помилок видалення токенів відновлення пароля та верифікації

- **Файли та рядки:**
  - `server/internal/repository/postgres/user_repo.go:65, 73, 144, 176`
- **Опис проблеми:**
  Виклики `_, _ = r.pool.Exec(ctx, "DELETE FROM password_resets WHERE user_id = $1", userID)` ігнорують помилки виконання SQL. Якщо через збій у базі запис не видалився, метод `MarkPasswordResetUsed` повертає `nil` (успіх), залишаючи токен валідним до закінчення його 1-годинного TTL.
- **Рекомендація щодо виправлення:**
  Перевіряти повернений `err` і повертати помилку або логувати попередження у security audit log.

---

### MED-03: Відсутність таймаутів з'єднання у клієнті оновлень UpdateService

- **Файли та рядки:**
  - `lib/data/services/update_service.dart:22`, `167-172`
- **Опис проблеми:**
  Інстанс `_dio` створюється конструктором за замовчуванням без `connectTimeout` та `sendTimeout`. При завантаженні оновлення встановлено лише `receiveTimeout: Duration(minutes: 10)`. Якщо TCP SYN-пакет потрапляє в "чорну діру" або проксі зависає на встановленні з'єднання, процес оновлення залипає безкінечно.
- **Рекомендація щодо виправлення:**
  Явно конфігурувати `BaseOptions(connectTimeout: Duration(seconds: 15), sendTimeout: Duration(seconds: 15))`.

---

### MED-04: Відсутність очищення тимчасових файлів при завантаженні аватарів у Go

- **Файли та рядки:**
  - `server/internal/transport/http/auth_handler.go:450`
- **Опис проблеми:**
  Виклик `r.ParseMultipartForm(5 << 20)` створює тимчасові файли у системній директорії `/tmp`, якщо файл перевищує поріг пам'яті. У коді відсутній обов'язковий виклик `defer r.MultipartForm.RemoveAll()`, що спричиняє поступове засмічення диску сервера при частих завантаженнях файлів.
- **Рекомендація щодо виправлення:**
  Додати `defer func() { if r.MultipartForm != nil { _ = r.MultipartForm.RemoveAll() } }()` одразу після парсингу.

---

### MED-05: Відсутність таймауту на WebSocket.connect у OxideServerService

- **Файли та рядки:**
  - `lib/data/services/oxide_server_service.dart:506`
- **Опис проблеми:**
  Метод `WebSocket.connect(uri.toString())` викликається без таймауту. Якщо сервер недоступний через підвисання фаєрвола, виклик чекає на системний таймаут сокета ОС (який може складати до 2-3 хвилин), блокуючи клієнтський потік ініціалізації Watch Party.
- **Рекомендація щодо виправлення:**
  Додати `.timeout(const Duration(seconds: 10))`.

---

## 5. Дефекти низького рівня ризику (Low / Severity P3)

### LOW-01: ProfilePage та LoginPage: відсутність finally та некоректний fallback помилок

- **Файли та рядки:**
  - `lib/presentation/pages/auth/profile_page.dart:284-297`
  - `lib/presentation/pages/auth/login_page.dart:533`
- **Опис проблеми:**
  У методі `_deleteAccount` відсутній блок `finally`, через що при виникненні непередбачуваної помилки індикатор `_isLoading` не скидається у вихідний стан. У діалозі відновлення пароля `login_page.dart` використовується `_authService.error` без елвіс-оператора `?? e.toString()`, що призводить до відображення тексту `Помилка: null`.
- **Рекомендація щодо виправлення:**
  Огорнути зміну стану в блок `finally` та забезпечити безпечний fallback рядка помилки.

---

### LOW-02: Використання порожніх блоків catch як штатного flow-control

- **Файли та рядки:**
  - `lib/presentation/pages/player/player_controls.dart:926-930`, `1178-1182`
  - `lib/presentation/pages/player/player_controller.dart:420`
- **Опис проблеми:**
  Пошук відповідної якості чи озвучки реалізовано через `firstWhere(...)` з обгорткою у `try { ... } catch (_) {}`. Використання винятків (`StateError`) для регулярного керування потоком виконання сповільнює роботу в Dart VM і заважає дебагу (triggering unhandled exception breakpoints).
- **Рекомендація щодо виправлення:**
  Замінити на стандартний безпечний метод `firstWhereOrNull` або передати іменований параметр `orElse: () => null`.

---

### LOW-03: Падіння пагінації у ProviderPage без інформування користувача

- **Файли та рядки:**
  - `lib/presentation/pages/provider/provider_page.dart:185-189`
- **Опис проблеми:**
  При невдачі довантаження наступної сторінки (`_loadMoreContent`) блок `catch` просто виставляє `_loadingByType[type] = false`. Інтерфейс нічого не сповіщає користувачу: скрол просто завмирає, і наступні сторінки не вантажаться.
- **Рекомендація щодо виправлення:**
  Показувати плаваючий `SnackBar` або кнопку "Спробувати довантажити знову" внизу списку.

---

## 6. Чеклист верифікації та план виправлення

1. [ ] **Патч безпеки Go-сервера (CRIT-01, CRIT-02):**
   - Додати `recover()` в `registry.go` та `main.go`.
   - Змінити порядок зупинки сервера в `main.go` (`server.Shutdown` -> `wsHub.GracefulStop`).
   - Додати перевірку `select` з `stopChan` у `wsHub.HandleWebSocket`.
2. [ ] **Патч відеоплеєра Flutter (CRIT-03):**
   - Зняти обмеження `_state.position == Duration.zero` у слухачі `_player.stream.error`.
3. [ ] **Патч таймаутів Watch Party (CRIT-04, MED-05):**
   - Додати таймаути на очікування брокера PeerDart (10s) та виклики `WebSocket.connect` (10s).
4. [ ] **Патч апстрім-відповідей провайдерів (HIGH-01, HIGH-05):**
   - Впровадити перевірку `resp.StatusCode` у `TLSClient.Get` / `PostForm`.
   - Показувати реальну помилку провайдера на головному екрані замість "Немає контенту".
5. [ ] **Патч транзакційної цілісності обраного (HIGH-02):**
   - Перевіряти помилки `favoritesRepo.AddFavorite` / `RemoveFavorite` у `sync_handler.go`.
6. [ ] **Патч UI-зависань (HIGH-03, HIGH-04, LOW-01):**
   - Додати `finally` блоки для скидання індикаторів завантаження.
   - Оновити агрегатор пошуку `search_service.dart`.
