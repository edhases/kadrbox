import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/presentation/widgets/open_stream_dialog.dart';

/// Holds whatever [OpenStreamDialog] popped, readable after the route is gone.
class _Popped {
  ({StreamDialogAction action, String url})? value;
}

/// Opens [OpenStreamDialog] the way production code does: from a button, via
/// `showDialog`, awaited in the caller.
Future<_Popped> openStreamDialog(WidgetTester tester) async {
  final popped = _Popped();

  await tester.pumpWidget(
    MaterialApp(
      home: Builder(
        builder: (context) => Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () async {
                popped.value = await showDialog<
                  ({StreamDialogAction action, String url})
                >(
                  context: context,
                  builder: (_) => const OpenStreamDialog(),
                );
              },
              child: const Text('open'),
            ),
          ),
        ),
      ),
    ),
  );

  await tester.tap(find.text('open'));
  await tester.pumpAndSettle();
  expect(find.byType(OpenStreamDialog), findsOneWidget);

  return popped;
}

/// Regression tests for the "open a network stream" dialog crash.
///
/// The bug: the controller was created in the caller and disposed as soon as
/// `showDialog` returned. `Navigator.pop` completes that future while the dialog
/// route is still animating out, so the still-mounted [TextField] was listening
/// to an already-disposed controller. Flutter threw from inside the exit
/// transition's `didUpdateWidget` ("A TextEditingController was used after being
/// disposed") and the element tree came apart behind it -- `_dependents.isEmpty`
/// on an [InheritedElement], then "Tried to build dirty widget in the wrong build
/// scope" naming the FilledButton, and a red debug screen over a stream that was
/// in fact playing.
void main() {
  // Exceptions raised during pump/pumpAndSettle go to FlutterError.onError
  // instead of failing the test, so the old code printed a red screen and still
  // went green. Collecting them here is what makes these tests bite.
  final errors = <FlutterErrorDetails>[];
  final previousOnError = FlutterError.onError;

  setUp(() {
    errors.clear();
    FlutterError.onError = (details) => errors.add(details);
  });

  tearDown(() => FlutterError.onError = previousOnError);

  void expectNoFlutterErrors() {
    expect(
      errors,
      isEmpty,
      reason: errors.isEmpty
          ? ''
          : 'The dialog threw while animating out:\n'
                '${errors.first.exceptionAsString()}',
    );
  }

  testWidgets('cancelling runs the exit animation without throwing',
      (tester) async {
    await openStreamDialog(tester);

    await tester.tap(find.text('Скасувати'));
    await tester.pump();
    // The dangerous window: the route is popped, so the dialog's future has
    // already completed, yet the TextField is still mounted and animating out.
    // Disposing the controller here is exactly what the old code did.
    await tester.pump(const Duration(milliseconds: 100));
    await tester.pumpAndSettle();

    expectNoFlutterErrors();
    expect(find.byType(OpenStreamDialog), findsNothing);
  });

  testWidgets('playing a valid URL pops it back with the play action',
      (tester) async {
    final popped = await openStreamDialog(tester);

    await tester.enterText(
      find.byType(TextField),
      'https://cdn.example.com/a.m3u8',
    );
    await tester.tap(find.text('Відтворити'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    await tester.pumpAndSettle();

    expectNoFlutterErrors();
    expect(popped.value?.action, StreamDialogAction.play);
    expect(popped.value?.url, 'https://cdn.example.com/a.m3u8');
  });

  testWidgets('the watch-party action survives the exit animation',
      (tester) async {
    final popped = await openStreamDialog(tester);

    await tester.enterText(
      find.byType(TextField),
      'https://cdn.example.com/a.m3u8',
    );
    await tester.tap(find.text('Спільний перегляд'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    await tester.pumpAndSettle();

    expectNoFlutterErrors();
    expect(popped.value?.action, StreamDialogAction.watchParty);
    expect(popped.value?.url, 'https://cdn.example.com/a.m3u8');
  });

  testWidgets('an invalid URL keeps the dialog open and shows the reason',
      (tester) async {
    final popped = await openStreamDialog(tester);

    await tester.enterText(find.byType(TextField), 'http://192.168.1.1/x.m3u8');
    await tester.tap(find.text('Відтворити'));
    await tester.pumpAndSettle();

    expect(find.byType(OpenStreamDialog), findsOneWidget);
    expect(
      find.text('Посилання на приватну мережу не підтримується'),
      findsOneWidget,
    );
    expectNoFlutterErrors();
    expect(popped.value, isNull);

    // Typing over the complaint clears it, and a now-valid URL goes through.
    await tester.enterText(
      find.byType(TextField),
      'https://cdn.example.com/a.m3u8',
    );
    await tester.pumpAndSettle();
    expect(find.byType(OpenStreamDialog), findsOneWidget);

    await tester.tap(find.text('Відтворити'));
    await tester.pumpAndSettle();
    expectNoFlutterErrors();
    expect(popped.value?.url, 'https://cdn.example.com/a.m3u8');
  });
}