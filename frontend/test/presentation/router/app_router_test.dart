import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/presentation/router/app_router.dart';

void main() {
  group('AppRouter.safeDecodeMediaId', () {
    test('passes through plain unencoded strings', () {
      expect(AppRouter.safeDecodeMediaId('12345'), equals('12345'));
      expect(
        AppRouter.safeDecodeMediaId('movie-title-slug'),
        equals('movie-title-slug'),
      );
    });

    test('decodes valid percent-encoded components', () {
      expect(
        AppRouter.safeDecodeMediaId('%7B%22source%22%3A%22src_e%22%7D'),
        equals('{"source":"src_e"}'),
      );
      expect(
        AppRouter.safeDecodeMediaId('hello%20world'),
        equals('hello world'),
      );
    });

    test('tolerates strings with unescaped percent symbols without throwing', () {
      // In standard Dart, Uri.decodeComponent("100% Вовк") throws FormatException:
      // "Illegal percent encoding in URI". safeDecodeMediaId must handle this gracefully.
      expect(AppRouter.safeDecodeMediaId('100% Вовк'), equals('100% Вовк'));
      expect(AppRouter.safeDecodeMediaId('%'), equals('%'));
      expect(AppRouter.safeDecodeMediaId('test%2'), equals('test%2'));
      expect(AppRouter.safeDecodeMediaId('foo%ZZbar'), equals('foo%ZZbar'));
    });

    test('tolerates already decoded JSON strings containing percent', () {
      const jsonPayload =
          '{"source":"src_f","ref":{"id":123},"title":"100% Pass"}';
      expect(AppRouter.safeDecodeMediaId(jsonPayload), equals(jsonPayload));
    });
  });
}
