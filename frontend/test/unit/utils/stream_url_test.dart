import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/core/utils/stream_url.dart';

// The watch-party room is the reason these rules exist. There the URL comes
// from another user, and every guest's player fetches it. Without the host
// rules a room becomes a way to make each guest's device probe its own LAN or
// read a local file, so the rejections below are the security property, not
// input tidiness.

void main() {
  group('accepts', () {
    const good = [
      'https://cdn.example/hls/serials/x/s01/e01/index.m3u8',
      'http://cdn.example/stream.mp4',
      'https://cdn.example/index.m3u8?token=abc&expires=123',
      'https://sub.domain.example:8443/a/b/c.m3u8',
    ];
    for (final url in good) {
      test(url, () {
        expect(validateMediaUrl(url), isNull);
      });
    }
  });

  group('rejects empty or unparsable', () {
    const bad = [
      '',
      '   ',
      'not a url',
      'example.com/stream.m3u8',
      '://missing-scheme',
    ];
    for (final url in bad) {
      test(url.isEmpty ? '(empty)' : url, () {
        expect(validateMediaUrl(url), isNotNull);
      });
    }

    test('null', () {
      expect(validateMediaUrl(null), isNotNull);
    });
  });

  group('rejects schemes that would read local state', () {
    // file:// is the sharp one: a player handed file:///C:/Users/... will
    // read that file, and the bytes then go wherever the stream goes next.
    const bad = [
      'file:///C:/Users/someone/.aws/credentials',
      'file:////etc/passwd',
      'ftp://cdn.example/a.mp4',
      'data:video/mp4;base64,AAAA',
      'javascript:alert(1)',
      'blob:https://cdn.example/uuid',
      'content://media/external/video/1',
    ];
    for (final url in bad) {
      test(url.split(':').first, () {
        expect(validateMediaUrl(url), isNotNull);
      });
    }
  });

  group('rejects loopback and link-local', () {
    const bad = [
      'http://127.0.0.1:8089/index.m3u8',
      'http://127.1.2.3/index.m3u8',
      'http://localhost:8089/index.m3u8',
      'http://[::1]:8089/index.m3u8',
      'http://169.254.169.254/latest/meta-data/',
      'http://0.0.0.0/index.m3u8',
    ];
    for (final url in bad) {
      test(url, () {
        expect(validateMediaUrl(url), isNotNull);
      });
    }
  });

  group('rejects private networks', () {
    // A guest whose router answers 192.168.0.1 exposes its whole LAN.
    const bad = [
      'http://10.0.0.5/index.m3u8',
      'http://172.16.4.4/index.m3u8',
      'http://172.31.255.1/index.m3u8', // upper bound of 172.16/12
      'http://192.168.1.1/index.m3u8',
      'http://[fd00::1]/index.m3u8', // unique local
    ];
    for (final url in bad) {
      test(url, () {
        expect(validateMediaUrl(url), isNotNull);
      });
    }
  });

  test('does not over-reject addresses just outside the private ranges', () {
    // 172.15/16 and 172.32/16 are public. A validator that blocks the whole
    // /16 would be technically safe and practically broken.
    expect(validateMediaUrl('http://172.15.0.1/a.m3u8'), isNull);
    expect(validateMediaUrl('http://172.32.0.1/a.m3u8'), isNull);
    expect(validateMediaUrl('http://11.0.0.1/a.m3u8'), isNull);
    expect(validateMediaUrl('http://9.255.255.255/a.m3u8'), isNull);
  });

  test('rejects an over-long URL', () {
    final long = 'https://cdn.example/${'a' * 5000}.m3u8';
    expect(validateMediaUrl(long), isNotNull);
  });

  test('a hostname that looks numeric but is not an address is a name', () {
    // 1.2.3 is a legal hostname shape; refusing it would break a real domain.
    expect(validateMediaUrl('http://1.2.3/index.m3u8'), isNull);
  });

  test('isAcceptableMediaUrl mirrors validateMediaUrl', () {
    expect(isAcceptableMediaUrl('https://cdn.example/a.m3u8'), isTrue);
    expect(isAcceptableMediaUrl('file:///etc/passwd'), isFalse);
  });
}