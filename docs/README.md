# Документація проєкту Oxide Film

Дана директорія містить вичерпну архітектурну, технічну та користувацьку документацію кросплатформного медіа-агрегатора **Oxide Film**, підготовлену командою з 5 спеціалізованих агентів-дослідників із застосуванням графового аналізу знань (**graphify**).

---

## 📚 Структура та зміст розділів

| Документ | Основні теми та зміст |
| :--- | :--- |
| **[01. Архітектура платформи та стек](01_TECH_STACK_AND_ARCHITECTURE.md)** | • Загальний концепт та цілі проєкту (Offline-first, агрегація медіа)<br>• Повний технологічний стек (Flutter Dart 3.10.7, Drift, GetIt, Dio, MediaKit, PocketBase тощо)<br>• Специфікація Clean Architecture (Domain, Data, Presentation, Core)<br>• Ланцюг старту та DI-реєстрації в `main.dart`<br>• Кросплатформна конфігурація (Windows безрамкові вікна, Android NDK/Permissions, Linux) |
| **[02. Джерела даних та механізми парсингу](02_DATA_SOURCES_AND_PARSING.md)** | • Базові інтерфейси `ContentProvider`, `ResolvedUrlMixin` та `ProviderRegistry`<br>• Розбір 6 провайдерів: UAFlix, UAKino, Eneyida, UASerials, YummyAnime, YouTube<br>• Алгоритми обходу блокувань, PlayerJS парсинг<br>• Мережевий клієнт `ApiClient` (Dio, автоматичні повтори `dio_smart_retry`, збереження `CookieJar`, ротація `UserAgentService`)<br>• Винесення парсингу DOM у фонові Dart Isolates через `compute()` |
| **[03. Локальні дані, синхронізація та сервіси](03_DATABASE_SYNC_AND_CORE_SERVICES.md)** | • Локальна база даних Drift (SQLite v9): ER-діаграма, таблиці, індекси, конвертери `DownloadStatus`<br>• Детальний розбір 6 DAO (`HistoryDao`, `FavoritesDao`, `DownloadsDao`, `SearchHistoryDao`, `SettingsDao`, `MediaItemsDao`)<br>• Інтеграція з бекендом PocketBase: автентифікація, збереження токенів, Realtime SSE синхронізація зі стратегією "Newer Timestamp Wins"<br>• Підсистема спільного перегляду **Watch Party**: дворівневий бекенд (PocketBase / WebRTC PeerDart) та багаторівневий алгоритм корекції Drift<br>• Інтеграція з TMDB API (метадані, постери, трейлери) та система безшовних OTA-оновлень (SHA-256) |
| **[04. UI/UX, Навігація та Медіаплеєр](04_UI_UX_AND_MEDIA_PLAYER.md)** | • Повна карта екранів (Sitemap) та конфігурація `GoRouter` + `ShellRoute` (для плаваючого плеєра)<br>• Дизайн-система, токени відступів `AppSpacing`, теми (Light, Dark, AMOLED True Black, кольорові акценти)<br>• UI-компоненти: `MediaCard`, `FilterSheet`, кастомний `CustomTitleBar` для десктопу, D-Pad/TV фокусування<br>• Відеоплеєр на базі `media_kit` (libmpv): апаратне декодування `auto-copy`, мультиаудіо, HLS/DASH/MP4, субтитри, алгоритм автоматичного пониження якості при лагах, режим Picture-in-Picture та MiniPlayerOverlay<br>• Підтримка хоткеїв для клавіатури/миші та жести керування гучністю/яскравістю на мобільних пристроях |
| **[05. Управління станом та системні потоки](05_STATE_MANAGEMENT_AND_WORKFLOWS.md)** | • Реєстр усіх сервісів та контролерів стану (`PlayerController`, `SettingsService`, `HistoryService`, `SmartSearchService` тощо)<br>• Специфікація наскрізних зв'язків між усіма шарами (UI -> Services -> Domain -> DataSources)<br>• Mermaid-діаграми станів для плеєра, пошуку та автентифікації<br>• Детальні End-to-End User Journeys (sequence diagrams): запуск, пошук із нечітким порівнянням (Fuzzy), завантаження джерел стрімінгу, збереження прогресу в БД Drift<br>• Реактивні патерни на базі Streams (Drift `watchAll()` -> UI, PocketBase SSE, WebRTC) |

---

## 🗺️ Граф знань (Graphify)

У проєкт інтегровано систему графового аналізу кодової бази **graphify**:
- **Звіт графа**: [`../graphify-out/GRAPH_REPORT.md`](../graphify-out/GRAPH_REPORT.md)
- **Інтерактивна візуалізація**: [`../graphify-out/graph.html`](../graphify-out/graph.html) (можна відкрити у будь-якому браузері)
- **Структуровані дані графа**: [`../graphify-out/graph.json`](../graphify-out/graph.json)
