// Helper to create an in-memory AppDatabase for tests
// Usage:
// final db = createTestAppDatabase();
// final historyDao = HistoryDao(db);

import 'package:drift/native.dart';
import 'package:kadrbox/data/database/app_database.dart'
    as appdb
    show AppDatabase;

/// Creates an in-memory AppDatabase instance suitable for unit tests.
appdb.AppDatabase createTestAppDatabase() {
  final executor = NativeDatabase.memory();
  return appdb.AppDatabase(executor);
}
