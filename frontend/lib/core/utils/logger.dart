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
///
/// The file sink is *batched*: lines accumulate in [_pending] and are appended
/// as one chunk every [flushInterval]. Writing per line with `flush: true` cost
/// an fsync on the UI isolate for every single log call, which the player
/// controller turns into dozens of blocking disk flushes per second while
/// scrubbing a seek bar.
class Logger {
  static const String _tag = 'Kadrbox';

  /// Enable console logs for CLI debugging (only works in debug mode)
  static bool useConsoleLogs = true;

  /// Check if logging should output to console
  static bool get _shouldLogToConsole => useConsoleLogs && kDebugMode;

  /// Maximum number of lines retained in the in-memory ring buffer.
  static const int bufferCapacity = 5000;

  /// Hard cap on the on-disk log file. Older lines are trimmed once the file
  /// crosses this, so an unbounded file cannot grow forever on a user's disk.
  static const int maxFileBytes = 2 * 1024 * 1024;

  /// How often buffered lines are appended to disk.
  static const Duration flushInterval = Duration(seconds: 2);

  static final Queue<String> _buffer = Queue<String>();

  // --- file sink -------------------------------------------------------------

  /// Lines recorded since the last flush, oldest first.
  static final Queue<String> _pending = Queue<String>();

  /// Cap on [_pending]. Without a usable sink (web, missing plugin, a test
  /// environment) the buffer would otherwise grow without bound.
  static const int _pendingCapacity = 20000;

  /// Serialises file writes so two flushes can never interleave.
  static Future<void> _writeChain = Future<void>.value();

  static Timer? _flushTimer;
  static bool _disposed = false;
  static bool _sinkResolved = false;

  static bool _fileSinkInitialized = false;
  static File? _fileSink;

  /// Test seam: when non-null, receives each batch instead of the real file.
  ///
  /// Lets a test assert how many times the sink was invoked without depending
  /// on `path_provider`, which is unavailable in a plain unit test.
  @visibleForTesting
  static Future<void> Function(String batch)? batchWriter;

  /// Number of sink invocations since the last [reset].
  @visibleForTesting
  static int sinkInvocationCount = 0;

  /// Last recorded log lines (newest last), for the copy-diagnostics action.
  ///
  /// Returns at most [bufferCapacity] entries.
  static List<String> get recentLines => List<String>.unmodifiable(_buffer);

  /// Last recorded log lines as a single string, ready to copy to clipboard.
  static String get recentText => _buffer.join('\n');

  static void clearBuffer() => _buffer.clear();

  /// Restores pristine state. Only meaningful for tests.
  @visibleForTesting
  static Future<void> reset() async {
    await flush();
    _flushTimer?.cancel();
    _flushTimer = null;
    _disposed = false;
    _pending.clear();
    _buffer.clear();
    _writeChain = Future<void>.value();
    batchWriter = null;
    sinkInvocationCount = 0;
    _fileSink = null;
    _fileSinkInitialized = false;
    _sinkResolved = false;
  }

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
      ..write('][')
      ..write(tag ?? _tag)
      ..write('] ')
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
    if (kIsWeb || _disposed) return;
    _pending.add(line);
    if (_pending.length > _pendingCapacity) _pending.removeFirst();
    _scheduleFlush();
  }

  /// Starts the periodic flush — but only once there is somewhere for a batch to
  /// go.
  ///
  /// The gate matters in widget tests: an unconditional periodic timer would be
  /// left running after the tree is torn down ("a Timer is still pending"), and
  /// in any environment where `path_provider` is unavailable there would be
  /// nothing to flush to anyway.
  static void _scheduleFlush() {
    if (_flushTimer != null || _sinkResolved) return;
    _sinkResolved = true;
    unawaited(
      _ensureFileSink().then((_) {
        if (_disposed ||
            _flushTimer != null ||
            (batchWriter == null && _fileSink == null)) {
          return;
        }
        _flushTimer = Timer.periodic(flushInterval, (_) => unawaited(flush()));
      }),
    );
  }

  /// Appends everything buffered so far as a single write.
  ///
  /// Returns a future that completes once the batch has hit the sink, so tests
  /// (and [dispose]) can observe the result without sleeping.
  static Future<void> flush() {
    if (_pending.isEmpty) return _writeChain;
    final batch = '${_pending.join('\n')}\n';
    _pending.clear();
    _writeChain = _writeChain.then((_) => _writeBatch(batch));
    return _writeChain;
  }

  /// Cancels the periodic flush and writes the buffered tail.
  static Future<void> dispose() async {
    _disposed = true;
    _flushTimer?.cancel();
    _flushTimer = null;
    await flush();
  }

  static Future<void> _writeBatch(String batch) async {
    try {
      final override = batchWriter;
      if (override != null) {
        sinkInvocationCount++;
        await override(batch);
        return;
      }

      await _ensureFileSink();
      final f = _fileSink;
      if (f == null) return;
      sinkInvocationCount++;
      // No `flush: true`. An fsync per batch would reintroduce the UI-thread
      // stall this design exists to avoid; the OS page cache owns durability.
      await f.writeAsString(batch, mode: FileMode.append);
      if (await f.length() > maxFileBytes) {
        await _trim(f, maxFileBytes);
      }
    } catch (_) {
      // Never let logging break the app.
    }
  }

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
      final f = File('${dir.path}${Platform.pathSeparator}kadrbox.log');
      _fileSink = f;
      // A file left over from an earlier run may already be oversized.
      if (f.existsSync() && await f.length() > maxFileBytes) {
        await _trim(f, maxFileBytes);
      }
    } catch (_) {
      // Expected in tests and on web. Do not log: the binding error text is a
      // multi-paragraph Flutter explanation and would flood the output.
      _fileSink = null;
    }
  }

  /// Trims [f] against [maxFileBytes]. Exposed so the tail-keeping logic can be
  /// tested without a `path_provider` binding.
  @visibleForTesting
  static Future<void> trimFileForTest(File f) => _trim(f, maxFileBytes);

  /// Drops the oldest lines until the file fits under [budget].
  ///
  /// Trimming on whole lines keeps the log from starting mid-stack-frame.
  static Future<void> _trim(File f, int budget) async {
    try {
      final lines = await f.readAsLines();
      var bytes = 0;
      var start = lines.length;
      while (start > 0) {
        final size = lines[start - 1].length + 1;
        if (bytes + size > budget) break;
        bytes += size;
        start--;
      }
      if (start == 0) return;
      // Rewriting the whole file is rare (once per [maxFileBytes] logged), so
      // this is the one place an fsync is worth paying for: a torn rewrite
      // would throw away the history we just kept.
      await f.writeAsString(
        '${lines.sublist(start).join('\n')}\n',
        mode: FileMode.write,
        flush: true,
      );
    } catch (_) {
      // Never let logging break the app.
    }
  }

  // --- levels ----------------------------------------------------------------

  static void d(String message, {String? tag}) {
    _record('DEBUG', message, tag: tag);
    if (_shouldLogToConsole) debugPrint('[DEBUG] [$tag] $message');
    developer.log(message, name: tag ?? _tag, level: 500);
  }

  static void i(String message, {String? tag}) {
    _record('INFO', message, tag: tag);
    if (_shouldLogToConsole) debugPrint('[INFO] [$tag] $message');
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
      debugPrint('[WARN] [$tag] $message');
      if (error != null) debugPrint('$error');
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
      debugPrint('[ERROR] [$tag] $message');
      if (error != null) debugPrint('$error');
      if (stackTrace != null) debugPrint('$stackTrace');
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
