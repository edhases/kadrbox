import 'package:flutter_cache_manager/flutter_cache_manager.dart';
import 'package:http/http.dart' as http;

/// Poster requests need a browser-like User-Agent.
///
/// The CDNs in front of the providers reject the default client string with
/// HTTP 403, which surfaced as a FlutterError per poster and left every card
/// blank.
///
/// `CachedNetworkImage` 3.x has no `headers` parameter, and its `FileService`
/// API differs between flutter_cache_manager releases, so the header is
/// attached to the underlying [http.Client] instead. That keeps this working
/// across versions and covers every cache-backed poster request.
const String _browserUserAgent =
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 '
    '(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36';

/// Headers needed to fetch posters from CDNs with Cloudflare / hotlink protections.
Map<String, String>? getPosterHeaders(String? url) {
  if (url == null || url.isEmpty) return null;
  final uri = Uri.tryParse(url);
  if (uri == null || uri.host.isEmpty) return null;
  return {
    'User-Agent': _browserUserAgent,
    'Accept': 'image/avif,image/webp,image/apng,image/*,*/*;q=0.8',
    'Referer': 'https://${uri.host}/',
  };
}

/// Normalises poster URLs (rewrites old uakino mirrors, validates http/https scheme).
/// Returns null for invalid or non-URL identifiers (e.g. raw UUIDs).
String? sanitizePosterUrl(String? url) {
  if (url == null || url.trim().isEmpty) return null;
  final trimmed = url.trim();
  if (!trimmed.startsWith('http://') && !trimmed.startsWith('https://')) {
    return null;
  }
  return trimmed
      .replaceAll('uakino.best', 'uakino.biz')
      .replaceAll('uakino.me', 'uakino.biz');
}

/// Shared cache manager for remote poster artwork.
///
/// One instance so the on-disk cache is shared across every poster widget and
/// the header cannot drift between call sites.
final CacheManager posterCacheManager = CacheManager(
  Config(
    'posterImages',
    stalePeriod: const Duration(days: 14),
    fileService: HttpFileService(httpClient: _PosterImageClient()),
  ),
);

/// Adds browser-ish headers to every poster request.
class _PosterImageClient extends http.BaseClient {
  final http.Client _inner = http.Client();

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) {
    var effectiveRequest = request;
    if (request.url.host == 'uakino.best' || request.url.host == 'uakino.me') {
      final newUri = request.url.replace(host: 'uakino.biz');
      final newReq = http.Request(request.method, newUri);
      newReq.headers.addAll(request.headers);
      effectiveRequest = newReq;
    }

    effectiveRequest.headers['User-Agent'] = _browserUserAgent;
    effectiveRequest.headers['Accept'] =
        'image/avif,image/webp,image/apng,image/*,*/*;q=0.8';
    if (effectiveRequest.url.host.isNotEmpty) {
      effectiveRequest.headers['Referer'] =
          'https://${effectiveRequest.url.host}/';
    }
    return _inner.send(effectiveRequest);
  }

  @override
  void close() {
    _inner.close();
    super.close();
  }
}
