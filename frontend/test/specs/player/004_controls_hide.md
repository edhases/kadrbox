Title: Controls automatically hide after 4 seconds when player is playing

Given:
- PlayerController.state.isPlaying == true
- PlayerPage is shown and controls are currently visible (_showControls == true)

When:
- Advance fake time by 4 seconds (FakeAsync) or wait real time in test harness

Mocks/Helpers:
- Use FakeAsync to control timers
- Mock PlayerController with isPlaying true

Expectations:
- After 4 seconds, controls are hidden: _showControls == false and UI no longer displays controls widgets
- If user moves mouse or presses a key before 4s, _showControls resets and hides again after another 4s

Notes:
- Avoid real timers; use FakeAsync or override Duration in test to speed up
