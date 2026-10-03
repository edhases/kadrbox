// Pins the AppStringsDelegate fix: switching `MaterialApp.locale` must actually
// change the rendered strings, and it must keep doing so when the delegate is a
// single memoised `const` instance.
//
// Before the fix, `AppStringsDelegate` carried an `AppLocale locale` field that
// `shouldReload` compared against while `load` ignored it in favour of its
// parameter. Language switching only worked because `app.dart` rebuilt a brand
// new non-const delegate on every frame; hoisting or memoising it broke the
// feature silently.

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/core/l10n/app_strings.dart';

/// Mirrors `app.dart`, but with the delegate hoisted to a single const value —
/// the refactor the old field made impossible.
const _memoisedDelegate = AppStringsDelegate();

Widget _app(Locale locale) {
  return MaterialApp(
    locale: locale,
    supportedLocales: const [Locale('uk'), Locale('en')],
    localizationsDelegates: const <LocalizationsDelegate<Object>>[
      _memoisedDelegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    home: Builder(
      builder: (context) {
        final s = AppStrings.of(context);
        return Column(
          children: [
            Text('settings:${s.settings}', key: const Key('settings')),
            Text('search:${s.search}', key: const Key('search')),
            Text('cancel:${s.cancel}', key: const Key('cancel')),
            Text('locale:${s.locale.code}', key: const Key('locale')),
          ],
        );
      },
    ),
  );
}

void main() {
  group('AppStringsDelegate', () {
    test('loads strings for the requested locale parameter', () async {
      const delegate = AppStringsDelegate();

      expect((await delegate.load(const Locale('uk'))).locale, AppLocale.uk);
      expect((await delegate.load(const Locale('en'))).locale, AppLocale.en);
      // Unsupported codes fall back rather than throwing.
      expect((await delegate.load(const Locale('de'))).locale, AppLocale.uk);
    });

    test('supports exactly uk and en', () {
      const delegate = AppStringsDelegate();
      expect(delegate.isSupported(const Locale('uk')), isTrue);
      expect(delegate.isSupported(const Locale('en')), isTrue);
      expect(delegate.isSupported(const Locale('fr')), isFalse);
    });

    test('never asks to reload in place', () {
      // A new locale makes WidgetsApp build a fresh delegate, which is the only
      // path that produces new strings. See `load` above.
      const delegate = AppStringsDelegate();
      expect(delegate.shouldReload(delegate), isFalse);
    });

    testWidgets('rendered string follows MaterialApp.locale', (tester) async {
      await tester.pumpWidget(_app(const Locale('uk')));
      await tester.pumpAndSettle();

      expect(find.text('settings:Налаштування'), findsOneWidget);
      expect(find.text('search:Пошук'), findsOneWidget);
      expect(find.text('locale:uk'), findsOneWidget);

      await tester.pumpWidget(_app(const Locale('en')));
      await tester.pumpAndSettle();

      expect(find.text('settings:Settings'), findsOneWidget);
      expect(find.text('search:Search'), findsOneWidget);
      expect(find.text('locale:en'), findsOneWidget);
      expect(find.text('settings:Налаштування'), findsNothing);
    });

    testWidgets('a single const delegate instance switches both ways', (
      tester,
    ) async {
      await tester.pumpWidget(_app(const Locale('en')));
      await tester.pumpAndSettle();
      expect(find.text('cancel:Cancel'), findsOneWidget);

      // Same const delegate object, different locale.
      await tester.pumpWidget(_app(const Locale('uk')));
      await tester.pumpAndSettle();
      expect(find.text('cancel:Скасувати'), findsOneWidget);

      await tester.pumpWidget(_app(const Locale('en')));
      await tester.pumpAndSettle();
      expect(find.text('cancel:Cancel'), findsOneWidget);
    });
  });
}
