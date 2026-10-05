import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/services/catalog_client.dart';

/// `catalog_id` is the client's storage namespace: every stored reference is
/// scoped `<catalog_id>:<item_id>`. When a server does not declare one, the
/// client derives it from the base URL -- and that derivation has to hold three
/// properties at once, or it silently destroys a user's history:
///
///   * stable, so the same server keeps the same namespace across restarts;
///   * distinct per server, so two catalogues never merge their storage;
///   * insensitive to spelling that does not change *which* server it is.
///
/// The derivation is a pure function of the URL string, so it needs no server.
void main() {
  group('derived catalog_id', () {
    /// The derivation is a pure function of the URL string, so it can be tested
    /// without any server at all.
    String derive(String url) =>
        CatalogClient(baseUrl: url).derivedCatalogId;

    test('is stable across identical URLs', () {
      expect(derive('https://a.example/catalog'), derive('https://a.example/catalog'));
    });

    test('ignores a trailing slash', () {
      expect(derive('https://a.example/cat'), derive('https://a.example/cat/'));
      expect(derive('https://a.example/cat'), derive('https://a.example/cat///'));
    });

    test('ignores scheme and host case', () {
      expect(derive('https://A.Example/cat'), derive('https://a.example/cat'));
      expect(derive('HTTPS://a.example/cat'), derive('https://a.example/cat'));
    });

    test('ignores an explicit default port', () {
      expect(derive('https://a.example/cat'), derive('https://a.example:443/cat'));
      expect(derive('http://a.example/cat'), derive('http://a.example:80/cat'));
    });

    test('distinguishes two catalogues on one host', () {
      // This is the case the earlier host-derived id got wrong.
      expect(
        derive('https://a.example/first'),
        isNot(derive('https://a.example/second')),
      );
    });

    test('distinguishes a non-default port', () {
      expect(derive('https://a.example/cat'), isNot(derive('https://a.example:8443/cat')));
    });

    test('distinguishes scheme, so http and https do not share history', () {
      expect(derive('http://a.example/cat'), isNot(derive('https://a.example/cat')));
    });

    test('keeps the host readable as a prefix', () {
      expect(derive('https://media.example/cat'), startsWith('media.example-'));
    });

    test('is safe to use as a storage key component', () {
      for (final url in [
        'https://a.example/cat',
        'http://127.0.0.1:8080',
        'https://xn--80ak6aa92e.example/cat',
      ]) {
        expect(derive(url), matches(RegExp(r'^[A-Za-z0-9._:-]+$')), reason: url);
      }
    });

    test('survives an unparseable URL without throwing', () {
      expect(derive('not a url'), isNotEmpty);
      expect(derive(''), isNotEmpty);
    });
  });
}