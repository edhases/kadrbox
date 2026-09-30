import 'dart:async';
import 'dart:collection';
import 'dart:developer' as developer;
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';

/// Simple logger utility for debugging.
///
/// Console output stays gated behind [kDebugMode] (noisy), but every level is
/// recorded into an in-memory ring buffer and, when possible, a file sink so
/// playback failures remain diagnosable in release builds.
class Logger {
  static const String _tag = 'OxideFilm';

  /// Enable console logs for CLI debugging (only works in debug mode)
  static bool useConsoleLogs = true;

  /// Check if logging should output to console
  static bool get _shouldLogToConsole => useConsoleLogs && kDebugMode;

  /// Maximum number of lines retained in the in-memory ring buffer.
  static const int bufferCapacity = 5000;

  static final Queue<String> _buffer = Queue<String>();

  static bool _fileSinkInitialized = false;
  static File? _fileSink;

  /// Last recorded log lines (newest last), for the copy-diagnostics action.
  ///
  /// Returns at most [bufferCapacity] entries.
  static List<String> get recentLines => List<String>.unmodifiable(_buffer);

  /// Last recorded log lines as a single string, ready to copy to clipboard.
  static String get recentText => _buffer.join('\n');

  static void clearBuffer() => _buffer.clear();

  static void _record(
    String level,
    String message, {
    String? tag,
    Object? error,
    StackTrace? stackTrace,
  }) {
    final sb = StringBuffer()
      ..write('[')
      ..write(level)
      ..write(']')
      ..write('[${tag ?? _tag}] ')
      ..write(message);
    if (error != null) sb.write('\n$error');
    if (stackTrace != null) sb.write('\n$stackTrace');
    _add(sb.toString());
  }

  static void _add(String line) {
    // Split multi-line payloads so the ring buffer counts lines, not entries.
    for (final l in line.split('\n')) {
      _buffer.addLast(l);
    }
    while (_buffer.length > bufferCapacity) {
      _buffer.removeFirst();
    }
    _writeToFile(line);
  }

  // --- file sink -------------------------------------------------------------

  /// Lazily resolves the log file inside the app support directory.
  ///
  /// Any failure (web, missing plugin, no permissions, or a test environment
  /// where the services binding was never initialised) silently disables the
  /// sink — the in-memory buffer always works, so there is nothing to report.
  static Future<void> _ensureFileSink() async {
    if (_fileSinkInitialized) return;
    _fileSinkInitialized = true;
    try {
      final dir = await getApplicationSupportDirectory();
      if (!dir.existsSync()) {
        await dir.create(recursive: true);
      }
      _fileSink = File('${dir.path}${Platform.pathSeparator}oxide_film.log');
    } catch (_) {
      // Expected in tests and on web. Do not log: the binding error text is a
      // multi-paragraph Flutter explanation and would flood the output.
      _fileSink = null;
    }
  }

  static void _writeToFile(String line) {
    if (kIsWeb) return;
    _ensureFileSink().then((_) {
      final f = _fileSink;
      if (f == null) return;
      try {
        f.writeAsStringSync('$line\n', mode: FileMode.append, flush: true);
      } catch (_) {
        // Never let logging break the app.
      }
    });
  }

  // --- levels ----------------------------------------------------------------

  static void d(String message, {String? tag}) {
    _record('DEBUG', message, tag: tag);
    if (_shouldLogToConsole) print('[DEBUG] [$tag] $message');
    developer.log(message, name: tag ?? _tag, level: 500);
  }

  static void i(String message, {String? tag}) {
    _record('INFO', message, tag: tag);
    if (_shouldLogToConsole) print('[INFO] [$tag] $message');
    developer.log(message, name: tag ?? _tag, level: 800);
  }

  static void w(
    String message, {
    String? tag,
    Object? error,
    StackTrace? stackTrace,
  }) {
    _record('WARN', message, tag: tag, error: error, stackTrace: stackTrace);
    if (_shouldLogToConsole) {
      print('[WARN] [$tag] $message');
      if (error != null) print(error);
    }
    developer.log(
      message,
      name: tag ?? _tag,
      level: 900,
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
    _record('ERROR', message, tag: tag, error: error, stackTrace: stackTrace);
    if (_shouldLogToConsole) {
      print('[ERROR] [$tag] $message');
      if (error != null) print(error);
      if (stackTrace != null) print(stackTrace);
    }
    developer.log(
      message,
      name: tag ?? _tag,
      level: 1000,
      error: error,
      stackTrace: stackTrace,
    );
  }
}
