import 'package:flutter_test/flutter_test.dart';

import 'package:kadrbox/domain/entities/stream_source.dart';
import 'package:kadrbox/presentation/pages/player/playback_error.dart';

void main() {
  group('PlaybackError.fromMpvMessage classification', () {
    // Every string below is a real libmpv/libavformat message from the audit
    // report, plus the odd/edge cases the classifier must tolerate.
    final cases = <String, PlaybackFailure>{
      // unsupportedContainer — "the server handed back an HTML page".
      'Failed to recognize file format.': PlaybackFailure.unsupportedContainer,
      'Failed to recognize file format': PlaybackFailure.unsupportedContainer,
      'Unrecognized file format': PlaybackFailure.unsupportedContainer,
      'Demuxer error': PlaybackFailure.unsupportedContainer,
      'unknown format': PlaybackFailure.unsupportedContainer,
      'Invalid data found when processing input':
          PlaybackFailure.unsupportedContainer,
      'No demuxer matching': PlaybackFailure.unsupportedContainer,
      'Not supported file format: https://site.tv/embed/x':
          PlaybackFailure.unsupportedContainer,

      // forbidden — a rejected request usually also fails to demux, so this
      // must win over unsupportedContainer.
      'HTTP error 403 Forbidden': PlaybackFailure.forbidden,
      '403': PlaybackFailure.forbidden,
      'Forbidden': PlaybackFailure.forbidden,
      'Access denied': PlaybackFailure.forbidden,
      '401 Unauthorized': PlaybackFailure.forbidden,
      'Not permitted': PlaybackFailure.forbidden,

      // unreachable — transport failures.
      'Failed to open https://cdn.tv/x.m3u8': PlaybackFailure.unreachable,
      'Connection refused': PlaybackFailure.unreachable,
      'Unable to connect to server': PlaybackFailure.unreachable,
      'Temporary failure in name resolution': PlaybackFailure.unreachable,
      'Could not connect': PlaybackFailure.unreachable,
      'Network is unreachable': PlaybackFailure.unreachable,
      'No route to host': PlaybackFailure.unreachable,
      'Operation timed out': PlaybackFailure.unreachable,
      '404 Not Found': PlaybackFailure.unreachable,
      'Premature end of HTTP stream': PlaybackFailure.unreachable,

      // tls — checked before unreachable so certs are not reported as network.
      'SSL certificate verify failed': PlaybackFailure.tls,
      'TLS handshake failed': PlaybackFailure.tls,
      'certificate has expired': PlaybackFailure.tls,
      'handshake error': PlaybackFailure.tls,

      // codec — the file opened, this machine cannot decode it.
      'decoder not found': PlaybackFailure.codec,
      'Unsupported codec': PlaybackFailure.codec,
      'hwdec-copy failed': PlaybackFailure.codec,
      'Vulkan device lost': PlaybackFailure.codec,
      'AO: [pulse] could not open audio device': PlaybackFailure.codec,
      'VD: no video codec': PlaybackFailure.codec,

      // unknown.
      '': PlaybackFailure.unknown,
      'something nobody has ever seen': PlaybackFailure.unknown,
    };

    cases.forEach((message, expected) {
      test('"$message" -> ${expected.name}', () {
        final error = PlaybackError.fromMpvMessage(
          message,
          url: 'https://cdn.tv/video.m3u8',
        );
        expect(error.kind, expected);
        // The raw text is always preserved for the "Деталі" disclosure.
        expect(error.mpvMessage, message);
      });
    });

    test('classification is case-insensitive', () {
      expect(
        PlaybackError.classifyMpvMessage('failed to RECOGNIZE file FORMAT.'),
        PlaybackFailure.unsupportedContainer,
      );
      expect(
        PlaybackError.classifyMpvMessage('hTtP eRrOr 403 fOrBiDdEn'),
        PlaybackFailure.forbidden,
      );
      expect(
        PlaybackError.classifyMpvMessage('SSL CERTIFICATE VERIFY FAILED'),
        PlaybackFailure.tls,
      );
    });

    test('forbidden wins over unsupportedContainer for a 403 HTML page', () {
      // This is the real-world combination: a 403 that also fails to demux.
      final error = PlaybackError.fromMpvMessage(
        'Failed to recognize file format. (HTTP 403)',
        url: 'https://cdn.tv/x.m3u8',
      );
      expect(error.kind, PlaybackFailure.forbidden);
    });
  });

  group('PlaybackError fields', () {
    test('carries the full diagnostic context', () {
      final error = PlaybackError.fromMpvMessage(
        'Failed to recognize file format.',
        providerId: 'src_a',
        url: 'https://cdn.tv/embed/x',
        httpStatus: 403,
        headersSent: const {
          'Referer': 'https://cdn.tv',
          'User-Agent': 'Mozilla/5.0',
        },
        streamType: StreamType.iframe,
        playerHost: 'cdn.tv',
      );

      expect(error.kind, PlaybackFailure.unsupportedContainer);
      expect(error.providerId, 'src_a');
      expect(error.url, 'https://cdn.tv/embed/x');
      expect(error.httpStatus, 403);
      expect(error.headersSent['Referer'], 'https://cdn.tv');
      expect(error.streamType, StreamType.iframe);
      expect(error.playerHost, 'cdn.tv');
    });

    test('headersSent is unmodifiable and defaults to empty', () {
      final error = PlaybackError.fromMpvMessage(
        'unknown',
        url: 'https://cdn.tv/x.mp4',
      );
      expect(error.headersSent, isEmpty);
      expect(error.providerId, isNull);
      expect(error.httpStatus, isNull);
      expect(() => error.headersSent['X'] = 'y', throwsUnsupportedError);
    });
  });

  group('PlaybackError user-facing copy', () {
    test('every kind has a non-empty Ukrainian title and message', () {
      for (final kind in PlaybackFailure.values) {
        final error = PlaybackError(kind: kind, mpvMessage: '', url: '');
        expect(error.title, isNotEmpty, reason: kind.name);
        expect(error.message, isNotEmpty, reason: kind.name);
      }
    });

    test('unsupportedContainer plainly says the server returned HTML', () {
      final error = PlaybackError(
        kind: PlaybackFailure.unsupportedContainer,
        mpvMessage: '',
        url: '',
      );
      expect(error.title, contains('HTML'));
      expect(error.message, contains('HTML'));
    });
  });

  group('PlaybackError diagnostics', () {
    test('toDiagnosticString includes every debugging field', () {
      final error = PlaybackError.fromMpvMessage(
        'Failed to recognize file format.',
        providerId: 'src_a',
        url: 'https://cdn.tv/embed/x',
        httpStatus: 403,
        headersSent: const {'Referer': 'https://cdn.tv'},
        streamType: StreamType.iframe,
        playerHost: 'cdn.tv',
      );

      final text = error.toDiagnosticString();
      expect(text, contains('unsupportedContainer'));
      expect(text, contains('src_a'));
      expect(text, contains('https://cdn.tv/embed/x'));
      expect(text, contains('iframe'));
      expect(text, contains('cdn.tv'));
      expect(text, contains('403'));
      expect(text, contains('Referer: https://cdn.tv'));
      expect(text, contains('Failed to recognize file format.'));
    });

    test('headers are sorted so the block is stable between runs', () {
      final error = PlaybackError.fromMpvMessage(
        'x',
        url: 'https://cdn.tv/a.mp4',
        headersSent: const {
          'User-Agent': 'UA',
          'Accept': '*/*',
          'Referer': 'https://cdn.tv',
        },
      );
      final headerLines = error
          .toDiagnosticString()
          .split('\n')
          .where(
            (l) =>
                l.startsWith('  Accept:') ||
                l.startsWith('  Referer:') ||
                l.startsWith('  User-Agent:'),
          )
          .toList();
      expect(headerLines, [
        '  Accept: */*',
        '  Referer: https://cdn.tv',
        '  User-Agent: UA',
      ]);
    });

    test('marks an empty header set explicitly', () {
      final error = PlaybackError.fromMpvMessage(
        'x',
        url: 'https://cdn.tv/a.mp4',
      );
      expect(error.toDiagnosticString(), contains('headersSent:\n  <none>'));
    });

    test('includes the log tail when provided', () {
      final error = PlaybackError.fromMpvMessage(
        'x',
        url: 'https://cdn.tv/a.mp4',
      );
      final text = error.toDiagnosticString(logTail: 'line-1\nline-2');
      expect(text, contains('--- recent log ---'));
      expect(text, contains('line-2'));
    });
  });
}
