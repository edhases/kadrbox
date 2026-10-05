Title: Initialize and resume playback position based on user default quality

Given:
- SettingsService.state.defaultQuality == DefaultQuality.q720p
- HistoryService.getLastPosition(mediaId: 'm123', providerId: 'uakino') returns Duration(minutes:15)
- PlayerController.initialUrl == URL of q1080p stream in provided streams list
- streams contains at least two StreamSource entries: q1080p and q720p

When:
- A PlayerController is constructed with initialUrl, streams, historyService and settingsService and initialize() is called

Mocks:
- MockSettingsService.defaultQuality -> DefaultQuality.q720p
- MockHistoryService.getLastPosition('m123','uakino') -> Duration(minutes:15)
- FakePlayer (or mock Player) monitors seek calls

Expectations (assertions):
- controller.state.currentQuality == StreamQuality.q720p
- controller.state.currentUrl == url of q720p stream (exact string)
- FakePlayer.seek called once with Duration(minutes:15) or controller.onPositionResumed invoked with Duration(15m)

Notes/Edge cases:
- If defaultQuality == auto, expect currentQuality == null and initialUrl preserved
- Use FakeAsync to ensure no timer side-effects interfere

Fixture:
- none required. This spec drives PlayerController with a synthetic
  StreamSource list; it never resolves streams through the repository flow, so
  it does not need an HTML fixture. The catalog-page fixture that does exist for
  the uakino scraper is server/internal/provider/testdata/uakino_catalog.html,
  and it is a catalog card list with no player or streams in it, so it cannot
  serve this spec either.
