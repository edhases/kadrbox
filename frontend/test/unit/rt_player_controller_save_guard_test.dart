// Runtime tests for the 10-second periodic progress saver in PlayerController.
//
// The defect: `Timer.periodic` does not wait for its callback and `_saveProgress`
// awaits a Drift write plus a cloud sync whose timeouts run to tens of seconds,
// so ticks overlapped. An out-of-order write could land an OLDER positionMs over
// a NEWER one. The dispose-time save must survive regardless.

import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:media_kit/media_kit.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';
import 'package:oxide_film/presentation/pages/player/player_controller.dart';
import 'package:wakelock_plus_platform_interface/wakelock_plus_platform_interface.dart';

import '../helpers/mock_services.dart';

class PlayableFake extends Fake implements Playable {}

/// `PlayerController.dispose()` toggles the wakelock; without a fake platform the
/// pigeon channel throws and surfaces as an unhandled async error.
class _FakeWakelockPlusPlatform extends WakelockPlusPlatformInterface {
  bool _enabled = false;

  @override
  Future<bool> get enabled async => _enabled;

  @override
  Future<void> toggle({required bool enable}) async {
    _enabled = enable;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  WakelockPlusPlatformInterface.instance = _FakeWakelockPlusPlatform();

  setUpAll(() {
    registerFallbackValue(Duration.zero);
    registerFallbackValue(PlayableFake());
  });

  late MockHistoryService history;
  late MockSettingsService settings;
  late MockWatchPartyService watchParty;
  late MockPlayer player;
  late PlayerController controller;

  PlayerController build() {
    final c = PlayerController(
      initialUrl: 'https://cdn/video.mkv',
      mediaId: 'm1',
      providerId: 'uakino',
      title: 'Movie',
      historyService: history,
      settingsService: settings,
      watchPartyService: watchParty,
      streams: const [],
      playerFactory: () => player,
      setupPlayerStreams: false,
    );
    return c;
  }

  setUp(() async {
    history = MockHistoryService();
    settings = MockSettingsService();
    watchParty = MockWatchPartyService();
    player = MockPlayer();

    when(() => player.open(any())).thenAnswer((_) async {});
    when(() => player.stop()).thenAnswer((_) async {});
    when(() => player.dispose()).thenAnswer((_) async {});
    when(() => player.playOrPause()).thenAnswer((_) async {});
    when<dynamic>(() => player.state).thenReturn(FakePlayerState(false));
    when(
      () => history.getLastPosition(any(), any()),
    ).thenAnswer((_) async => null);

    when(() => watchParty.state).thenReturn(WatchPartyState.idle);
    when(() => watchParty.isHost).thenReturn(false);
    when(() => watchParty.isPlaying).thenReturn(false);
    when(() => watchParty.playbackSpeed).thenReturn(1.0);
    when(() => settings.state).thenReturn(const SettingsState());

    controller = build();
    await controller.initialize();
    controller.setPlaybackForTest(
      position: const Duration(seconds: 42),
      duration: const Duration(minutes: 120),
    );
  });

  tearDown(() {
    if (!controller.disposedForTest) controller.dispose();
  });

  group('periodic progress saver single-flight guard', () {
    test('overlapping ticks run the save body once', () async {
      final gate = Completer<void>();
      var calls = 0;
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((_) async {
        calls++;
        await gate.future;
      });

      // Ten "ticks" fired back to back, the first one still in flight.
      for (var i = 0; i < 10; i++) {
        unawaited(controller.tickProgressSaveForTest());
      }
      await Future<void>.delayed(Duration.zero);

      expect(calls, 1, reason: 'overlapping ticks must be de-duplicated');
      expect(controller.saveInFlightForTest, isTrue);

      gate.complete();
      await Future<void>.delayed(const Duration(milliseconds: 10));

      expect(calls, 1);
      expect(controller.saveInFlightForTest, isFalse);
    });

    test('a tick after the previous one finished saves again', () async {
      var calls = 0;
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((_) async => calls++);

      await controller.tickProgressSaveForTest();
      await controller.tickProgressSaveForTest();
      await controller.tickProgressSaveForTest();

      expect(calls, 3);
    });

    test('the newest position is the one persisted', () async {
      final saved = <Duration>[];
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((invocation) async {
        final named = invocation.namedArguments;
        saved.add(named[#position]! as Duration);
      });

      controller.setPlaybackForTest(
        position: const Duration(seconds: 500),
        duration: const Duration(minutes: 120),
      );
      await controller.tickProgressSaveForTest();

      expect(saved.single, const Duration(seconds: 500));
    });

    test('ticks after dispose are dropped', () async {
      var calls = 0;
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((_) async => calls++);

      await controller.tickProgressSaveForTest();
      expect(calls, 1);

      controller.dispose();
      await controller.tickProgressSaveForTest();
      // Only the dispose-time save (which bypasses the guard) ran after.
      expect(calls, 2);
    });
  });

  group('dispose-time save', () {
    test('is not dropped even while a periodic save is in flight', () async {
      final gate = Completer<void>();
      var calls = 0;
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((_) async {
        calls++;
        if (calls == 1) await gate.future;
      });

      // Park the guard in the in-flight state.
      unawaited(controller.tickProgressSaveForTest());
      await Future<void>.delayed(Duration.zero);
      expect(calls, 1);
      expect(controller.saveInFlightForTest, isTrue);

      // dispose() must bypass the guard so the position is never lost.
      controller.dispose();
      await Future<void>.delayed(Duration.zero);

      expect(calls, 2);

      gate.complete();
      await Future<void>.delayed(const Duration(milliseconds: 10));
      expect(calls, 2);
    });

    test('fires a final save on a clean close', () async {
      var calls = 0;
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((_) async => calls++);

      expect(calls, 0);
      controller.dispose();
      await Future<void>.delayed(Duration.zero);
      expect(calls, 1);
    });

    test('skips the save when there is nothing to key on', () async {
      var calls = 0;
      when(
        () => history.saveProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          mediaType: any(named: 'mediaType'),
          position: any(named: 'position'),
          duration: any(named: 'duration'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
        ),
      ).thenAnswer((_) async => calls++);

      final noId = PlayerController(
        initialUrl: 'x',
        historyService: history,
        settingsService: settings,
        watchPartyService: watchParty,
        streams: const [],
        playerFactory: () => player,
        setupPlayerStreams: false,
      );
      await noId.initialize();
      noId.setPlaybackForTest(
        position: const Duration(seconds: 5),
        duration: const Duration(minutes: 10),
      );
      noId.dispose();
      await Future<void>.delayed(Duration.zero);

      expect(calls, 0);
    });
  });
}
