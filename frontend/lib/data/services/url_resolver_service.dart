import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/utils/logger.dart';

/// Service for resolving provider URLs by following HTTP redirects
///
/// Many Ukrainian streaming sites frequently change domains (e.g., uaflix.net -> uafix.net).
/// This service automatically detects these changes by following HTTP redirects.
class UrlResolverService {
  static const String _tag = 'UrlResolver';
  static const String _cachePrefix = 'resolved_url_';
  static const Duration _cacheDuration = Duration(hours: 24);
  static const String _cacheTimePrefix = 'resolved_url_time_';

  final Dio _dio;
  final SharedPreferences _prefs;

  /// In-memory cache for current session
  final Map<String, String> _sessionCache = {};

  UrlResolverService(this._prefs)
    : _dio = Dio(
        BaseOptions(
          connectTimeout: const Duration(seconds: 10),
          receiveTimeout: const Duration(seconds: 10),
          followRedirects: false, // We'll handle redirects manually
          validateStatus: (status) => status != null && status < 400,
        ),
      );

  /// Resolve the actual URL for a provider by following redirects
  ///
  /// Returns the final URL after following all redirects, or the original URL if no redirects.
  /// Results are cached both in memory (session) and persistently (SharedPreferences).
  Future<String> resolveUrl(String originalUrl) async {
    // Check session cache first
    if (_sessionCache.containsKey(originalUrl)) {
      return _sessionCache[originalUrl]!;
    }

    // Check persistent cache
    final cached = _getCachedUrl(originalUrl);
    if (cached != null) {
      _sessionCache[originalUrl] = cached;
      return cached;
    }

    // Resolve by following redirects
    try {
      final resolvedUrl = await _followRedirects(originalUrl);

      // Cache the result
      _sessionCache[originalUrl] = resolvedUrl;
      await _cacheUrl(originalUrl, resolvedUrl);

      if (resolvedUrl != originalUrl) {
        Logger.i(
          'URL redirect detected: $originalUrl -> $resolvedUrl',
          tag: _tag,
        );
      }

      return resolvedUrl;
    } catch (e) {
      Logger.w('Failed to resolve URL $originalUrl: $e', tag: _tag);
      // Return original URL on error
      return originalUrl;
    }
  }

  /// Follow HTTP redirects and return the final URL
  Future<String> _followRedirects(String url) async {
    String currentUrl = url;
    int maxRedirects = 5;
    int redirectCount = 0;

    while (redirectCount < maxRedirects) {
      try {
        final response = await _dio.head(
          currentUrl,
          options: Options(
            followRedirects: false,
            validateStatus: (status) => status != null && status < 500,
          ),
        );

        // Check for redirect status codes
        if (response.statusCode == 301 ||
            response.statusCode == 302 ||
            response.statusCode == 307 ||
            response.statusCode == 308) {
          final location = response.headers.value('location');
          if (location != null && location.isNotEmpty) {
            // Handle relative URLs
            if (location.startsWith('/')) {
              final uri = Uri.parse(currentUrl);
              currentUrl = '${uri.scheme}://${uri.host}$location';
            } else if (!location.startsWith('http')) {
              final uri = Uri.parse(currentUrl);
              currentUrl = '${uri.scheme}://${uri.host}/$location';
            } else {
              currentUrl = location;
            }
            redirectCount++;
            Logger.d('Redirect $redirectCount: $location', tag: _tag);
            continue;
          }
        }

        // No more redirects
        break;
      } on DioException catch (e) {
        // If HEAD fails, try GET with range header (some servers don't support HEAD)
        if (e.type == DioExceptionType.badResponse ||
            e.response?.statusCode == 405) {
          return await _followRedirectsWithGet(currentUrl);
        }
        rethrow;
      }
    }

    // Extract base URL (remove path, keep just scheme + host)
    final uri = Uri.parse(currentUrl);
    return '${uri.scheme}://${uri.host}';
  }

  /// Fallback: follow redirects using GET with minimal data transfer
  Future<String> _followRedirectsWithGet(String url) async {
    try {
      final response =
          await Dio(
            BaseOptions(
              followRedirects: true,
              maxRedirects: 5,
              connectTimeout: const Duration(seconds: 10),
              receiveTimeout: const Duration(seconds: 5),
            ),
          ).get(
            url,
            options: Options(
              // Request minimal data
              headers: {'Range': 'bytes=0-0'},
              validateStatus: (status) => status != null && status < 500,
            ),
          );

      // Get the final URL after redirects
      final finalUri = response.realUri;
      return '${finalUri.scheme}://${finalUri.host}';
    } catch (e) {
      Logger.w('GET redirect resolution failed: $e', tag: _tag);
      final uri = Uri.parse(url);
      return '${uri.scheme}://${uri.host}';
    }
  }

  /// Get cached URL if not expired
  String? _getCachedUrl(String originalUrl) {
    final cachedUrl = _prefs.getString('$_cachePrefix$originalUrl');
    if (cachedUrl == null) return null;

    final cachedTime = _prefs.getInt('$_cacheTimePrefix$originalUrl') ?? 0;
    final cacheAge = DateTime.now().millisecondsSinceEpoch - cachedTime;

    if (cacheAge > _cacheDuration.inMilliseconds) {
      // Cache expired
      return null;
    }

    Logger.d('Using cached URL: $originalUrl -> $cachedUrl', tag: _tag);
    return cachedUrl;
  }

  /// Cache resolved URL
  Future<void> _cacheUrl(String originalUrl, String resolvedUrl) async {
    await _prefs.setString('$_cachePrefix$originalUrl', resolvedUrl);
    await _prefs.setInt(
      '$_cacheTimePrefix$originalUrl',
      DateTime.now().millisecondsSinceEpoch,
    );
  }

  /// Clear all cached URLs (useful for debugging or forcing refresh)
  Future<void> clearCache() async {
    _sessionCache.clear();
    final keys = _prefs.getKeys();
    for (final key in keys) {
      if (key.startsWith(_cachePrefix) || key.startsWith(_cacheTimePrefix)) {
        await _prefs.remove(key);
      }
    }
    Logger.i('URL cache cleared', tag: _tag);
  }

  /// Force refresh URL for a specific provider
  Future<String> forceResolve(String originalUrl) async {
    _sessionCache.remove(originalUrl);
    await _prefs.remove('$_cachePrefix$originalUrl');
    await _prefs.remove('$_cacheTimePrefix$originalUrl');
    return resolveUrl(originalUrl);
  }

  /// Get all currently resolved URLs (for debugging)
  Map<String, String> getResolvedUrls() {
    final result = <String, String>{};
    for (final key in _prefs.getKeys()) {
      if (key.startsWith(_cachePrefix) && !key.startsWith(_cacheTimePrefix)) {
        final original = key.substring(_cachePrefix.length);
        final resolved = _prefs.getString(key);
        if (resolved != null) {
          result[original] = resolved;
        }
      }
    }
    return result;
  }
}
