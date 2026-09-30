import '../../../core/utils/logger.dart';
import '../../../domain/entities/stream_source.dart';

/// Broad class of playback failure, derived from libmpv's free-form text.
///
/// libmpv reports everything through a single `Player.stream.error` string, so
/// the only way to give the user something actionable is to classify that text.
enum PlaybackFailure {
  /// The URL could not be turned into a playable source at all.
  unresolvable,

  /// Server refused us (403/401) — usually missing Referer/UA or a dead token.
  forbidden,

  /// Server answered with something mpv cannot demux — most often an HTML
  /// page (error page / captcha / login wall) instead of media.
  unsupportedContainer,

  /// DNS / TCP / HTTP level failure — host down, timeout, wrong host.
  unreachable,

  /// Certificate validation or TLS handshake failure.
  tls,

  /// mpv could not decode or output audio/video on this machine.
  codec,

  /// Anything we could not classify.
  unknown,
}

/// Structured playback failure with everything needed to debug it in the field.
///
/// The raw mpv string alone is not enough: without the provider id, the URL
/// that was opened and the exact headers that were sent, a report of
/// "Failed to recognize file format." is unactionable.
class PlaybackError {
  const PlaybackError({
    required this.kind,
    required this.mpvMessage,
    this.providerId,
    required this.url,
    this.httpStatus,
    this.headersSent = const {},
    this.streamType = StreamType.direct,
    this.playerHost,
  });

  final PlaybackFailure kind;

  /// Raw libmpv text, verbatim. Shown only inside the "Деталі" disclosure.
  final String mpvMessage;

  final String? providerId;

  /// The URL that was actually handed to mpv.
  final String url;

  final int? httpStatus;

  /// Exact headers passed to `Media(..., httpHeaders: ...)`.
  final Map<String, String> headersSent;

  final StreamType streamType;

  /// Host of the Referer/Origin actually used, if any.
  final String? playerHost;

  /// Builds a [PlaybackError] from libmpv's free-form error text.
  ///
  /// [url], [headersSent] and [streamType] describe what we *asked* mpv to
  /// open; they are needed because mpv's message never contains them.
  factory PlaybackError.fromMpvMessage(
    String message, {
    String? providerId,
    required String url,
    int? httpStatus,
    Map<String, String> headersSent = const {},
    StreamType streamType = StreamType.direct,
    String? playerHost,
  }) {
    return PlaybackError(
      kind: classifyMpvMessage(message),
      mpvMessage: message,
      providerId: providerId,
      url: url,
      httpStatus: httpStatus,
      headersSent: Map<String, String>.unmodifiable(headersSent),
      streamType: streamType,
      playerHost: playerHost,
    );
  }

  /// Classifies a libmpv/libavformat error message into a [PlaybackFailure].
  ///
  /// Matching is case-insensitive and intentionally tolerant: mpv wording
  /// changes between versions and across FFmpeg builds.
  static PlaybackFailure classifyMpvMessage(String message) {
    final m = message.toLowerCase();

    // 1. Access control. Checked first: a 403 page usually *also* fails to
    //    demux, and the actionable cause is the rejection, not the format.
    if (m.contains('403') ||
        m.contains('forbidden') ||
        m.contains('access denied') ||
        m.contains('401') ||
        m.contains('unauthorized') ||
        m.contains('not permitted')) {
      return PlaybackFailure.forbidden;
    }

    // 2. TLS before "failed to open" so a certificate error is not reported
    //    as a generic network problem.
    if (m.contains('certificate') ||
        m.contains('tls') ||
        m.contains('ssl') ||
        m.contains('handshake')) {
      return PlaybackFailure.tls;
    }

    // 3. Wrong bytes. "Failed to recognize file format." is libmpv's own text
    //    for "this is not media" — in practice an HTML page.
    if (m.contains('failed to recognize file format') ||
        m.contains('unrecognized file format') ||
        m.contains('unknown format') ||
        m.contains('invalid data found') ||
        m.contains('demuxer error') ||
        m.contains('no demuxer') ||
        m.contains('not supported file format')) {
      return PlaybackFailure.unsupportedContainer;
    }

    // 4. Local decode/output problems.
    if (m.contains('decoder') ||
        m.contains('codec') ||
        m.contains('hwdec') ||
        m.contains('vulkan') ||
        m.contains('ao:') ||
        m.contains('vd:') ||
        m.contains('vo:') ||
        m.contains('audio output') ||
        m.contains('video output') ||
        m.contains('no video codec') ||
        m.contains('unsupported codec')) {
      return PlaybackFailure.codec;
    }

    // 5. Transport.
    if (m.contains('failed to open') ||
        m.contains('connection refused') ||
        m.contains('unable to connect') ||
        m.contains('could not connect') ||
        m.contains('network is unreachable') ||
        m.contains('name or service not known') ||
        m.contains('no route to host') ||
        m.contains('resolve') ||
        m.contains('resolut') ||
        m.contains('lookup') ||
        m.contains('timeout') ||
        m.contains('timed out') ||
        m.contains('404') ||
        m.contains('no such host') ||
        m.contains('premature end of http')) {
      return PlaybackFailure.unreachable;
    }

    return PlaybackFailure.unknown;
  }

  // --- User-facing copy ------------------------------------------------------
  //
  // TODO(i18n): the following user-facing strings are declared locally and
  // should move to `lib/core/l10n/app_strings.dart` (section "PLAYBACK
  // ERRORS") once that file is owned by this change:
  //   * titleUnsupportedContainer / messageUnsupportedContainer
  //   * titleForbidden / messageForbidden
  //   * titleUnreachable / messageUnreachable
  //   * titleTls / messageTls
  //   * titleCodec / messageCodec
  //   * titleUnresolvable / messageUnresolvable
  //   * titleUnknown / messageUnknown
  //   * copyDiagnostics, diagnosticsCopied, details

  /// Short Ukrainian headline for this failure kind.
  String get title {
    switch (kind) {
      case PlaybackFailure.unresolvable:
        return 'Джерело не знайдено';
      case PlaybackFailure.forbidden:
        return 'Доступ заборонено';
      case PlaybackFailure.unsupportedContainer:
        return 'Сервер повернув HTML замість відео';
      case PlaybackFailure.unreachable:
        return 'Джерело недоступне';
      case PlaybackFailure.tls:
        return 'Помилка безпеки з\'єднання';
      case PlaybackFailure.codec:
        return 'Не вдалося декодувати відео';
      case PlaybackFailure.unknown:
        return 'Помилка відтворення';
    }
  }

  /// Ukrainian, human-readable explanation for this failure kind.
  String get message {
    switch (kind) {
      case PlaybackFailure.unresolvable:
        return 'Провайдер не повернув жодного джерела для цього епізоду. Спробуйте інше джерело або поверніться пізніше.';
      case PlaybackFailure.forbidden:
        return 'Сервер відхилив запит (403). Найімовірніше потрібні правильні Referer/User-Agent або посилання вже застаріло.';
      case PlaybackFailure.unsupportedContainer:
        return 'Сервер повернув HTML-сторінку замість відеофайлу. Зазвичай це сторінка помилки, капча або вимога входу — перезавантажте джерело або оберіть інше.';
      case PlaybackFailure.unreachable:
        return 'Не вдалося зв\'язатися із сервером відео. Перевірте з\'єднання або спробуйте інше джерело.';
      case PlaybackFailure.tls:
        return 'Не вдалося перевірити сертифікат сервера. Це може бути перехоплення трафіку або проблема з системним часом.';
      case PlaybackFailure.codec:
        return 'Файл відкрито, але відео або звук не вдалося декодувати на цьому пристрої.';
      case PlaybackFailure.unknown:
        return 'Не вдалося запустити відтворення. Відкрийте деталі нижче або спробуйте інше джерело.';
    }
  }

  /// Monospace, copy-friendly diagnostic block.
  ///
  /// Contains everything needed to reproduce the failure from a bug report.
  String toDiagnosticString({String? logTail}) {
    final b = StringBuffer()
      ..writeln('=== OxideFilm playback error ===')
      ..writeln('kind: ${kind.name}')
      ..writeln('title: $title')
      ..writeln('provider: ${providerId ?? '<none>'}')
      ..writeln('url: $url')
      ..writeln('streamType: ${streamType.name}')
      ..writeln('playerHost: ${playerHost ?? '<none>'}')
      ..writeln('httpStatus: ${httpStatus ?? '<none>'}')
      ..writeln('headersSent:');
    if (headersSent.isEmpty) {
      b.writeln('  <none>');
    } else {
      // Sorted so the block is stable between runs.
      final keys = headersSent.keys.toList()..sort();
      for (final k in keys) {
        b.writeln('  $k: ${headersSent[k]}');
      }
    }
    b.writeln('mpvMessage: $mpvMessage');
    if (logTail != null && logTail.isNotEmpty) {
      b
        ..writeln('--- recent log ---')
        ..write(logTail);
    }
    return b.toString();
  }

  /// One-line ERROR-level log with the full diagnostic.
  void log({String tag = 'PlaybackError'}) {
    Logger.e(toDiagnosticString(), tag: tag);
  }

  @override
  String toString() => 'PlaybackError(${kind.name}, $url)';
}
