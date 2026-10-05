Title: exportData writes JSON with favorites, history and settings keys and returns file path

Given:
- FavoritesDao.getAll returns 2 favorite rows
- HistoryDao.getAll(limit: null) returns 3 history rows
- SettingsDao.getAllSettings returns a Map with several keys
- kIsWeb == false and getApplicationDocumentsDirectory returns a temporary directory

When:
- Call SyncService.exportData(includeFavorites: true, includeHistory: true, includeSettings: true)

Mocks/Helpers:
- Use a mock FavoritesDao, HistoryDao, SettingsDao to return data
- Use test/helpers/file_system_mocks.dart to set a mock documents directory

Expectations:
- exportData returns a non-null file path that ends with `.json`
- The file exists on disk and contains keys: 'favorites' (array length 2), 'history' (array length 3), 'settings' (map matches provided keys)
- The service's isExporting flags are correctly toggled around the operation

Notes:
- Also test exportData(includeFavorites: false, includeHistory: false, includeSettings: false) to ensure an empty payload still includes metadata (version, app)
