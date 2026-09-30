import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/data/models/provider_catalog.dart';
import 'package:oxide_film/data/providers/server_backed_provider.dart';
import 'package:oxide_film/domain/entities/entities.dart';

void main() {
  group('ServerBackedProvider', () {
    late ServerBackedProvider provider;

    setUp(() {
      provider = ServerBackedProvider(
        const ProviderCatalogEntry(
          id: 'uakino',
          name: 'UAKino',
          baseUrl: 'https://uakino.biz',
          showOnHome: true,
          hasFixedStreams: false,
          contentTypes: ['movie', 'series', 'cartoon', 'anime'],
          searchEnabledDefault: true,
          enabled: true,
          healthy: true,
        ),
      );
    });

    test('should have correct metadata', () {
      expect(provider.id, 'uakino');
      expect(provider.name, 'UAKino');
      expect(provider.baseUrl, 'https://uakino.biz');
      expect(provider.supportedTypes, contains(ContentType.movie));
      expect(provider.supportedTypes, contains(ContentType.series));
      expect(provider.supportedTypes, contains(ContentType.cartoon));
      expect(provider.supportedTypes, contains(ContentType.anime));
    });

    test('should be enabled by default', () {
      expect(provider.isEnabled, isTrue);
    });
  });
}
