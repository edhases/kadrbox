Title: HistoryDao.getContinueWatching filters items by progress (5%-95%)

Given:
- Database seeded with watch history entries with durations and positions such that their progress percentages are: 1%, 10%, 50%, 96%.
- Use an in-memory AppDatabase via createTestAppDatabase().

When:
- Call HistoryDao.getContinueWatching(limit: 10)

Mocks/Helpers:
- Use test/helpers/in_memory_db.dart and seed watch_history table with sample entries.

Expectations:
- Returned list contains only items with 10% and 50% (progress >5% and <95%).
- Items are ordered by watchedAt desc (most recent first) and length == 2.

Notes:
- Ensure entries with durationMs == 0 are excluded.
- Edge case: if durations are zero or position == 0, items must not be returned.
