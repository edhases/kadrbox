import 'dart:async';
import 'dart:io' show Platform;

import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';
import 'package:dio_cookie_manager/dio_cookie_manager.dart';
import 'package:dio_smart_retry/dio_smart_retry.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../config/app_config.dart';
import '../error/exceptions.dart';
import '../../data/services/user_agent_service.dart';

/// Exchanges the refresh token for a new access token. Returns `true` when a
/// fresh access token was installed.
typedef AuthRefreshHandler = Future<bool> Function();

/// HTTP methods that are safe to replay after a transient failure.
const Set<String> _idempotentMethods = {
  'GET',
  'HEAD',
  'PUT',
  'DELETE',
  'OPTIONS',
};

/// Paths that must never be retried: replaying a consumed refresh token turns
/// a recoverable blip into a 401 and an unwanted sign-out.
const List<String> _nonRetryablePathFragments = [
  '/api/v1/auth/refresh',
  '/auth/refresh',
];

/// HTTP client wrapper with retry logic, cookies, and error handling
class ApiClient {
  /// Access tokens live 15 minutes server-side (`auth_handler.go`). Refresh at
  /// 80% of that so the common case never needs a 401 round-trip.
  static const Duration accessTokenTtl = Duration(minutes: 15);
  static const Duration proactiveRefreshInterval = Duration(minutes: 12);

  static const String _authRetryFlag = 'oxide.auth_retried';

  late final Dio _dio;
  final CookieJar _cookieJar = CookieJar();
  late final UserAgentService _uaService;

  AuthRefreshHandler? _onUnauthorized;
  Future<void> Function()? _onSignOut;
  bool _authInterceptorInstalled = false;
  final bool _ownsLifecycle;
  Future<bool>? _refreshInFlight;
  Timer? _proactiveRefreshTimer;

  /// Whether a bearer token is currently installed. Used to skip proactive
  /// refreshes for guests (which would otherwise sign them out).
  bool get hasAuthToken {
    final header = _dio.options.headers['Authorization'];
    return header is String && header.isNotEmpty;
  }

  /// [dio] is a test seam: when provided, the client uses it as-is and does
  /// not install cookie/retry/proactive-refresh interceptors. Production code
  /// never passes it. The 401 interceptor is still installed when
  /// [onUnauthorized] is supplied, since it is inert until a 401 arrives.
  ApiClient({
    UserAgentService? uaService,
    SharedPreferences? prefs,
    Dio? dio,
    AuthRefreshHandler? onUnauthorized,
    Future<void> Function()? onSignOut,
  }) : _onUnauthorized = onUnauthorized,
       _onSignOut = onSignOut,
       _ownsLifecycle = dio == null {
    if (uaService != null) {
      _uaService = uaService;
    } else if (prefs != null) {
      _uaService = UserAgentService(prefs: prefs);
    } else {
      // No prefs available (e.g. early init): use stateless fallback UA.
      _uaService = UserAgentService.fallback();
    }

    if (dio != null) {
      _dio = dio;
      _installAuthInterceptor();
      return;
    }

    _dio = Dio(
      BaseOptions(
        connectTimeout: AppConfig.connectTimeout,
        receiveTimeout: AppConfig.receiveTimeout,
        sendTimeout: AppConfig.connectTimeout,
        headers: {
          'User-Agent': _uaService.getRandomUA(),
          'Accept':
              'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
          'Accept-Language': _buildAcceptLanguage(),
        },
      ),
    );

    // Add cookie manager for session persistence (PHPSESSID etc.)
    _dio.interceptors.add(CookieManager(_cookieJar));

    // Add retry interceptor (only for network errors / 5xx on idempotent calls)
    _dio.interceptors.add(
      RetryInterceptor(
        dio: _dio,
        retries: 3,
        retryDelays: const [
          Duration(seconds: 1),
          Duration(seconds: 2),
          Duration(seconds: 3),
        ],
        retryEvaluator: (error, _) {
          final options = error.requestOptions;
          if (!_isRetryableMethod(options) ||
              _isNonRetryablePath(options.path)) {
            return false;
          }
          if (error.type == DioExceptionType.badResponse) {
            final code = error.response?.statusCode ?? 0;
            // 400/401/403/422 are deterministic client errors — replaying them
            // only wastes time and, for 401, can cascade into a sign-out.
            if (code == 400 || code == 401 || code == 403 || code == 422) {
              return false;
            }
            return code >= 500;
          }
          return error.type == DioExceptionType.connectionTimeout ||
              error.type == DioExceptionType.receiveTimeout ||
              error.type == DioExceptionType.sendTimeout ||
              error.type == DioExceptionType.connectionError ||
              error.type == DioExceptionType.transformTimeout ||
              error.type == DioExceptionType.unknown;
        },
      ),
    );

    _startProactiveRefresh();
    _installAuthInterceptor();
  }

  /// A request may be retried only when replaying it cannot change state.
  /// `POST` is included solely when the caller supplies an `Idempotency-Key`.
  static bool _isRetryableMethod(RequestOptions options) {
    final method = options.method.toUpperCase();
    if (_idempotentMethods.contains(method)) return true;
    if (method != 'POST') return false;
    final headers = options.headers;
    return headers.keys.any(
      (k) =>
          k.toLowerCase() == 'idempotency-key' &&
          (headers[k]?.toString().isNotEmpty ?? false),
    );
  }

  static bool _isNonRetryablePath(String path) {
    return _nonRetryablePathFragments.any((f) => path.contains(f));
  }

  // ===========================================================================
  // 401 handling: refresh once, then replay the original request
  // ===========================================================================

  void _installAuthInterceptor() {
    if (_onUnauthorized == null || _authInterceptorInstalled) return;
    _authInterceptorInstalled = true;
    _dio.interceptors.add(
      InterceptorsWrapper(
        onError: (err, handler) async {
          final options = err.requestOptions;
          if (err.response?.statusCode != 401 ||
              options.extra[_authRetryFlag] == true ||
              _isNonRetryablePath(options.path)) {
            handler.next(err);
            return;
          }

          final refreshed = await _refreshOnce();
          if (!refreshed) {
            // Refresh itself failed (401 / network). Do NOT replay: retrying a
            // non-idempotent request with a dead session is worse than failing.
            await _forceSignOut();
            handler.next(err);
            return;
          }

          // A 401 means the auth middleware rejected the request before any
          // handler ran, so replaying is safe even for POST.
          try {
            options.extra[_authRetryFlag] = true;
            final response = await _dio.fetch<dynamic>(options);
            handler.resolve(response);
          } on DioException catch (retryError) {
            if (retryError.response?.statusCode == 401) {
              await _forceSignOut();
            }
            handler.next(retryError);
          }
        },
      ),
    );
  }

  /// Refresh the access token, collapsing concurrent 401s onto one HTTP call.
  Future<bool> _refreshOnce() {
    final existing = _refreshInFlight;
    if (existing != null) return existing;
    final future = _runRefresh();
    _refreshInFlight = future;
    return future;
  }

  Future<bool> _runRefresh() async {
    try {
      final handler = _onUnauthorized;
      if (handler == null) return false;
      return await handler();
    } catch (_) {
      return false;
    } finally {
      _refreshInFlight = null;
    }
  }

  Future<void> _forceSignOut() async {
    // `OxideServerService.signOut` is the single place that clears the session;
    // reaching it through a callback keeps this file unaware of the pref store.
    try {
      await _onSignOut?.call();
    } catch (_) {
      // Best effort: a failure here must not mask the original 401.
    }
  }

  void _startProactiveRefresh() {
    // Only when this client created its own Dio: a caller-supplied instance is a
    /// test seam and must not leave a periodic timer behind.
    if (!_ownsLifecycle) return;
    _proactiveRefreshTimer?.cancel();
    _proactiveRefreshTimer = Timer.periodic(proactiveRefreshInterval, (_) {
      if (!hasAuthToken) return;
      unawaited(_refreshOnce());
    });
  }

  /// Wires the session callbacks after construction.
  ///
  /// The DI container builds `ApiClient` before `OxideServerService` (which owns
  /// the tokens), so the service attaches itself once it exists. Safe to call
  /// more than once.
  void attachAuthCallbacks({
    AuthRefreshHandler? refresh,
    Future<void> Function()? signOut,
  }) {
    if (signOut != null) _onSignOut = signOut;
    if (refresh != null) _onUnauthorized = refresh;
    if (_onUnauthorized == null) return;
    _installAuthInterceptor();
    _startProactiveRefresh();
  }

  /// Hook for `WidgetsBindingObserver.didChangeAppLifecycleState`: call this on
  /// `AppLifecycleState.resumed` so a token that expired in the background is
  /// renewed before the next request instead of costing a 401 round-trip.
  void onAppResumed() {
    if (!hasAuthToken) return;
    unawaited(_refreshOnce());
  }

  /// Releases the proactive refresh timer. The service is a GetIt singleton, so
  /// this only matters in tests and on logout/account switch.
  void dispose() {
    _proactiveRefreshTimer?.cancel();
    _proactiveRefreshTimer = null;
  }

  /// Build Accept-Language header based on system locale with fallback to Ukrainian
  static String _buildAcceptLanguage() {
    try {
      final locale = Platform.localeName; // e.g., "uk_UA", "en_US"
      final parts = locale.split('_');
      final lang = parts.isNotEmpty ? parts[0] : 'uk';
      final country = parts.length > 1 ? parts[1] : '';

      // Build proper Accept-Language header with quality values
      if (country.isNotEmpty) {
        return '$lang-$country,$lang;q=0.9,en-US;q=0.8,en;q=0.7';
      }
      return '$lang;q=0.9,en-US;q=0.8,en;q=0.7';
    } catch (_) {
      // Fallback for platforms where Platform.localeName is not available
      return 'uk-UA,uk;q=0.9,en-US;q=0.8,en;q=0.7';
    }
  }

  /// GET request that returns response body as String
  Future<String> get(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.get<String>(
        url,
        queryParameters: queryParameters,
        options: Options(headers: headers, responseType: ResponseType.plain),
      );
      return response.data ?? '';
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  /// Set or clear Authorization Bearer token
  void setAuthToken(String? token) {
    if (token != null && token.isNotEmpty) {
      _dio.options.headers['Authorization'] = 'Bearer $token';
    } else {
      _dio.options.headers.remove('Authorization');
    }
  }

  /// GET request that returns JSON Map
  Future<Map<String, dynamic>> getJson(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.get(
        url,
        queryParameters: queryParameters,
        options: Options(headers: headers),
      );
      if (response.data is Map) {
        // The server wraps object payloads in {"data": ...}. Unwrap here so
        // every caller sees the bare object; without this the provider catalog
        // parsed to zero entries and the home screen rendered empty.
        final unwrapped = _unwrapEnvelope(response.data);
        if (unwrapped is Map) {
          return Map<String, dynamic>.from(unwrapped);
        }
      }
      return {};
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  /// GET request that returns raw response data (Map or List or dynamic)
  Future<dynamic> getRawJson(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.get(
        url,
        queryParameters: queryParameters,
        options: Options(headers: headers),
      );
      return response.data;
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  /// GET request that returns JSON List
  Future<List<dynamic>> getJsonList(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.get(
        url,
        queryParameters: queryParameters,
        options: Options(headers: headers),
      );
      if (response.data is List) {
        return List<dynamic>.from(response.data as List);
      }
      // The server wraps list payloads in {"data": [...], "meta": {...}}.
      // Without this every list endpoint silently decoded to an empty list,
      // which surfaced as an empty home screen rather than as an error.
      final unwrapped = _unwrapEnvelope(response.data);
      if (unwrapped is List) {
        return List<dynamic>.from(unwrapped);
      }
      return [];
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  /// POST request returning Map or dynamic
  Future<dynamic> post(
    String url, {
    dynamic data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.post(
        url,
        data: data,
        queryParameters: queryParameters,
        options: Options(headers: headers),
      );
      return response.data;
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  /// PUT request returning Map or dynamic
  Future<dynamic> put(
    String url, {
    dynamic data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.put(
        url,
        data: data,
        queryParameters: queryParameters,
        options: Options(headers: headers),
      );
      return response.data;
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  /// DELETE request returning Map or dynamic
  Future<dynamic> delete(
    String url, {
    dynamic data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    try {
      final response = await _dio.delete(
        url,
        data: data,
        queryParameters: queryParameters,
        options: Options(headers: headers),
      );
      return response.data;
    } on DioException catch (e) {
      throw _handleDioError(e);
    }
  }

  // ===========================================================================
  // Typed primitives
  //
  // These keep the `raw is Map` guard inside ApiClient, so call sites stop
  // spreading `dynamic` (and unchecked `as Map` casts) across the codebase.
  // ===========================================================================

  /// POST expecting a JSON object. Returns an empty map for any other payload.
  Future<Map<String, dynamic>> postJson(
    String url, {
    Object? data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    final raw = await post(
      url,
      data: data,
      queryParameters: queryParameters,
      headers: headers,
    );
    return _asJsonMap(raw);
  }

  /// POST expecting a JSON array of objects. Returns `[]` for any other payload.
  Future<List<Map<String, dynamic>>> postJsonList(
    String url, {
    Object? data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    final raw = await post(
      url,
      data: data,
      queryParameters: queryParameters,
      headers: headers,
    );
    return _asJsonMapList(raw);
  }

  /// PUT expecting a JSON object. Returns an empty map for any other payload.
  Future<Map<String, dynamic>> putJson(
    String url, {
    Object? data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    final raw = await put(
      url,
      data: data,
      queryParameters: queryParameters,
      headers: headers,
    );
    return _asJsonMap(raw);
  }

  /// DELETE expecting a JSON object. Returns an empty map for any other payload.
  Future<Map<String, dynamic>> deleteJson(
    String url, {
    Object? data,
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    final raw = await delete(
      url,
      data: data,
      queryParameters: queryParameters,
      headers: headers,
    );
    return _asJsonMap(raw);
  }

  /// GET expecting a JSON array of objects, or the `{"data": [...]}` envelope the
  /// server now returns for list endpoints. Returns `[]` for any other payload.
  Future<List<Map<String, dynamic>>> getJsonMapList(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    final raw = await getJsonList(
      url,
      queryParameters: queryParameters,
      headers: headers,
    );
    return _asJsonMapList(raw);
  }

  /// Same as [getJsonMapList] but also surfaces the pagination metadata the
  /// server puts in `meta` (offset-paginated endpoints only).
  Future<ListWithMeta> getJsonEnvelopeList(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    final raw = await get(
      url,
      queryParameters: queryParameters,
      headers: headers,
    );
    final unwrapped = _unwrapEnvelope(raw);
    if (unwrapped is Map) {
      final meta = unwrapped['meta'];
      return ListWithMeta(
        items: _asJsonMapList(unwrapped['data']),
        meta: meta is Map ? Map<String, dynamic>.from(meta) : const {},
      );
    }
    return ListWithMeta(items: _asJsonMapList(unwrapped), meta: const {});
  }

  /// Unwraps the `{"data": ...}` envelope used by every success response,
  /// falling back to the raw payload for legacy/bare responses.
  ///
  /// Deliberately strict: the envelope is only recognised when `data` is
  /// accompanied exclusively by envelope keys. A bare domain object that
  /// happens to have a field named `data` must pass through untouched, or the
  /// caller would silently receive an empty map instead of its payload.
  static Object? _unwrapEnvelope(Object? raw) {
    if (raw is! Map) return raw;
    if (!raw.containsKey('data')) return raw;
    const envelopeKeys = {
      'data',
      'meta',
      'error',
      'status',
      'message',
      'success',
    };
    final keys = raw.keys.map((k) => k.toString()).toSet();
    if (!keys.every(envelopeKeys.contains)) return raw;
    return raw['data'];
  }

  static Map<String, dynamic> _asJsonMap(Object? raw) {
    final unwrapped = _unwrapEnvelope(raw);
    if (unwrapped is Map) return Map<String, dynamic>.from(unwrapped);
    return const <String, dynamic>{};
  }

  static List<Map<String, dynamic>> _asJsonMapList(Object? raw) {
    final unwrapped = _unwrapEnvelope(raw);
    if (unwrapped is List) {
      return unwrapped.whereType<Map>().map(Map<String, dynamic>.from).toList();
    }
    return const <Map<String, dynamic>>[];
  }

  AppException _handleDioError(DioException e) {
    // Preserve original error details for debugging
    final originalError = e.error;
    final originalMessage = e.message ?? 'Unknown error';

    switch (e.type) {
      case DioExceptionType.connectionTimeout:
        return NetworkException(
          message:
              'Connection timeout after ${AppConfig.connectTimeout.inSeconds}s',
          code: 'CONNECTION_TIMEOUT',
        );
      case DioExceptionType.sendTimeout:
        return const NetworkException(
          message: 'Request send timeout',
          code: 'SEND_TIMEOUT',
        );
      case DioExceptionType.receiveTimeout:
        return NetworkException(
          message:
              'Response timeout after ${AppConfig.receiveTimeout.inSeconds}s',
          code: 'RECEIVE_TIMEOUT',
        );
      case DioExceptionType.connectionError:
        // Preserve the actual connection error type (DNS, SSL, etc.)
        final errorType = _classifyConnectionError(originalError);
        return NetworkException(
          message: 'Connection failed: $errorType',
          code: 'CONNECTION_ERROR',
        );
      case DioExceptionType.badCertificate:
        return const NetworkException(
          message: 'SSL certificate validation failed',
          code: 'SSL_ERROR',
        );
      case DioExceptionType.badResponse:
        final statusCode = e.response?.statusCode ?? 0;
        String message =
            'Server error $statusCode: ${e.response?.statusMessage ?? 'Unknown'}';
        final data = e.response?.data;
        if (data is Map) {
          if (data['error'] != null &&
              data['error'].toString().trim().isNotEmpty) {
            message = data['error'].toString().trim();
          } else if (data['message'] != null &&
              data['message'].toString().trim().isNotEmpty) {
            message = data['message'].toString().trim();
          }
        } else if (data is String && data.trim().isNotEmpty) {
          message = data.trim();
        }
        return ServerException(
          message: message,
          statusCode: statusCode,
          code: 'HTTP_$statusCode',
        );
      case DioExceptionType.cancel:
        return const NetworkException(
          message: 'Request was cancelled',
          code: 'CANCELLED',
        );
      case DioExceptionType.transformTimeout:
        return const NetworkException(
          message: 'Request transformation timeout',
          code: 'TRANSFORM_TIMEOUT',
        );
      case DioExceptionType.unknown:
        return NetworkException(
          message: 'Network error: $originalMessage',
          code: 'UNKNOWN',
        );
    }
  }

  /// Classify connection errors for better debugging
  String _classifyConnectionError(Object? error) {
    if (error == null) return 'Unknown connection error';

    final errorStr = error.toString().toLowerCase();

    if (errorStr.contains('socketexception')) {
      if (errorStr.contains('connection refused')) {
        return 'Connection refused - server may be down';
      }
      if (errorStr.contains('network is unreachable')) {
        return 'Network unreachable - no internet connection';
      }
      if (errorStr.contains('no route to host')) {
        return 'No route to host';
      }
      return 'Socket error';
    }

    if (errorStr.contains('handshakeexception') || errorStr.contains('ssl')) {
      return 'SSL/TLS handshake failed';
    }

    if (errorStr.contains('dns') ||
        errorStr.contains('getaddrinfo') ||
        errorStr.contains('nodename nor servname provided')) {
      return 'DNS resolution failed';
    }

    if (errorStr.contains('timeout')) {
      return 'Connection timed out';
    }

    return error.runtimeType.toString();
  }

  /// Get the underlying Dio instance for advanced usage
  Dio get dio => _dio;
}

/// Items plus the `meta` object of a paginated `{"data": [...], "meta": {...}}`
/// response. `hasMore` is derived when the server supplies `total`, and falls
/// back to "a full page was returned" for catalogue endpoints that expose no
/// total — those may still advertise a next page that yields `[]`.
class ListWithMeta {
  const ListWithMeta({required this.items, required this.meta});

  final List<Map<String, dynamic>> items;
  final Map<String, dynamic> meta;

  int get count => items.length;

  bool get hasMore {
    final explicit = meta['has_more'];
    if (explicit is bool) return explicit;
    if (explicit is num) return explicit != 0;
    final total = meta['total'];
    final offset = meta['offset'];
    final limit = meta['limit'];
    if (total is num && offset is num && limit is num && limit > 0) {
      return offset.toInt() + items.length < total.toInt();
    }
    return items.isNotEmpty;
  }

  int? get total =>
      meta['total'] is num ? (meta['total'] as num).toInt() : null;

  int? get nextOffset {
    final offset = meta['offset'];
    final limit = meta['limit'];
    if (offset is num && limit is num) return (offset + limit).toInt();
    final page = meta['page'];
    if (page is num) return (page + 1).toInt();
    return null;
  }
}
