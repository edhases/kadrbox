import 'package:drift/drift.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/database/app_database.dart';
import 'package:oxide_film/data/database/dao/settings_dao.dart';

import '../../helpers/in_memory_db.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('Database Initialization & Migration Tests', () {
    test('Default settings are populated on onCreate', () async {
      final db = createTestAppDatabase();
      final settingsDao = SettingsDao(db);

      // Verify defaults
      expect(await settingsDao.getSetting('theme'), 'dark');
      expect(await settingsDao.getSetting('default_quality'), 'auto');
      expect(await settingsDao.getSetting('player_type'), 'internal');
      expect(await settingsDao.getSetting('auto_play_next'), 'true');
      expect(await settingsDao.getSetting('remember_position'), 'true');
      expect(await settingsDao.getSetting('subtitle_language'), 'uk');

      await db.close();
    });

    test('Migration step < 10 purges hdrezka data cleanly', () async {
      final db = createTestAppDatabase();

      // Seed rows with hdrezka
      await db
          .into(db.enabledProviders)
          .insert(
            EnabledProvidersCompanion.insert(providerId: 'hdrezka'),
            mode: InsertMode.insertOrIgnore,
          );
      await db
          .into(db.favorites)
          .insert(
            FavoritesCompanion.insert(
              mediaId: 'rezka1',
              providerId: 'hdrezka',
              title: 'Rezka Movie',
              mediaType: 'movie',
            ),
            mode: InsertMode.insertOrIgnore,
          );
      await db
          .into(db.watchHistory)
          .insert(
            WatchHistoryCompanion.insert(
              mediaId: 'rezka1',
              providerId: 'hdrezka',
              title: 'Rezka History',
              mediaType: 'movie',
            ),
            mode: InsertMode.insertOrIgnore,
          );
      await db
          .into(db.storedMediaItems)
          .insert(
            StoredMediaItemsCompanion.insert(
              id: 'rezka1',
              providerId: 'hdrezka',
              title: 'Rezka Cache',
              mediaType: 'movie',
            ),
            mode: InsertMode.insertOrReplace,
          );

      // Verify row exists before upgrade logic
      var favs = await db.select(db.favorites).get();
      expect(favs.any((f) => f.providerId == 'hdrezka'), isTrue);

      // Execute migration callback for from=9, to=10
      final migrator = db.createMigrator();
      await db.migration.onUpgrade(migrator, 9, 10);

      // Verify hdrezka rows are wiped out completely
      favs = await db.select(db.favorites).get();
      expect(favs.any((f) => f.providerId == 'hdrezka'), isFalse);

      final hist = await db.select(db.watchHistory).get();
      expect(hist.any((h) => h.providerId == 'hdrezka'), isFalse);

      final provs = await db.select(db.enabledProviders).get();
      expect(provs.any((p) => p.providerId == 'hdrezka'), isFalse);

      final cached = await db.select(db.storedMediaItems).get();
      expect(cached.any((c) => c.providerId == 'hdrezka'), isFalse);

      await db.close();
    });
  });
}
