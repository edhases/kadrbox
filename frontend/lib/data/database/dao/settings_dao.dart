import 'package:drift/drift.dart';
import '../app_database.dart';

/// Data Access Object for application settings
class SettingsDao {
  final AppDatabase _db;

  SettingsDao(this._db);

  // ============================================================================
  // APP SETTINGS
  // ============================================================================

  /// Get a setting value by key
  Future<String?> getSetting(String key) async {
    final query = _db.select(_db.appSettings)..where((t) => t.key.equals(key));
    final result = await query.getSingleOrNull();
    return result?.value;
  }

  /// Set a setting value
  Future<void> setSetting(String key, String value) async {
    await _db
        .into(_db.appSettings)
        .insert(
          AppSettingsCompanion.insert(key: key, value: value),
          onConflict: DoUpdate(
            (old) => AppSettingsCompanion(
              value: Value(value),
              updatedAt: Value(DateTime.now()),
            ),
            target: [_db.appSettings.key],
          ),
        );
  }

  /// Get all settings as a map
  Future<Map<String, String>> getAllSettings() async {
    final results = await _db.select(_db.appSettings).get();
    return {for (var row in results) row.key: row.value};
  }

  /// Watch a specific setting
  Stream<String?> watchSetting(String key) {
    final query = _db.select(_db.appSettings)..where((t) => t.key.equals(key));
    return query.watchSingleOrNull().map((row) => row?.value);
  }

  /// Watch all settings
  Stream<Map<String, String>> watchAllSettings() {
    return _db.select(_db.appSettings).watch().map((rows) {
      return {for (var row in rows) row.key: row.value};
    });
  }

  // ============================================================================
  // PROVIDER SETTINGS
  // ============================================================================

  /// Get all enabled providers
  Future<List<EnabledProvider>> getEnabledProviders() async {
    final query = _db.select(_db.enabledProviders)
      ..orderBy([(t) => OrderingTerm.asc(t.priority)]);
    return query.get();
  }

  /// Check if a provider is enabled
  Future<bool> isProviderEnabled(String providerId) async {
    final query = _db.select(_db.enabledProviders)
      ..where((t) => t.providerId.equals(providerId));
    final result = await query.getSingleOrNull();
    return result?.isEnabled ?? true; // Default to enabled
  }

  /// Set provider enabled state
  Future<void> setProviderEnabled(String providerId, bool isEnabled) async {
    await _db
        .into(_db.enabledProviders)
        .insert(
          EnabledProvidersCompanion.insert(
            providerId: providerId,
            isEnabled: Value(isEnabled),
          ),
          onConflict: DoUpdate(
            (old) => EnabledProvidersCompanion(
              isEnabled: Value(isEnabled),
              updatedAt: Value(DateTime.now()),
            ),
            target: [_db.enabledProviders.providerId],
          ),
        );
  }

  /// Set provider priority (for ordering)
  Future<void> setProviderPriority(String providerId, int priority) async {
    await (_db.update(_db.enabledProviders)
          ..where((t) => t.providerId.equals(providerId)))
        .write(EnabledProvidersCompanion(priority: Value(priority)));
  }

  /// Watch provider enabled states
  Stream<Map<String, bool>> watchProviderStates() {
    return _db.select(_db.enabledProviders).watch().map((rows) {
      return {for (var row in rows) row.providerId: row.isEnabled};
    });
  }

  /// Clear all app settings (reset to defaults)
  Future<void> clearAllSettings() async {
    await _db.delete(_db.appSettings).go();
  }
}
