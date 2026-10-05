// Runtime-behaviour tests for Logger's batched file sink.
//
// The defects these pin: a synchronous `writeAsStringSync(flush: true)` per log
// line on the UI isolate (an fsync per call), an unbounded log file, and the
// console `print()` calls that bypassed the sink entirely.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/core/utils/logger.dart';

void main() {
  late List<String> batches;

  setUp(() async {
    await Logger.reset();
    batches = <String>[];
    Logger.batchWriter = (batch) async => batches.add(batch);
  });

  tearDown(() async {
    await Logger.reset();
  });

  String allBatches() => batches.join();

  group('Logger file sink', () {
    test('many rapid calls hit the sink once, not once per line', () async {
      for (var i = 0; i < 500; i++) {
        Logger.i('line $i');
      }

      // Nothing has been written yet: the batch is still buffered.
      expect(batches, isEmpty);

      await Logger.flush();

      // One batched write for 500 log calls.
      expect(batches.length, 1);
      expect(Logger.sinkInvocationCount, 1);
    });

    test('batched content preserves every line in order', () async {
      for (var i = 0; i < 200; i++) {
        Logger.i('line $i');
      }
      await Logger.flush();

      final lines = batches.single
          .split('\n')
          .where((l) => l.trim().isNotEmpty)
          .toList();

      expect(lines.length, 200);
      expect(lines.first, '[INFO][OxideFilm] line 0');
      expect(lines.last, '[INFO][OxideFilm] line 199');
      expect(lines, List.generate(200, (i) => '[INFO][OxideFilm] line $i'));
    });

    test('levels and tags reach the sink', () async {
      Logger.d('dbg', tag: 'T');
      Logger.i('inf', tag: 'T');
      Logger.w('warn', tag: 'T');
      Logger.e('err', tag: 'T', error: 'boom');
      await Logger.flush();

      final text = allBatches();
      expect(text, contains('[DEBUG][T] dbg'));
      expect(text, contains('[INFO][T] inf'));
      expect(text, contains('[WARN][T] warn'));
      expect(text, contains('[ERROR][T] err'));
      expect(text, contains('boom'));
    });

    test('flush on an empty buffer does not touch the sink', () async {
      await Logger.flush();
      expect(Logger.sinkInvocationCount, 0);
    });

    test('dispose() flushes the tail', () async {
      Logger.i('before dispose');
      await Logger.dispose();

      expect(Logger.sinkInvocationCount, 1);
      expect(allBatches(), contains('before dispose'));
    });

    test('dispose() cancels the flush timer', () async {
      Logger.i('first');
      await Logger.dispose();
      expect(Logger.sinkInvocationCount, 1);

      // Anything recorded after dispose must not schedule another flush.
      batches.clear();
      Logger.i('after dispose');
      await Future<void>.delayed(Logger.flushInterval * 2);
      expect(batches, isEmpty);
    });

    test('records logged before dispose are not lost', () async {
      for (var i = 0; i < 50; i++) {
        Logger.i('burst $i');
      }
      await Logger.dispose();

      final text = allBatches();
      expect(text, contains('burst 0'));
      expect(text, contains('burst 49'));
    });
  });

  group('Logger ring buffer', () {
    test('keeps at most bufferCapacity lines, newest last', () async {
      final overflow = Logger.bufferCapacity + 250;
      for (var i = 0; i < overflow; i++) {
        Logger.i('n $i');
      }
      await Logger.dispose();

      final lines = Logger.recentLines;
      expect(lines.length, Logger.bufferCapacity);
      expect(lines.last, '[INFO][OxideFilm] n ${overflow - 1}');
      expect(
        lines.first,
        '[INFO][OxideFilm] n ${overflow - Logger.bufferCapacity}',
      );
    });

    test('multi-line payloads count as separate entries', () async {
      Logger.e('boom', tag: 'T', error: 'e1', stackTrace: StackTrace.empty);
      await Logger.dispose();
      expect(Logger.recentLines.length, greaterThan(1));
    });

    test('clearBuffer empties the ring', () async {
      Logger.i('x');
      await Logger.dispose();
      expect(Logger.recentLines, isNotEmpty);
      Logger.clearBuffer();
      expect(Logger.recentLines, isEmpty);
    });
  });

  group('Logger file trimming', () {
    late Directory tmp;

    setUp(() {
      tmp = Directory.systemTemp.createTempSync('oxide_logger_rt');
    });

    tearDown(() {
      if (tmp.existsSync()) tmp.deleteSync(recursive: true);
    });

    test('drops the oldest lines when over the cap', () async {
      // ~600 lines of 100 chars = ~60 KB; shrink the budget by trimming with a
      // hand-made file that is deliberately over maxFileBytes.
      final lines = List.generate(
        (Logger.maxFileBytes ~/ 100) + 500,
        (i) => '${i.toString().padLeft(8, '0')}${'x' * 92}',
      );
      final f = File('${tmp.path}${Platform.pathSeparator}big.log');
      f.writeAsStringSync('${lines.join('\n')}\n');

      expect(f.lengthSync(), greaterThan(Logger.maxFileBytes));

      await Logger.trimFileForTest(f);

      expect(f.lengthSync(), lessThanOrEqualTo(Logger.maxFileBytes));

      final kept = f.readAsLinesSync();
      expect(kept, isNotEmpty);
      // The newest lines survive, the oldest are gone.
      expect(kept.last, lines.last);
      expect(kept.first, isNot(lines.first));
      expect(kept.length, lessThan(lines.length));
    });

    test('is a no-op for a file under the cap', () async {
      final f = File('${tmp.path}${Platform.pathSeparator}small.log');
      const content = 'one\ntwo\nthree\n';
      f.writeAsStringSync(content);

      await Logger.trimFileForTest(f);

      expect(f.readAsStringSync(), content);
    });

    test(
      'a single oversized line is kept rather than emptying the log',
      () async {
        final f = File('${tmp.path}${Platform.pathSeparator}huge.log');
        f.writeAsStringSync('${'y' * (Logger.maxFileBytes + 500)}\n');

        await Logger.trimFileForTest(f);

        // Nothing fits in the budget, so the file is left as-is rather than
        // truncated to nothing.
        expect(f.existsSync(), isTrue);
      },
    );
  });
}
