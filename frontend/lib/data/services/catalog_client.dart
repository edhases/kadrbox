import 'dart:async';
import 'dart:convert';

import 'package:crypto/crypto.dart';
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

    final rawCaps = body['capabilities'];
    final capabilities = <String>{};
    if (rawCaps is List) {
      for (final c in rawCaps) {
        if (c is String) capabilities.add(c);
      }
    }

    final declared = body['catalog_id'];
    final fromServer = declared is String ? declared.trim() : '';

    return CatalogStatus(
      protocolVersion: version,
      catalogId: fromServer.isEmpty ? derivedCatalogId : fromServer,
      app: body['app'] is String ? body['app'] as String : baseUrl,
      capabilities: capabilities,
    );
  }

  /// Identity for a server that does not send `catalog_id`.
  ///
  /// `catalog_id` is the storage namespace: every stored reference is scoped
  /// `<catalog_id>:<item_id>`, so it has to be stable across restarts and
  /// different for two different servers. Deriving it from the normalised base
  /// URL gets both for free, with no server cooperation -- which matters,
  /// because the normative contract (contracts/openapi.yaml) never required the
  /// field. The client refusing a server for a field the contract does not ask
  /// for is the same failure the contract warns about from the other side: a
  /// server we cannot update must never be able to break a deployed client.
  ///
  /// The URL, not the host: two catalogues on one host (`/a` and `/b`) collided
  /// when the id came from the host alone. The digest is over the whole
  /// normalised URL, and the host is kept only as a readable prefix.
  late final String derivedCatalogId = _deriveCatalogId(baseUrl);

  static String _deriveCatalogId(String url) {
    final normalised = _normaliseUrl(url);
    final digest = sha256
        .convert(utf8.encode(normalised))
        .toString()
        .substring(0, 16);
    final host = Uri.tryParse(normalised)?.host ?? '';
    final readable = host.isEmpty ? 'catalog' : host;
    return '$readable-$digest';
  }

  /// Collapses the differences that must not create a second namespace:
  /// scheme and host case, a trailing slash, and the default port for the
  /// scheme. Anything beyond that is treated as a genuinely different server,
  /// because guessing at equivalence is how two catalogues silently share one
  /// namespace.
  static String _normaliseUrl(String url) {
    final trimmed = url.replaceAll(RegExp(r'/+$'), '');
    final uri = Uri.tryParse(trimmed);
    if (uri == null || uri.host.isEmpty) return trimmed.toLowerCase();

    final scheme = uri.scheme.toLowerCase();
    final host = uri.host.toLowerCase();
    final isDefaultPort = (scheme == 'http' && uri.port == 80) ||
        (scheme == 'https' && uri.port == 443);
    final authority = isDefaultPort ? host : '$host:${uri.port}';
    return '$scheme://$authority${uri.path}';
  }

  /// `GET /search`.
  ///
  /// [q] is optional on purpose: an empty query is the protocol's browse
  /// listing, which is what the home rows are built from. [page] is the opaque
  /// continuation token from a previous response and must never be synthesised
  /// from a page number.
  Future<Map<String, dynamic>> search({
    String? q,
    String? page,
    String? kind,
    String? voiceover,
    int? limit,
  }) => _getJson(
    '/search',
    query: {
      'q': ?q,
      'page': ?page,
      'kind': ?kind,
      'voiceover': ?voiceover,
      'limit': ?limit,
    },
    what: 'пошук',
  );

  /// `GET /details` for one item id from `/search`.
  Future<Map<String, dynamic>> details({
    required String id,
    String? voiceover,
  }) => _getJson(
    '/details',
    query: {'id': id, 'voiceover': ?voiceover},
    what: 'деталі',
  );

  /// `GET /streams` for an item id, or an episode id from `/details`.
  ///
  /// Never cached by the client: these URLs are signed and expire.
  Future<Map<String, dynamic>> streams({
    required String id,
    int? season,
    int? episode,
    String? voiceover,
  }) => _getJson(
    '/streams',
    query: {
      'id': id,
      'season': ?season,
      'episode': ?episode,
      'voiceover': ?voiceover,
    },
    what: 'потоки',
  );

  /// Shared GET that turns a non-200 into an actionable message.
  ///
  /// The content endpoints answer 404 legitimately -- "no such item", "nothing
  /// playable right now" -- so the message names what was being asked for
  /// instead of claiming the server is not a catalog at all.
  Future<Map<String, dynamic>> _getJson(
    String path, {
    required Map<String, dynamic> query,
    required String what,
  }) async {
    final response = await _get(path, query);
    final code = response.statusCode ?? 0;
    if (code == 404) {
      throw CatalogException(
        'Сервер каталогу не знає $what за цим запитом',
        statusCode: 404,
      );
    }
    if (code != 200) {
      throw CatalogException(_statusMessage(code), statusCode: code);
    }
    return _asMap(response.data);
  }

  Future<Response<dynamic>> _get(String path, [Map<String, dynamic>? query]) async {
    try {
      return await _dio.get<dynamic>(
        '$baseUrl$path',
        queryParameters: query,
      );
    } on DioException catch (e) {
      // Marked as transport so the caller can suppress a repeat: a catalog that
      // is unreachable will still be unreachable thirty seconds later.
      throw CatalogException(_transportMessage(e), isTransport: true);
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
  const CatalogException(this.message, {this.statusCode, this.isTransport = false});

  final String message;

  /// The HTTP status the catalog answered with, or null when the request never
  /// produced one.
  final int? statusCode;

  /// True when the request failed before or during transport -- DNS, TCP, TLS,
  /// a timeout -- rather than being answered.
  final bool isTransport;

  /// Whether the same request is worth repeating in a moment.
  ///
  /// A catalog that is down, timing out or answering 5xx will very likely still
  /// be in that state on the next tap, so the caller suppresses the repeat. A 404
  /// or a 403 is the caller's own doing and must never be suppressed: silently
  /// serving nothing because the user asked for something that does not exist
  /// reads as an empty catalogue, not a mistake.
  bool get isRetryable =>
      isTransport || (statusCode != null && statusCode! >= 500);

  @override
  String toString() => message;
}