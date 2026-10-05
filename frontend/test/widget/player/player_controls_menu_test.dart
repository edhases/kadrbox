import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:media_kit/media_kit.dart' hide PlayerState;
import 'package:mocktail/mocktail.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';
import 'package:oxide_film/presentation/pages/player/player_controller.dart';
import 'package:oxide_film/presentation/pages/player/player_controls.dart';

// Mocks
class MockPlayerController extends Mock implements PlayerController {}

class MockSettingsService extends Mock implements SettingsService {}

class MockWatchPartyService extends Mock implements WatchPartyService {}

void main() {
  late MockPlayerController mockController;
  late MockSettingsService mockSettings;
  late MockWatchPartyService mockWatchParty;

  setUp(() {
    mockController = MockPlayerController();
    mockSettings = MockSettingsService();
    mockWatchParty = MockWatchPartyService();

    // Default stubs
    when(() => mockController.state).thenReturn(const PlayerState());
    when(() => mockController.title).thenReturn('Test Movie');
    when(() => mockController.buildSubtitleText(any())).thenReturn('Subtitle');
    when(
      () => mockController.positionNotifier,
    ).thenReturn(ValueNotifier(Duration.zero));

    when(() => mockController.settingsService).thenReturn(mockSettings);
    when(() => mockController.watchPartyService).thenReturn(mockWatchParty);

    when(() => mockWatchParty.state).thenReturn(WatchPartyState.idle);
    when(() => mockWatchParty.chatMessages).thenReturn([]);

    // Default capabilities
    when(() => mockController.hasMultipleQualities()).thenReturn(false);
    when(() => mockController.hasMultipleVoiceovers()).thenReturn(false);
    when(() => mockController.hasMultipleAudioTracks()).thenReturn(false);
  });

  Widget createWidgetUnderTest() {
    return MaterialApp(
      theme: ThemeData(platform: TargetPlatform.android),
      home: Scaffold(
        body: PlayerControls(
          controller: mockController,
          showControls: true,
          onToggleControls: () {},
          onToggleChat: () {},
          showChat: false,
          newChatMessages: 0,
        ),
      ),
    );
  }

  group('PlayerControls Menu Tests', () {
    testWidgets('shows Audio Track menu button when multiple tracks exist', (
      tester,
    ) async {
      // Set screen size to 550 (ExpandedMenu range: 500-600)
      tester.view.physicalSize = const Size(550, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);

      // Given multiple audio tracks
      final tracks = [AudioTrack('1', null, 'uk'), AudioTrack('2', null, 'en')];

      when(() => mockController.hasMultipleAudioTracks()).thenReturn(true);
      when(() => mockController.state).thenReturn(
        PlayerState(audioTracks: tracks, selectedAudioTrack: tracks[0]),
      );

      // When
      await tester.pumpWidget(createWidgetUnderTest());
      await tester.pumpAndSettle();

      // Then - should find audio track icon (audiotrack is the icon)
      // Note: In _ExpandedMenu (large screen) it's visible. In _CompactMenu it's in the popup.
      // Assuming mobile/compact view checks first, or large.
      // Tester defaults to 800x600 which is large/tablet-ish or desktop.
      // _ExpandedMenu is used for mobile/large (but logic differs).
      // Let's force a specific size or check for icon.

      final audioIcon = find.byIcon(Icons.audiotrack);
      expect(audioIcon, findsOneWidget);
    });

    testWidgets('Audio Track menu displays formatted language names', (
      tester,
    ) async {
      // Set screen size to 550 (ExpandedMenu range: 500-600)
      tester.view.physicalSize = const Size(550, 800);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);

      // Given
      final tracks = [
        AudioTrack('1', null, 'uk'), // Should be 'Українська'
        AudioTrack('2', null, 'en'), // Should be 'Англійська'
        AudioTrack('3', 'Custom Title', 'jp'), // Should be 'Custom Title'
      ];

      when(() => mockController.hasMultipleAudioTracks()).thenReturn(true);
      when(() => mockController.state).thenReturn(
        PlayerState(audioTracks: tracks, selectedAudioTrack: tracks[0]),
      );

      await tester.pumpWidget(createWidgetUnderTest());
      await tester.pumpAndSettle();

      // Tap the audio track button to open menu
      await tester.tap(find.byIcon(Icons.audiotrack));
      await tester.pumpAndSettle();

      // Verify text
      expect(find.text('Українська'), findsOneWidget);
      expect(find.text('Англійська'), findsOneWidget);
      expect(find.text('Custom Title'), findsOneWidget);
    });
  });
}
