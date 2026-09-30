import 'dart:developer' as developer;
import 'package:flutter/foundation.dart';

/// Simple logger utility for debugging
class Logger {
  static const String _tag = 'OxideFilm';

  /// Enable console logs for CLI debugging (only works in debug mode)
  static bool useConsoleLogs = true;

  /// Check if logging should output to console
  static bool get _shouldLogToConsole => useConsoleLogs && kDebugMode;

  static void d(String message, {String? tag}) {
    if (_shouldLogToConsole) print('[DEBUG] [$tag] $message');
    developer.log(
      message,
      name: tag ?? _tag,
      level: 500, // Fine/Debug
    );
  }

  static void i(String message, {String? tag}) {
    if (_shouldLogToConsole) print('[INFO] [$tag] $message');
    developer.log(
      message,
      name: tag ?? _tag,
      level: 800, // Info
    );
  }

  static void w(
    String message, {
    String? tag,
    Object? error,
    StackTrace? stackTrace,
  }) {
    if (_shouldLogToConsole) {
      print('[WARN] [$tag] $message');
      if (error != null) print(error);
    }
    developer.log(
      message,
      name: tag ?? _tag,
      level: 900, // Warning
      error: error,
      stackTrace: stackTrace,
    );
  }

  static void e(
    String message, {
    String? tag,
    Object? error,
    StackTrace? stackTrace,
  }) {
    if (_shouldLogToConsole) {
      print('[ERROR] [$tag] $message');
      if (error != null) print(error);
      if (stackTrace != null) print(stackTrace);
    }
    developer.log(
      message,
      name: tag ?? _tag,
      level: 1000, // Severe/Error
      error: error,
      stackTrace: stackTrace,
    );
  }
}
