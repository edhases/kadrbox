import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:kadrbox/core/network/api_client.dart';

class MockDio extends Mock implements Dio {}

/// A body the backend actually sent in production: a catalogue source answers
/// popular with `200` and JSON `null`, which Dio surfaces as a null body. These
/// tests pin the normalisation contract so a source that went quiet stays an
/// empty catalogue instead of a crash.
void main() {
  late MockDio dio;
  late ApiClient api;

  Response<dynamic> responseWith(Object? data) => Response<dynamic>(
    requestOptions: RequestOptions(path: '/content/popular'),
    statusCode: 200,
    data: data,
  );

  setUp(() {
    dio = MockDio();
    api = ApiClient(dio: dio);
  });

  Future<void> stubGet(Object? data) async {
    when(
      () => dio.get(
        any(),
        queryParameters: any(named: 'queryParameters'),
        options: any(named: 'options'),
      ),
    ).thenAnswer((_) async => responseWith(data));
  }

  group('getJsonList', () {
    test('null body becomes an empty list, not a crash', () async {
      await stubGet(null);

      expect(await api.getJsonList('/content/popular'), isEmpty);
    });

    test('a non-list body becomes an empty list', () async {
      await stubGet({'unexpected': 'map'});

      expect(await api.getJsonList('/content/popular'), isEmpty);
    });

    test('a real list passes through untouched', () async {
      await stubGet([
        {'title': 'A'},
        {'title': 'B'},
      ]);

      final list = await api.getJsonList('/content/popular');

      expect(list, hasLength(2));
    });
  });

  group('getJson', () {
    test('a non-map body becomes an empty map', () async {
      await stubGet([
        {'title': 'A'},
      ]);

      expect(await api.getJson('/content/details'), isEmpty);
    });
  });

  group('getRawJson', () {
    test('passes null through so the caller can decide', () async {
      await stubGet(null);

      expect(await api.getRawJson('/content/search'), isNull);
    });
  });
}
