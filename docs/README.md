# Документація проєкту Oxide Film

Ця директорія містить архітектурну, технічну документацію та звіти аудиту медіа-агрегатора **Oxide Film**.

---

## ⚠️ Актуальність документації

> **Джерело істини щодо стеку — [`../README.md`](../README.md).** Розділи 01-05 нижче було
> написано **до** міграції парсингу та авторизації на Go-сервер і містять застарілі твердження:
>
> | Застаріле твердження | Реальність |
> |---|---|
> | PocketBase (авторизація, синхронізація, Realtime SSE) | Go-бекенд у `server/`: JWT + PostgreSQL 16 + Redis |
> | Парсинг у Dart, 6 провайдерів (UAFlix, UAKino, Eneyida, UASerials, YummyAnime, YouTube) | Парсинг у Go: `uakino`, `lavakino`, `eneyida`, `bandera` |
> | WebRTC / PocketBase Realtime для Watch Party | Go WebSocket hub + Redis |
> | Web (beta) | Не підтримується — каталогу `web/` немає |
>
> Розділи **не переписані** (занадто великі для супровідної правки), а позначені як
> історичні. Див. також [`audit/`](audit/) та [`REMEDIATION_PLAN.md`](REMEDIATION_PLAN.md).

---

## 📚 Структура та зміст розділів

| Документ | Статус | Основні теми та зміст |
| :--- | :--- | :--- |
| **[01. Архітектура платформи та стек](01_TECH_STACK_AND_ARCHITECTURE.md)** | 🕐 історичний | • Загальний концепт та цілі проєкту (Offline-first, агрегація медіа)<br>• Повний технологічний стек<br>• Специфікація Clean Architecture (Domain, Data, Presentation, Core)<br>• Ланцюг старту та DI-реєстрації в `main.dart`<br>• Кросплатформна конфігурація (Windows безрамкові вікна, Android NDK/Permissions, Linux) |
| **[02. Джерела даних та меченізми парсингу](02_DATA_SOURCES_AND_PARSING.md)** | 🕐 історичний | • Базові інтерфейси `ContentProvider`, `ProviderRegistry`<br>• Розбір провайдерів<br>• Алгоритми обходу блокувань, PlayerJS парсинг<br>• Мережевий клієнт `ApiClient` (Dio, `dio_smart_retry`, `CookieJar`, `UserAgentService`) |
| **[03. Локальні дані, синхронізація та сервіси](03_DATABASE_SYNC_AND_CORE_SERVICES.md)** | 🕐 історичний | • Локальна база Drift (SQLite): ER-діаграма, таблиці, індекси<br>• Розбір DAO (`HistoryDao`, `FavoritesDao`, `DownloadsDao`, `SearchHistoryDao`, `SettingsDao`, `MediaItemsDao`)<br>• Синхронізація з бекендом<br>• Підсистема Watch Party<br>• TMDB API та OTA-оновлення (SHA-256) |
| **[04. UI/UX, Навігація та Медіаплеєр](04_UI_UX_AND_MEDIA_PLAYER.md)** | 🕐 історичний | • Карта екранів та конфігурація `GoRouter` + `ShellRoute`<br>• Дизайн-система, токени відступів, теми<br>• UI-компоненти: `MediaCard`, `FilterSheet`, `CustomTitleBar`<br>• Відеоплеєр на базі `media_kit` (libmpv), Picture-in-Picture, MiniPlayerOverlay |
| **[05. Управління станом та системні потоки](05_STATE_MANAGEMENT_AND_WORKFLOWS.md)** | 🕐 історичний | • Реєстр сервісів та контролерів стану<br>• Наскрізні зв'язки між шарами<br>• Mermaid-діаграми станів<br>• End-to-End User Journeys |
| **[06. Roadmap джерел даних](06_ROADMAP_DATA_SOURCES.md)** | ✅ актуальний | Дорожня карта розширення провайдерів |
| **[REMEDIATION_PLAN.md](REMEDIATION_PLAN.md)** | ✅ актуальний | План усунення знахідок аудиту: Wave 0-3, розподіл власності файлів між агентами, критерії приймання |
| **[audit/](audit/)** | ✅ актуальний | 8 звітів аудиту (5 основних + 3 незалежних) зі статусами `fixed` / `superseded`. Див. покажчик [audit/README.md](audit/README.md) |

---

## 🗺️ Граф знань (Graphify)

У проєкт інтегровано систему графового аналізу кодової бази **graphify**:
- **Звіт графа**: [`../graphify-out/GRAPH_REPORT.md`](../graphify-out/GRAPH_REPORT.md)
- **Інтерактивна візуалізація**: [`../graphify-out/graph.html`](../graphify-out/graph.html) (можна відкрити у будь-якому браузері)
- **Структуровані дані графа**: [`../graphify-out/graph.json`](../graphify-out/graph.json)
