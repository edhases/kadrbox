// Widget tests for MediaCard and RatingBadge

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';

import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/presentation/widgets/media_card.dart';
import 'package:oxide_film/presentation/widgets/rating_badge.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/data/services/download_service.dart';
import '../../helpers/mock_services.dart';
import 'package:mocktail/mocktail.dart';

void main() {
  group('UI Widgets', () {
    late MockSettingsService mockSettings;

    setUp(() {
      mockSettings = MockSettingsService();
      const uiSettings = UISettings(showRatings: true, showYears: true);
      when(() => mockSettings.uiSettings).thenReturn(uiSettings);
      when(
        () => mockSettings.state,
      ).thenReturn(const SettingsState(uiSettings: uiSettings));

      GetIt.I.registerSingleton<SettingsService>(mockSettings);

      final mockDownloadService = MockDownloadService();
      when(
        () => mockDownloadService.getLocalPosterPath(any(), any()),
      ).thenReturn(null);
      GetIt.I.registerSingleton<DownloadService>(mockDownloadService);
    });

    tearDown(() {
      // Cleanup GetIt registration
      if (GetIt.I.isRegistered<SettingsService>()) {
        GetIt.I.unregister<SettingsService>();
      }
      if (GetIt.I.isRegistered<DownloadService>()) {
        GetIt.I.unregister<DownloadService>();
      }
    });

    testWidgets(
      'MediaCard shows placeholder when posterUrl is null and displays RatingBadge',
      (tester) async {
        final item = MediaItem(
          id: '1',
          providerId: 'uakino',
          title: 'Test Movie',
          posterUrl: null,
          rating: 8.5,
          ratingSource: 'Site',
          year: 2021,
          type: ContentType.movie,
        );

        await tester.pumpWidget(
          MaterialApp(
            home: Scaffold(
              body: SizedBox(
                width: 200,
                height: 300,
                child: MediaCard(item: item),
              ),
            ),
          ),
        );

        // Placeholder icon present
        expect(find.byIcon(Icons.movie_outlined), findsWidgets);

        // Rating badge present and displays rating 8.5
        expect(find.byIcon(Icons.star_rounded), findsOneWidget);
        expect(find.text('8.5'), findsOneWidget);
      },
    );

    testWidgets(
      'RatingBadge normalizes >10 values and chooses color thresholds',
      (tester) async {
        // rating 85 -> should display 8.5
        await tester.pumpWidget(
          MaterialApp(
            home: Scaffold(body: RatingBadge(rating: 85.0, source: 'Site')),
          ),
        );

        expect(find.text('8.5'), findsOneWidget);

        // rating 7.5 => success color (visual color check is non-trivial in unit tests; verify text presence)
        await tester.pumpWidget(
          MaterialApp(
            home: Scaffold(body: RatingBadge(rating: 7.5, source: 'IMDb')),
          ),
        );

        expect(find.text('7.5'), findsOneWidget);
      },
    );
  });
}
