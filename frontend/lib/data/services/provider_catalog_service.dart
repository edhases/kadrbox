import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/utils/logger.dart';
import '../models/provider_catalog.dart';
import '../providers/provider_registry.dart';
import '../providers/server_backed_provider.dart';

/// Syncs the backend provider catalog into the app.
///
/// Backend is the source of truth: a provider added in backend Go code
/// appears everywhere (home, catalog, search, settings) via a
/// [ServerBackedProvider] adapter — no app update needed.
/// `enabled=false` from backend (kill-switch) hides the provider
/// regardless of the local user toggle.
///
/// Offline: falls back to the last cached catalog and sets [isStale].
class ProviderCatalogService extends ChangeNotifier {
  static const _tag = 'ProviderCatalog';
  // Cache keys are versioned: the v1 snapshot may still contain providers that
  // no longer exist locally (e.g. hdrezka), and [_applyCatalog] auto-registers a
  // working ServerBackedProvider for any unknown id. A stale snapshot would thus
  // resurrect a removed provider on every offline start — bypassing the backend
  // DISABLED_PROVIDERS kill-switch, since the snapshot carries its own
  // `enabled: true` and replaces the flag map wholesale. Bumping the key makes
  // the first launch after this release fall back to the static catalog below.
  static const _cacheKey = 'provider_catalog_json_v2';
  static const _versionKey = 'provider_catalog_version_v2';

  final ProviderRegistry _registry;
  final SharedPreferences _prefs;

  int _version = 0;
  bool _isStale = false;
  bool _synced = false;
  DateTime? _lastSync;

  ProviderCatalogService({ProviderRegistry? registry, SharedPreferences? prefs})
    : _registry = registry ?? GetIt.instance<ProviderRegistry>(),
      _prefs = prefs ?? GetIt.instance<SharedPreferences>();

  int get version => _version;
  bool get isStale => _isStale;
  bool get synced => _synced;
  DateTime? get lastSync => _lastSync;

  /// Syncs the backend provider catalog into the app.
  /// Backend no longer serves pirate scrapers.
  /// Offline / fallback catalog is strictly empty unless external plugin sources are added.
  Future<void> sync({bool force = false}) async {
    if (_synced && !force) return;
    // We do NOT fetch /content/providers from the backend anymore.
    // The backend is purely legitimate (auth, sync, watch party).
    await _applyCached(stale: false);
  }

  /// Apply backend catalog to [ProviderRegistry].
  Future<void> _applyCatalog(
    ProviderCatalog catalog, {
    bool stale = false,
  }) async {
    final backendStates = <String, bool>{};
    final showOnHome = <String, bool>{};
    final fixedStreams = <String, bool>{};
    final List<String> knownIds = [];

    for (final entry in catalog.providers) {
      backendStates[entry.id] = entry.enabled;
      showOnHome[entry.id] = entry.showOnHome;
      fixedStreams[entry.id] = entry.hasFixedStreams;
      knownIds.add(entry.id);

      if (_registry.getById(entry.id) == null) {
        _registry.register(ServerBackedProvider(entry));
        Logger.i('Registered server-backed provider: ${entry.id}', tag: _tag);
      }
    }

    _registry.applyBackendCatalog(
      enabled: backendStates,
      showOnHome: showOnHome,
      fixedStreams: fixedStreams,
    );

    _version = catalog.version;
    _isStale = stale;
    _synced = true;
    _lastSync = DateTime.now();

    notifyListeners();
  }

  Future<void> _applyCached({bool stale = true}) async {
    // Clear old scraper cache if present
    try {
      await _prefs.remove(_cacheKey);
      await _prefs.remove(_versionKey);
    } catch (_) {}

    // Clean default: zero hardcoded scraper providers.
    // External plugins / custom sources are connected dynamically.
    final fallbackEntries = <ProviderCatalogEntry>[];
    await _applyCatalog(
      ProviderCatalog(version: 1, providers: fallbackEntries),
      stale: stale,
    );
  }
}
