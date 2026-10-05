Title: Keyboard shortcuts (space, enter, arrows, escape) invoke player actions

Given:
- PlayerPage is pumped into WidgetTester and has focus
- Controller is a mock or fake that tracks calls to: playOrPause(), seekForward(), seekBackward(), toggleFullscreen()

When:
- Simulate KeyDownEvent for LogicalKeyboardKey.space, LogicalKeyboardKey.enter, LogicalKeyboardKey.arrowRight, LogicalKeyboardKey.arrowLeft, LogicalKeyboardKey.escape

Mocks/Helpers:
- Mock PlayerController with spy methods
- A FocusNode assigned to PlayerPage so KeyboardListener receives events

Expectations:
- space/enter -> playOrPause called once
- arrowRight -> seekForward called once
- arrowLeft -> seekBackward called once
- escape -> if state.isFullscreen true -> toggleFullscreen called; else -> Navigator.pop was invoked (or page closed)

Notes:
- Use WidgetTester.sendKeyEvent / sendKeyDownEvent utilities
