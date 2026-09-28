import 'dart:io' show Platform;

import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';
import 'package:dio_cookie_manager/dio_cookie_manager.dart';
import 'package:dio_smart_retry/dio_smart_retry.dart';
import '../config/app_config.dart';
import '../error/exceptions.dart';
import '../../data/services/user_agent_service.dart';

/// HTTP client wrapper with retry logic, cookies, and error handling
class ApiClient {
  late final Dio _dio;
  final CookieJar _cookieJar = CookieJar();
  late final UserAgentService _uaService;

  ApiClient({UserAgentService? uaService}) {
    _uaService = uaService ?? UserAgentService(prefs: null as dynamic);

    _dio = Dio(
      BaseOptions(
        connectTimeout: AppConfig.connectTimeout,
        receiveTimeout: AppConfig.receiveTimeout,
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

    // Add retry interceptor
    _dio.interceptors.add(
      RetryInterceptor(
        dio: _dio,
        retries: 3,
        retryDelays: const [
          Duration(seconds: 1),
          Duration(seconds: 2),
          Duration(seconds: 3),
        ],
      ),
    );
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
        return Map<String, dynamic>.from(response.data as Map);
      }
      return {};
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
        final statusMessage = e.response?.statusMessage ?? 'Unknown';
        return ServerException(
          message: 'Server error $statusCode: $statusMessage',
          statusCode: statusCode,
          code: 'HTTP_$statusCode',
        );
      case DioExceptionType.cancel:
        return const NetworkException(
          message: 'Request was cancelled',
          code: 'CANCELLED',
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
