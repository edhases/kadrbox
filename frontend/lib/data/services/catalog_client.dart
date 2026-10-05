import 'dart:async';

import 'package:dio/dio.dart';

/// Client for a catalog server: the third-party service the user points the
/// app at. It is not our backend and must never be treated like one.
///
/// Three deliberate differences from `ApiClient`, which is why this is a
/// separate class rather than a constructor flag:
///
///   * **No Authorization header, ever.** Our bearer token belongs to our
///     backend. Sending it to a host the user typed would hand that host a
///     working credential, which is the same rule as SECURITY.md section 3.
///   * **No cookie jar.** Nothing about the session crosses this boundary.
///   * **Short timeouts.** A catalog is someone else's server and may be slow
///     or gone; the app must stay usable when it is.
class CatalogClient {
  /// Highest protocol version this build understands.
  static const int supportedProtocolVersion = 1;

  CatalogClient({required String baseUrl, Dio? dio})
    : baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), ''),
      _dio = dio ??
          Dio(
            BaseOptions(
              connectTimeout: const Duration(seconds: 6),
              receiveTimeout: const Duration(seconds: 12),
              sendTimeout: const Duration(seconds: 6),
              responseType: ResponseType.json,
              headers: {'Accept': 'application/json'},
              // Any status is handled below rather than thrown by dio, so that
              // a 404 can say "not a catalog" instead of "network error".
              validateStatus: (_) => true,
            ),
          );

  final String baseUrl;
  final Dio _dio;

  void dispose() => _dio.close(force: true);

  /// Performs the handshake and returns what the server says it is.
  ///
  /// Always called before a source is registered. The previous behaviour --
  /// registering on a scheme check alone and reporting success -- meant the app
  /// could confirm a source that had never been contacted.
  Future<CatalogStatus> handshake() async {
    final response = await _get('/status');
    // Dio 5 types statusCode as nullable even with validateStatus overridden, so
    // a null here means the response never carried one.
    final code = response.statusCode ?? 0;
    if (code != 200) {
      throw CatalogException(_statusMessage(code));
    }

    final body = _asMap(response.data);
    final version = body['protocol_version'];
    if (version is! int) {
      throw const CatalogException('Це не сервер каталогу Kadrbox');
    }

    final catalogId = body['catalog_id'];
    if (catalogId is! String || catalogId.trim().isEmpty) {
      throw const CatalogException(
        'Сервер не оголосив catalog_id, тому джерело неможливо розрізняти',
      );
    }

    final rawCaps = body['capabilities'];
    final capabilities = <String>{};
    if (rawCaps is List) {
      for (final c in rawCaps) {
        if (c is String) capabilities.add(c);
      }
    }

    return CatalogStatus(
      protocolVersion: version,
      catalogId: catalogId.trim(),
      app: body['app'] is String ? body['app'] as String : baseUrl,
      capabilities: capabilities,
    );
  }

  Future<Response<dynamic>> _get(String path, [Map<String, dynamic>? query]) async {
    try {
      return await _dio.get<dynamic>(
        '$baseUrl$path',
        queryParameters: query,
      );
    } on DioException catch (e) {
      throw CatalogException(_transportMessage(e));
    }
  }

  Map<String, dynamic> _asMap(dynamic data) {
    if (data is Map<String, dynamic>) return data;
    if (data is Map) return Map<String, dynamic>.from(data);
    throw const CatalogException('Сервер повернув не очікуваний формат');
  }

  /// A reason a user can act on, not a status code.
  String _statusMessage(int code) => switch (code) {
    404 => 'За цією адресою немає сервера каталогу',
    401 || 403 => 'Сервер каталогу не пускає без авторизації',
    429 => 'Сервер каталогу тимчасово обмежив запити. Спробуйте за хвилину',
    >= 500 => 'Сервер каталогу недоступний',
    _ => 'Сервер каталогу відповів помилкою ($code)',
  };

  String _transportMessage(DioException e) => switch (e.type) {
    DioExceptionType.connectionTimeout ||
    DioExceptionType.sendTimeout ||
    DioExceptionType.receiveTimeout =>
      'Сервер не відповідає. Перевірте адресу та VPN',
    DioExceptionType.connectionError =>
      'Не вдалося зєднатися. Перевірте адресу та VPN',
    DioExceptionType.badCertificate =>
      'Сервер не використовує HTTPS, або в нього недійсний сертифікат',
    DioExceptionType.badResponse => 'Сервер повернув неочікувану відповідь',
    _ => 'Помилка з’єднання: ${e.message ?? e.type.name}',
  };
}

/// What a catalog server says about itself.
class CatalogStatus {
  const CatalogStatus({
    required this.protocolVersion,
    required this.catalogId,
    required this.app,
    required this.capabilities,
  });

  final int protocolVersion;
  final String catalogId;
  final String app;
  final Set<String> capabilities;

  bool get isNewerThanApp => protocolVersion > CatalogClient.supportedProtocolVersion;
  bool get isOlderThanApp => protocolVersion < CatalogClient.supportedProtocolVersion;

  bool supports(String capability) => capabilities.contains(capability);

  /// A warning worth showing but not worth refusing over.
  ///
  /// Refusing a newer server would blank the app for a user whose server simply
  /// moved ahead; the client is required to degrade, so it says so and carries on.
  String? get compatibilityWarning {
    if (isNewerThanApp) {
      return 'Сервер новіший за застосунок. Можлива неповна поведінка.';
    }
    if (isOlderThanApp) {
      return 'Застарілий сервер. Можлива неповна поведінка.';
    }
    return null;
  }
}

/// A failure with a reason already phrased for the user.
class CatalogException implements Exception {
  const CatalogException(this.message);

  final String message;

  @override
  String toString() => message;
}