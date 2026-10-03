import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:oxide_film/data/services/user_agent_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('UserAgentService', () {
    late SharedPreferences prefs;

    setUp(() async {
      SharedPreferences.setMockInitialValues({});
      prefs = await SharedPreferences.getInstance();
    });

    test(
      'successfully parses user agents from useragents.me with new HTML markup',
      () async {
        const mockHtml = '''
<!DOCTYPE html>
<html>
<body>
  <table>
    <tr><td><textarea class="form-control font-monospace" rows="2" readonly style="min-width: 420px;" wrap="soft">Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36</textarea></td></tr>
    <tr><td><textarea class="form-control font-monospace" rows="2" readonly style="min-width: 420px;" wrap="soft">Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36</textarea></td></tr>
    <tr><td><textarea class="form-control font-monospace" rows="2" readonly style="min-width: 420px;" wrap="soft">Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0</textarea></td></tr>
    <tr><td><textarea class="form-control font-monospace" rows="2" readonly style="min-width: 420px;" wrap="soft">Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36</textarea></td></tr>
    <tr><td><textarea class="form-control font-monospace" rows="2" readonly style="min-width: 420px;" wrap="soft">Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15</textarea></td></tr>
    <tr><td><textarea class="form-control font-monospace" rows="2" readonly style="min-width: 420px;" wrap="soft">Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0</textarea></td></tr>
  </table>
</body>
</html>
''';

        final mockClient = MockClient((request) async {
          if (request.url.toString().contains('willshouse.com')) {
            // Primary source returns 403 (like Cloudflare block)
            return http.Response('Forbidden', 403);
          }
          if (request.url.toString().contains('useragents.me')) {
            return http.Response(mockHtml, 200);
          }
          return http.Response('Not Found', 404);
        });

        final service = UserAgentService(client: mockClient, prefs: prefs);
        await service.fetchLatestUserAgents();

        expect(service.userAgents.length, greaterThanOrEqualTo(6));
        final chromeUa = service.getChromeUserAgent();
        expect(chromeUa, contains('Chrome'));
        expect(chromeUa, isNot(contains('Mobile')));
      },
    );

    test('supports fallback UAs when network fails completely', () {
      final service = UserAgentService.fallback();
      final ua = service.getRandomUA();
      expect(ua, isNotEmpty);
      expect(ua, startsWith('Mozilla/5.0'));

      final chromeUa = service.getChromeUserAgent();
      expect(chromeUa, contains('Chrome'));
      expect(chromeUa, isNot(contains('Mobile')));
    });
  });
}
