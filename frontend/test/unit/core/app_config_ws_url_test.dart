import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/core/config/app_config.dart';

/// `serverWsUrl` has to survive the http -> wss rewrite.
///
/// Two things make that easy to get wrong, and both are silent:
///
///  * The port. `Uri.port` returns 0 for a `wss` URI with no explicit port,
///    because Dart's default-port table has no entry for `wss` -- so any code
///    that rebuilds the authority from `uri.port` emits `host:0`, which fails
///    to connect for a reason that looks nothing like a port bug.
///  * The scheme. `http` must become `ws`, and `https` must become `wss`;
///    anything else would hand a plaintext socket to a TLS deployment.
void main() {
  final original = AppConfig.serverBaseUrl;

  tearDown(() => AppConfig.serverBaseUrl = original);

  test('https base becomes wss with no invented port', () {
    AppConfig.serverBaseUrl = 'https://example.com';
    expect(AppConfig.serverWsUrl, 'wss://example.com/api/v1/ws/watch-party');
    expect(AppConfig.serverWsUrl, isNot(contains(':0')));
  });

  test('http base becomes ws', () {
    AppConfig.serverBaseUrl = 'http://example.com';
    expect(AppConfig.serverWsUrl, 'ws://example.com/api/v1/ws/watch-party');
  });

  test('an explicit port is carried over', () {
    AppConfig.serverBaseUrl = 'https://example.com:8443';
    expect(AppConfig.serverWsUrl, 'wss://example.com:8443/api/v1/ws/watch-party');
  });

  test('a trailing slash does not double up', () {
    AppConfig.serverBaseUrl = 'https://example.com/';
    expect(AppConfig.serverWsUrl, 'wss://example.com/api/v1/ws/watch-party');
  });

  test('the api url keeps the base untouched', () {
    AppConfig.serverBaseUrl = 'https://example.com';
    expect(AppConfig.serverApiUrl, 'https://example.com/api/v1');
  });

  test('the exact URI connectWatchParty builds has no invented port', () {
    AppConfig.serverBaseUrl = 'https://film.oxideteam.pp.ua';
    // Same expression as kadrbox_server_service.connectWatchParty.
    final uri = Uri.parse(AppConfig.serverWsUrl).replace(
      queryParameters: {
        'room': 'ZHTFZP',
        'user_id': '1791236235734',
        'user_name': 'User',
      },
    );
    // ignore: avoid_print
    print('BUILT-URI: $uri');
    expect(uri.toString(), isNot(contains(':0')));
    expect(uri.toString(), 'wss://film.oxideteam.pp.ua/api/v1/ws/watch-party'
        '?room=ZHTFZP&user_id=1791236235734&user_name=User');
  });
}