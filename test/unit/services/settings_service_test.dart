import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/core/l10n/app_strings.dart';
import 'package:oxide_film/data/database/app_database.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/domain/entities/ui_settings.dart';

import '../../helpers/in_memory_db.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late AppDatabase db;
  late SettingsService settingsService;

  setUp(() async {
    db = createTestAppDatabase();
    settingsService = SettingsService(db);
    // Allow initial async DB load to complete
    await Future.delayed(const Duration(milliseconds: 50));
  });

  tearDown(() async {
    settingsService.dispose();
    await db.close();
  });

  group('SettingsService Full Coverage Tests', () {
    test('Initial state loads default values', () {
      final s = settingsService.state;
      expect(s.theme, AppThemeMode.dark);
      expect(s.flutterThemeMode, ThemeMode.dark);
      expect(s.defaultQuality, DefaultQuality.auto);
      expect(s.playerType, PlayerType.internal);
      expect(s.autoPlayNext, isTrue);
      expect(s.rememberPosition, isTrue);
      expect(s.subtitleLanguage, 'uk');
    });

    test('Theme modifications update state and notify listeners', () async {
      int notifyCount = 0;
      settingsService.addListener(() => notifyCount++);

      await settingsService.setTheme(AppThemeMode.light);
      await pumpEventQueue();
      expect(settingsService.state.theme, AppThemeMode.light);
      expect(settingsService.state.flutterThemeMode, ThemeMode.light);
      expect(notifyCount, greaterThan(0));

      await settingsService.setTheme(AppThemeMode.amoled);
      await pumpEventQueue();
      expect(settingsService.state.theme, AppThemeMode.amoled);
      expect(settingsService.state.flutterThemeMode, ThemeMode.dark);

      await settingsService.setTheme(AppThemeMode.system);
      await pumpEventQueue();
      expect(settingsService.state.theme, AppThemeMode.system);
      expect(settingsService.state.flutterThemeMode, ThemeMode.system);
    });

    test('Quality and PlayerType updates', () async {
      await settingsService.setDefaultQuality(DefaultQuality.q1080p);
      await pumpEventQueue();
      expect(settingsService.state.defaultQuality, DefaultQuality.q1080p);

      await settingsService.setPlayerType(PlayerType.external);
      await pumpEventQueue();
      expect(settingsService.state.playerType, PlayerType.external);
    });

    test('Playback and subtitle settings updates', () async {
      await settingsService.setAutoPlayNext(false);
      await pumpEventQueue();
      expect(settingsService.state.autoPlayNext, isFalse);

      await settingsService.setRememberPosition(false);
      await pumpEventQueue();
      expect(settingsService.state.rememberPosition, isFalse);

      await settingsService.setSubtitleLanguage('en');
      await pumpEventQueue();
      expect(settingsService.state.subtitleLanguage, 'en');

      await settingsService.setWatchPartyName('User123');
      await pumpEventQueue();
      expect(settingsService.state.watchPartyName, 'User123');

      await settingsService.setDefaultSpeed(1.5);
      await pumpEventQueue();
      expect(settingsService.state.defaultSpeed, 1.5);

      await settingsService.setGestureControls(false);
      await pumpEventQueue();
      expect(settingsService.state.gestureControls, isFalse);

      await settingsService.setSkipIntro(true);
      await pumpEventQueue();
      expect(settingsService.state.skipIntro, isTrue);

      await settingsService.setNextEpisodeDelay(10);
      await pumpEventQueue();
      expect(settingsService.state.nextEpisodeDelay, 10);
    });

    test('UI, locale, and display customization settings', () async {
      await settingsService.setLocale(AppLocale.en);
      await pumpEventQueue();
      expect(settingsService.state.locale, AppLocale.en);

      await settingsService.setPosterSize(PosterSize.large);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.posterSize, PosterSize.large);

      await settingsService.setGridSpacing(GridSpacing.relaxed);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.gridSpacing, GridSpacing.relaxed);

      await settingsService.setAccentColor(AccentColor.teal);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.accentColor, AccentColor.teal);

      await settingsService.setCardInfoStyle(CardInfoStyle.below);
      await pumpEventQueue();
      expect(
        settingsService.state.uiSettings.cardInfoStyle,
        CardInfoStyle.below,
      );

      await settingsService.setListStyle(ListStyle.list);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.listStyle, ListStyle.list);

      await settingsService.setShowRatings(false);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.showRatings, isFalse);

      await settingsService.setShowYears(false);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.showYears, isFalse);

      await settingsService.setAnimationsEnabled(false);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.animationsEnabled, isFalse);

      await settingsService.setBlurBackgrounds(true);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.blurBackgrounds, isTrue);

      await settingsService.setGridColumns(4);
      await pumpEventQueue();
      expect(settingsService.state.uiSettings.gridColumns, 4);

      await settingsService.setDebugMode(true);
      await pumpEventQueue();
      expect(settingsService.state.debugMode, isTrue);

      await settingsService.setDeviceTypeOverride('tv');
      await pumpEventQueue();
      expect(settingsService.state.deviceTypeOverride, 'tv');

      await settingsService.setNewEpisodeNotify(true);
      await pumpEventQueue();
      expect(settingsService.state.newEpisodeNotify, isTrue);

      await settingsService.setUpdateNotify(false);
      await pumpEventQueue();
      expect(settingsService.state.updateNotify, isFalse);

      await settingsService.setFullscreenTransitionDelay(250);
      await pumpEventQueue();
      expect(settingsService.state.fullscreenTransitionDelay, 250);

      await settingsService.setDownloadPath('/custom/downloads');
      await pumpEventQueue();
      expect(settingsService.state.downloadPath, '/custom/downloads');

      await settingsService.setOnlyWifiDownload(false);
      await pumpEventQueue();
      expect(settingsService.state.onlyWifiDownload, isFalse);
    });

    test('Provider enable and search preferences', () async {
      await settingsService.setProviderEnabled('uakino', true);
      await settingsService.setProviderEnabled('lavakino', false);
      await pumpEventQueue();

      expect(settingsService.isProviderEnabled('uakino'), isTrue);
      expect(settingsService.isProviderEnabled('lavakino'), isFalse);
      expect(
        settingsService.isProviderEnabled('nonexistent'),
        isTrue,
      ); // default

      // Search enabled preference per provider
      await settingsService.setSearchEnabledForProvider('uakino', false);
      expect(settingsService.isSearchEnabledForProvider('uakino'), isFalse);
      expect(
        settingsService.isSearchEnabledForProvider('lavakino'),
        isFalse,
      ); // default is false for separate
    });

    test('resetAllSettings resets settings back to default', () async {
      await settingsService.setTheme(AppThemeMode.light);
      await settingsService.setDefaultQuality(DefaultQuality.q480p);
      await settingsService.setPlayerType(PlayerType.external);
      await settingsService.setAutoPlayNext(false);

      await settingsService.resetAllSettings();
      // Allow async load to complete
      await Future.delayed(const Duration(milliseconds: 50));

      expect(settingsService.state.theme, AppThemeMode.dark);
      expect(settingsService.state.defaultQuality, DefaultQuality.auto);
      expect(settingsService.state.playerType, PlayerType.internal);
      expect(settingsService.state.autoPlayNext, isTrue);
    });

    test('Enums display names and fromString helpers', () {
      expect(AppThemeMode.dark.displayName, 'Темна');
      expect(AppThemeMode.amoled.displayName, 'AMOLED');
      expect(AppThemeMode.light.displayName, 'Світла');
      expect(AppThemeMode.system.displayName, 'Системна');
      expect(AppThemeMode.fromString('light'), AppThemeMode.light);
      expect(AppThemeMode.fromString('unknown'), AppThemeMode.dark);

      expect(DefaultQuality.auto.displayName, 'Авто');
      expect(DefaultQuality.q480p.displayName, '480p');
      expect(DefaultQuality.q720p.displayName, '720p');
      expect(DefaultQuality.q1080p.displayName, '1080p');
      expect(DefaultQuality.q1440p.displayName, '1440p+');
      expect(DefaultQuality.fromString('q1080p'), DefaultQuality.q1080p);
      expect(DefaultQuality.fromString(null), DefaultQuality.auto);

      expect(PlayerType.internal.displayName, 'Вбудований');
      expect(PlayerType.external.displayName, 'Зовнішній');
      expect(PlayerType.fromString('external'), PlayerType.external);
      expect(PlayerType.fromString(null), PlayerType.internal);
    });
  });

  // The login screen used to offer "merge your local data into the account" on
  // every single sign-in, because its only condition was "the local tables are
  // non-empty". After a successful merge those rows ARE the account's data, so
  // the condition stayed true forever and the user was repeatedly asked to merge
  // their own history with their own account.
  group('Local data reconciliation marker', () {
    const me = 'user-a';
    const other = 'user-b';

    test('is unset on a fresh device', () async {
      expect(await settingsService.localDataReconciledWith, isNull);
    });

    test(
      'offers the merge when local data has never been reconciled',
      () async {
        expect(
          await settingsService.shouldOfferLocalDataMerge(
            userId: me,
            historyCount: 82,
            favoritesCount: 1,
          ),
          isTrue,
        );
      },
    );

    test('does NOT offer it again for the same account', () async {
      await settingsService.markLocalDataReconciled(me);

      expect(
        await settingsService.shouldOfferLocalDataMerge(
          userId: me,
          historyCount: 82,
          favoritesCount: 1,
        ),
        isFalse,
        reason:
            'the same account must not be asked to merge its own data twice',
      );
    });

    test('still offers it for a different account', () async {
      await settingsService.markLocalDataReconciled(me);

      // Device-local data has not been attributed to this account, so this is a
      // real decision rather than a repeat of the previous one.
      expect(
        await settingsService.shouldOfferLocalDataMerge(
          userId: other,
          historyCount: 82,
          favoritesCount: 1,
        ),
        isTrue,
      );
    });

    test('never offers it when there is nothing to merge', () async {
      expect(
        await settingsService.shouldOfferLocalDataMerge(
          userId: me,
          historyCount: 0,
          favoritesCount: 0,
        ),
        isFalse,
      );
    });

    test('favorites alone are enough to trigger the offer', () async {
      expect(
        await settingsService.shouldOfferLocalDataMerge(
          userId: me,
          historyCount: 0,
          favoritesCount: 3,
        ),
        isTrue,
      );
    });

    test('the marker survives an unrelated settings change', () async {
      await settingsService.markLocalDataReconciled(me);
      await settingsService.setTheme(AppThemeMode.light);

      expect(await settingsService.localDataReconciledWith, me);
    });

    test('resetAllSettings clears the marker, so the offer returns', () async {
      await settingsService.markLocalDataReconciled(me);
      await settingsService.resetAllSettings();
      await Future.delayed(const Duration(milliseconds: 50));

      expect(await settingsService.localDataReconciledWith, isNull);
      expect(
        await settingsService.shouldOfferLocalDataMerge(
          userId: me,
          historyCount: 82,
          favoritesCount: 1,
        ),
        isTrue,
      );
    });
  });
}
