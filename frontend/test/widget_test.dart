// This is a basic Flutter widget test.
//
// To perform an interaction with a widget in your test, use the WidgetTester
// utility in the flutter_test package. For example, you can send tap and scroll
// gestures. You can also use WidgetTester to find child widgets in the widget
// tree, read text, and verify that the values of widget properties are correct.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('Basic widget test - MaterialApp renders correctly', (
    WidgetTester tester,
  ) async {
    // Build a simple MaterialApp to verify Flutter widget testing works
    await tester.pumpWidget(
      const MaterialApp(home: Scaffold(body: Text('Test Widget'))),
    );

    // Verify the widget renders
    expect(find.text('Test Widget'), findsOneWidget);
  });

  testWidgets('Loading indicator renders correctly', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: CircularProgressIndicator())),
      ),
    );

    expect(find.byType(CircularProgressIndicator), findsOneWidget);
  });
}
