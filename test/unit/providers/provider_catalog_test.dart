import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/models/provider_catalog.dart';
import 'package:oxide_film/data/providers/provider_registry.dart';
import 'package:oxide_film/data/providers/server_backed_provider.dart';
import 'package:oxide_film/domain/entities/media_item.dart';

const _catalogJson = {
  'version': 7,
  'providers': [
    {
      'id': 'uakino',
      'name': 'UAKino',
      'baseUrl': 'https://uakino.best',
      'showOnHome': true,
      'hasFixedStreams': false,
      'contentTypes': ['movie', 'series'],
      'searchEnabledDefault': true,
      'enabled': true,
      'healthy': true,
    },
    {
      'id': 'newprov',
      'name': 'NewProv',
      'baseUrl': 'https://new.example',
      'showOnHome': false,
      'hasFixedStreams': true,
      'contentTypes': ['movie'],
      'searchEnabledDefault': false,
      'enabled': false,
      'healthy': false,
    },
  ],
};

void main() {
  group('ProviderCatalog parsing', () {
    test('parses version and entries', () {
      final catalog = ProviderCatalog.fromJson(_catalogJson);

      expect(catalog.version, 7);
      expect(catalog.providers, hasLength(2));

      final uakino = catalog.providers.first;
      expect(uakino.id, 'uakino');
      expect(uakino.showOnHome, isTrue);
      expect(uakino.enabled, isTrue);
      expect(uakino.supportedTypes,
          containsAll([ContentType.movie, ContentType.series]));
    });

    test('skips entries without id and tolerates missing fields', () {
      final catalog = ProviderCatalog.fromJson({
        'providers': [
          {'name': 'NoId'},
          {'id': 'x'},
        ],
      });

      expect(catalog.providers, hasLength(1));
      expect(catalog.providers.first.id, 'x');
      expect(catalog.providers.first.showOnHome, isTrue);
      expect(catalog.providers.first.enabled, isTrue);
    });

    test('round-trips through json', () {
      final catalog = ProviderCatalog.fromJson(_catalogJson);
      final restored = ProviderCatalog.fromJson(catalog.toJson());

      expect(restored.version, catalog.version);
      expect(restored.providers.map((e) => e.id),
          catalog.providers.map((e) => e.id));
    });
  });

  group('ProviderRegistry backend overrides', () {
    test('kill-switch excludes provider from enabled', () {
      final registry = ProviderRegistry();
      final catalog = ProviderCatalog.fromJson(_catalogJson);
      registry.register(
        ServerBackedProvider(
          catalog.providers.firstWhere((e) => e.id == 'newprov'),
        ),
      );

      // No SettingsService in GetIt -> falls back to provider.isEnabled (true).
      expect(registry.enabled.map((p) => p.id), contains('newprov'));

      registry.applyBackendCatalog(
        enabled: {'newprov': false},
        showOnHome: {'newprov': false},
        fixedStreams: {'newprov': true},
      );

      expect(registry.isBackendEnabled('newprov'), isFalse);
      expect(registry.enabled.map((p) => p.id), isNot(contains('newprov')));
      expect(registry.showsOnHome(registry.getById('newprov')!), isFalse);
      expect(registry.hasFixed(registry.getById('newprov')!), isTrue);
    });

    test('unknown ids default to backend-enabled', () {
      final registry = ProviderRegistry();
      expect(registry.isBackendEnabled('ghost'), isTrue);
    });
  });
}
