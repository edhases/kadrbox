Title: importFromJson imports favorites and history from valid JSON

Given:
- A valid JSON string exported by exportData containing 'favorites' and 'history' arrays and 'settings' map
- Mock DAOs for Favorites and History are ready to accept add/save calls

When:
- Call SyncService.importFromJson(jsonString)

Mocks/Helpers:
- MockFavoritesDao.add will record call params
- MockHistoryDao._importHistory stub to record saved entries

Expectations:
- importFromJson returns true
- FavoritesDao.add called for each favorite item with expected fields
- HistoryDao.saveProgress or insert called for each history entry
- Settings entries saved via SettingsDao.setSetting when included

Notes:
- If JSON has unsupported fields, they are ignored; function still succeeds if core arrays present
