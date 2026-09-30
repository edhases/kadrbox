import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/utils/logger.dart';

/// Remote configuration service for dynamic updates without app releases
///
/// Stores provider mirrors, CSS selectors, and other config that may need
/// to change frequently due to external factors (e.g., domain blocks).
class RemoteConfigService {
  static const String _tag = 'RemoteConfig';

  /// Remote config URL - replace with your own hosted JSON file
  /// Can be GitHub Gist, Firebase Remote Config, or any static JSON host
  static const String _remoteConfigUrl =
      'https://raw.githubusercontent.com/user/oxide_film_config/main/config.json';

  /// Fallback/cache key in SharedPreferences
  static const String _cacheKey = 'remote_config_cache';
  static const String _lastFetchKey = 'remote_config_last_fetch';

  /// Cache duration - refetch if older than this
  static const Duration _cacheDuration = Duration(hours: 6);

  final Dio _dio;

  /// Cached configuration
  RemoteConfig? _config;

  RemoteConfigService()
    : _dio = Dio(
        BaseOptions(
          connectTimeout: const Duration(seconds: 10),
          receiveTimeout: const Duration(seconds: 10),
        ),
      );

  /// Get current configuration (fetches if needed)
  Future<RemoteConfig> getConfig() async {
    if (_config != null) return _config!;

    // Try to load from cache first
    _config = await _loadFromCache();

    // Check if we should refresh
    if (await _shouldRefresh()) {
      try {
        final remote = await _fetchRemoteConfig();
        if (remote != null) {
          _config = remote;
          await _saveToCache(remote);
        }
      } catch (e) {
        Logger.w('Failed to fetch remote config: $e', tag: _tag);
        // Continue with cached config
      }
    }

    return _config ?? RemoteConfig.defaults();
  }

  /// Force refresh configuration from remote
  Future<RemoteConfig?> refresh() async {
    try {
      final remote = await _fetchRemoteConfig();
      if (remote != null) {
        _config = remote;
        await _saveToCache(remote);
        Logger.i('Remote config refreshed successfully', tag: _tag);
        return remote;
      }
    } catch (e) {
      Logger.e('Failed to refresh remote config', tag: _tag, error: e);
    }
    return null;
  }

  Future<RemoteConfig?> _fetchRemoteConfig() async {
    try {
      final response = await _dio.get<String>(_remoteConfigUrl);

      if (response.statusCode == 200 && response.data != null) {
        final json = jsonDecode(response.data!) as Map<String, dynamic>;
        return RemoteConfig.fromJson(json);
      }
    } on DioException catch (e) {
      Logger.w('DioException fetching config: ${e.type}', tag: _tag);
    } on FormatException catch (e) {
      Logger.w('Invalid JSON in remote config: ${e.message}', tag: _tag);
    }

    return null;
  }

  Future<RemoteConfig?> _loadFromCache() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final cached = prefs.getString(_cacheKey);

      if (cached != null) {
        final json = jsonDecode(cached) as Map<String, dynamic>;
        return RemoteConfig.fromJson(json);
      }
    } catch (e) {
      Logger.w('Failed to load cached config: $e', tag: _tag);
    }

    return null;
  }

  Future<void> _saveToCache(RemoteConfig config) async {
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_cacheKey, jsonEncode(config.toJson()));
      await prefs.setInt(_lastFetchKey, DateTime.now().millisecondsSinceEpoch);
    } catch (e) {
      Logger.w('Failed to cache config: $e', tag: _tag);
    }
  }

  Future<bool> _shouldRefresh() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final lastFetch = prefs.getInt(_lastFetchKey);

      if (lastFetch == null) return true;

      final lastFetchTime = DateTime.fromMillisecondsSinceEpoch(lastFetch);
      return DateTime.now().difference(lastFetchTime) > _cacheDuration;
    } catch (_) {
      return true;
    }
  }
}

/// Remote configuration data model
class RemoteConfig {
  /// Provider mirrors - providerId -> list of mirror URLs (ordered by priority)
  final Map<String, List<String>> providerMirrors;

  /// CSS selectors for DOM parsing - providerId -> selector key -> selector value
  final Map<String, Map<String, String>> cssSelectors;

  /// Feature flags
  final Map<String, bool> featureFlags;

  /// Minimum app version required (for force update)
  final String? minAppVersion;

  /// Message to show users (for announcements)
  final String? userMessage;

  const RemoteConfig({
    required this.providerMirrors,
    required this.cssSelectors,
    required this.featureFlags,
    this.minAppVersion,
    this.userMessage,
  });

  /// Default configuration when remote is unavailable
  factory RemoteConfig.defaults() {
    return const RemoteConfig(
      providerMirrors: {
        'hdrezka': ['https://hdrezka.ag', 'https://rezka.ag'],
        'uakino': ['https://uakino.biz', 'https://uakino.club', 'https://uakino.me'],
        'eneyida': ['https://eneyida.tv'],
        'lavakino': ['https://lavakino.net', 'https://lavakino.cc'],
      },
      cssSelectors: {
        'hdrezka': {
          'card': 'div.b-content__inline_item',
          'cardImage': 'img',
          'cardTitle': 'div.b-content__inline_item-link a',
        },
        'uakino': {
          'card': 'div.movie-item',
          'cardImage': 'img.movie-img',
          'cardTitle': 'a.movie-title',
        },
      },
      featureFlags: {
        'watchPartyEnabled': true,
        'downloadsEnabled': true,
        'tmdbIntegration': true,
      },
    );
  }

  factory RemoteConfig.fromJson(Map<String, dynamic> json) {
    return RemoteConfig(
      providerMirrors:
          (json['providerMirrors'] as Map<String, dynamic>?)?.map(
            (key, value) => MapEntry(key, (value as List).cast<String>()),
          ) ??
          {},
      cssSelectors:
          (json['cssSelectors'] as Map<String, dynamic>?)?.map(
            (key, value) => MapEntry(
              key,
              (value as Map<String, dynamic>).cast<String, String>(),
            ),
          ) ??
          {},
      featureFlags:
          (json['featureFlags'] as Map<String, dynamic>?)
              ?.cast<String, bool>() ??
          {},
      minAppVersion: json['minAppVersion'] as String?,
      userMessage: json['userMessage'] as String?,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'providerMirrors': providerMirrors,
      'cssSelectors': cssSelectors,
      'featureFlags': featureFlags,
      if (minAppVersion != null) 'minAppVersion': minAppVersion,
      if (userMessage != null) 'userMessage': userMessage,
    };
  }

  /// Get best available mirror for a provider
  String? getMirror(String providerId) {
    final mirrors = providerMirrors[providerId];
    return mirrors?.isNotEmpty == true ? mirrors!.first : null;
  }

  /// Get all mirrors for a provider
  List<String> getMirrors(String providerId) {
    return providerMirrors[providerId] ?? [];
  }

  /// Get CSS selector for a provider
  String? getSelector(String providerId, String selectorKey) {
    return cssSelectors[providerId]?[selectorKey];
  }

  /// Check if a feature is enabled
  bool isFeatureEnabled(String feature, {bool defaultValue = true}) {
    return featureFlags[feature] ?? defaultValue;
  }
}
