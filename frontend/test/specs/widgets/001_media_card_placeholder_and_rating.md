Title: MediaCard displays placeholder when posterUrl is null and shows RatingBadge when rating is present

Given:
- A MediaItem with: posterUrl = null, providerId = 'uakino', rating = 8.5, ratingSource = 'Site', year = 2021
- UISettings in SettingsService set to show ratings and years

When:
- Pump MediaCard widget with the MediaItem

Mocks/Helpers:
- Mock SettingsService or register UISettings via GetIt to enable showRatings and showYears

Expectations:
- Placeholder widget (Icon/movie placeholder) is present instead of CachedNetworkImage
- RatingBadge is present and displays text '8.5' and uses success color (green)
- Provider badge or small label shows provider id/name as expected (e.g., 'UAKINO')

Notes:
- Also check a case where posterUrl is present to ensure CachedNetworkImage is used and memCacheHeight is set
