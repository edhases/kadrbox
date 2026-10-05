// PlayerController unit tests
// Focus on pure logic: initial stream selection, quality/voiceover helpers, subtitle text, and fit cycling.

import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:media_kit/media_kit.dart';

import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/presentation/pages/player/player_controller.dart';

import '../../helpers/mock_services.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';

// Local fakes for mocktail fallback registration
class PlayableFake extends Fake implements Playable {}

class VideoTrackFake extends Fake implements VideoTrack {}

// Real Player instance used to provide a valid PlayerStream for mocked players
late Player realPlayer;

void main() {
  setUpAll(() {
    // Initialize Flutter test binding for video controller usage
    TestWidgetsFlutterBinding.ensureInitialized();

    registerFallbackValue(Duration.zero);

    // Register fakes for types used by mocktail matchers
    registerFallbackValue(PlayableFake());
    registerFallbackValue(VideoTrackFake());
  });

  group('PlayerController logic', () {
    late MockHistoryService mockHistory;
    late MockSettingsService mockSettings;
    late MockWatchPartyService mockWatchParty;

    setUp(() {
      mockHistory = MockHistoryService();
      mockSettings = MockSettingsService();
      mockWatchParty = MockWatchPartyService();

      // Default watch party state
      when(() => mockWatchParty.state).thenReturn(WatchPartyState.idle);
      when(() => mockWatchParty.isHost).thenReturn(false);
      when(() => mockWatchParty.isPlaying).thenReturn(false);
      when(() => mockWatchParty.playbackSpeed).thenReturn(1.0);

      // Default settings state to avoid nulls in controller initialization
      when(() => mockSettings.state).thenReturn(const SettingsState());
    });

    test('initializes and prefers default quality from settings', () async {
      // Given: defaultQuality = q720p
      when(
        () => mockSettings.state,
      ).thenReturn(const SettingsState(defaultQuality: DefaultQuality.q720p));

      final q1080 = StreamSource(
        url: 'https://cdn/1080.mp4',
        quality: StreamQuality.q1080p,
      );
      final q720 = StreamSource(
        url: 'https://cdn/720.mp4',
        quality: StreamQuality.q720p,
      );

      final controller = PlayerController(
        initialUrl: q1080.url,
        historyService: mockHistory,
        settingsService: mockSettings,
        watchPartyService: mockWatchParty,
        streams: [q1080, q720],
      );

      // Then: controller state should prefer 720p stream
      expect(controller.state.currentQuality, StreamQuality.q720p);
      expect(controller.state.currentUrl, q720.url);
    });

    test(
      'hasMultipleQualities and hasMultipleVoiceovers detect correctly',
      () async {
        when(() => mockSettings.state).thenReturn(const SettingsState());

        final s1 = StreamSource(
          url: 'a',
          quality: StreamQuality.q720p,
          voiceover: 'UA',
        );
        final s2 = StreamSource(
          url: 'b',
          quality: StreamQuality.q1080p,
          voiceover: 'RU',
        );

        final controller = PlayerController(
          initialUrl: s1.url,
          historyService: mockHistory,
          settingsService: mockSettings,
          watchPartyService: mockWatchParty,
          streams: [s1, s2],
        );

        expect(controller.hasMultipleQualities(), isTrue);
        expect(controller.hasMultipleVoiceovers(), isTrue);
      },
    );

    test(
      'buildSubtitleText uses quality and voiceover and falls back to provided subtitle',
      () async {
        when(() => mockSettings.state).thenReturn(const SettingsState());

        final s1 = StreamSource(
          url: 'a',
          quality: StreamQuality.q720p,
          voiceover: 'UA',
        );
        final controller = PlayerController(
          initialUrl: s1.url,
          historyService: mockHistory,
          settingsService: mockSettings,
          watchPartyService: mockWatchParty,
          streams: [s1],
        );

        final text = controller.buildSubtitleText('Fallback');
        expect(text, '720p • UA');

        // If no quality/voiceover, fallback is used
        final controller2 = PlayerController(
          initialUrl: 'x',
          historyService: mockHistory,
          settingsService: mockSettings,
          watchPartyService: mockWatchParty,
          streams: [],
        );

        final text2 = controller2.buildSubtitleText('Fallback');
        expect(text2, 'Fallback');
      },
    );

    test('cycleFit rotates through BoxFit options', () async {
      when(() => mockSettings.state).thenReturn(const SettingsState());

      final controller = PlayerController(
        initialUrl: 'x',
        historyService: mockHistory,
        settingsService: mockSettings,
        watchPartyService: mockWatchParty,
        streams: [],
      );

      final initialFit = controller.state.videoFit;
      controller.cycleFit();
      final second = controller.state.videoFit;
      expect(second != initialFit, isTrue);
      controller.cycleFit();
      controller.cycleFit();
      // After cycling through all fits we should get back to the original
      expect(controller.state.videoFit, initialFit);
    });

    // MockPlayer tests

    test('playOrPause notifies watch party when connected', () async {
      final mockPlayer = MockPlayer();
      // Provide a fake state object with 'playing' field
      final fakeState = FakePlayerState(false);
      when<dynamic>(() => mockPlayer.state).thenReturn(fakeState);
      when(() => mockPlayer.playOrPause()).thenAnswer((_) async {});
      when(() => mockPlayer.open(any())).thenAnswer((_) async {});

      when(() => mockWatchParty.state).thenReturn(WatchPartyState.connected);

      final controller = PlayerController(
        initialUrl: 'x',
        historyService: mockHistory,
        settingsService: mockSettings,
        watchPartyService: mockWatchParty,
        streams: [],
        playerFactory: () => mockPlayer,
        setupPlayerStreams: false,
      );

      // Initialize to ensure _player is created
      await controller.initialize();

      controller.playOrPause();

      // When player was not playing, watchParty.play should be called
      verify(() => mockWatchParty.play()).called(1);
    });

    test(
      'seekForward calls player.seek and notifies watch party when connected',
      () async {
        final mockPlayer = MockPlayer();
        when<dynamic>(
          () => mockPlayer.state,
        ).thenReturn(FakePlayerState(false));
        when(() => mockPlayer.seek(any())).thenAnswer((_) async {});
        when(() => mockPlayer.open(any())).thenAnswer((_) async {});

        when(() => mockWatchParty.state).thenReturn(WatchPartyState.connected);

        final controller = PlayerController(
          initialUrl: 'x',
          historyService: mockHistory,
          settingsService: mockSettings,
          watchPartyService: mockWatchParty,
          streams: [],
          playerFactory: () => mockPlayer,
          setupPlayerStreams: false,
        );

        // Initialize to ensure _player is created
        await controller.initialize();

        controller.seekForward();

        // verify player.seek called once
        verify(() => mockPlayer.seek(any())).called(1);
        // verify watchParty.seek called because state is connected
        verify(() => mockWatchParty.seek(any())).called(1);
      },
    );

    test(
      'tryReduceQuality reduces stream quality and calls onQualityReduced',
      () async {
        final mockPlayer = MockPlayer();
        when<dynamic>(
          () => mockPlayer.state,
        ).thenReturn(FakePlayerState(false));
        when(() => mockPlayer.open(any())).thenAnswer((_) async {});

        when(() => mockWatchParty.state).thenReturn(WatchPartyState.idle);
        when(() => mockSettings.state).thenReturn(const SettingsState());
        when(() => mockPlayer.seek(any())).thenAnswer((_) async {});

        final s1080 = StreamSource(url: 'u1080', quality: StreamQuality.q1080p);
        final s720 = StreamSource(url: 'u720', quality: StreamQuality.q720p);

        String? reduced;

        final controller = PlayerController(
          initialUrl: s1080.url,
          historyService: mockHistory,
          settingsService: mockSettings,
          watchPartyService: mockWatchParty,
          streams: [s1080, s720],
          playerFactory: () => mockPlayer,
          setupPlayerStreams: false,
        );

        controller.onQualityReduced = (q) {
          reduced = q;
        };

        // Initialize to ensure _player is created and open called
        await controller.initialize();

        controller.tryReduceQuality();

        // Verify player.open was invoked for switching stream and callback invoked
        final captured = verify(() => mockPlayer.open(captureAny())).captured;
        expect(
          captured.any((m) => (m as dynamic).toString().contains('u720')),
          isTrue,
        );
        expect(reduced, isNotNull);
        expect(reduced, contains('720'));
      },
    );

    test(
      'tryReduceQuality reduces video track and calls setVideoTrack/onQualityReduced',
      () async {
        final mockPlayer = MockPlayer();
        when<dynamic>(
          () => mockPlayer.state,
        ).thenReturn(FakePlayerState(false));
        when(() => mockPlayer.setVideoTrack(any())).thenAnswer((_) async {});

        when(() => mockWatchParty.state).thenReturn(WatchPartyState.idle);
        when(() => mockPlayer.open(any())).thenAnswer((_) async {});

        // VideoTrack(id, title, language, w: , h:) constructor (media_kit)
        final track1080 = VideoTrack('t1', null, null, w: 1920, h: 1080);
        final track720 = VideoTrack('t2', null, null, w: 1280, h: 720);
        final track480 = VideoTrack('t3', null, null, w: 854, h: 480);

        final controller = PlayerController(
          initialUrl: 'u1080',
          historyService: mockHistory,
          settingsService: mockSettings,
          watchPartyService: mockWatchParty,
          streams: [],
          playerFactory: () => mockPlayer,
          setupPlayerStreams: false,
        );

        String? reduced;
        controller.onQualityReduced = (q) => reduced = q;

        // Inject video tracks for test
        controller.setVideoTracksForTest([
          track1080,
          track720,
          track480,
        ], track1080);

        // Ensure player exists
        await controller.initialize();

        controller.tryReduceQuality();

        // verify player.setVideoTrack called with lower track
        verify(
          () => mockPlayer.setVideoTrack(
            any(that: predicate((t) => (t as VideoTrack).h == 720)),
          ),
        ).called(1);
        expect(reduced, isNotNull);
        expect(reduced, contains('720'));
      },
    );
  });

  // Top-level mocks & fakes are provided by test/helpers/mock_services.dart
}
