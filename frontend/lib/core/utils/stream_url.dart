// Validation for user-entered and peer-supplied media URLs.
//
// One implementation, because the callers have the same threat and it is much
// cheaper to reason about a single rule set than two that drift.
//
// The watch-party case is the reason this is stricter than a `startsWith`
// check. There the URL originates from *another user*: the host of a room
// supplies the media and every guest's player fetches it. Without the host
// rules below, a room becomes a tool for making every guest's device probe its
// own LAN, or read a local file and stream it onward. These are the same rules
// as contracts/plugins/SECURITY.md, applied to a peer instead of a server.

/// Schemes that must never reach a player.
const Set<String> _rejectedSchemes = {
  'file', // file:///C:/Users/... would make the player read a local file
  'ftp',
  'data',
  'javascript',
  'blob',
  'content',
  'intent',
};

/// Host names that always mean "this device" or "this network".
const Set<String> _rejectedHostNames = {
  'localhost',
  'localhost.localdomain',
  'ip6-localhost',
  'ip6-loopback',
};

/// Long enough for any signed CDN URL, short enough that a room cannot use the
/// media field to smuggle a payload through a chat frame.
const int _maxUrlLength = 4096;

/// Returns a human-readable reason the URL is unusable, or null when it is fine.
///
/// Written for a Ukrainian UI because every caller shows the result verbatim.
String? validateMediaUrl(String? raw) {
  final trimmed = raw?.trim() ?? '';
  if (trimmed.isEmpty) {
    return 'Введіть посилання';
  }
  if (trimmed.length > _maxUrlLength) {
    return 'Посилання задовге';
  }

  final uri = Uri.tryParse(trimmed);
  if (uri == null) {
    return 'Не вдалося розібрати посилання';
  }

  final scheme = uri.scheme.toLowerCase();
  if (scheme.isEmpty) {
    return 'Посилання має починатися з http:// або https://';
  }
  if (scheme != 'http' && scheme != 'https') {
    return _rejectedSchemes.contains(scheme)
        ? 'Схема $scheme заборонена'
        : 'Підтримуються лише http та https';
  }

  if (uri.host.isEmpty) {
    return 'У посилання немає хоста';
  }

  final hostProblem = _hostProblem(uri.host.toLowerCase());
  if (hostProblem != null) {
    return hostProblem;
  }

  return null;
}

/// Rejects hosts that resolve inside the device or its network.
///
/// Literal IPs are checked exactly. A *name* that resolves to a private address
/// cannot be detected without a DNS lookup, and doing one here would leak a
/// lookup per keystroke to a resolver we do not control. That limit is stated
/// in SECURITY.md rather than papered over with a lookup that pretends to be a
/// defence.
String? _hostProblem(String host) {
  if (_rejectedHostNames.contains(host)) {
    return 'Посилання на локальний хост не підтримується';
  }

  // Uri.host already strips the brackets from a literal IPv6 address.
  final ip = _parseIp(host);
  if (ip == null) return null; // a name, not an address literal

  if (ip.isLoopback || ip.isLinkLocal || ip.isUnspecified) {
    return 'Посилання на локальну мережу не підтримується';
  }
  if (ip.isPrivate) {
    return 'Посилання на приватну мережу не підтримується';
  }
  return null;
}

_ParseableIp? _parseIp(String value) {
  // Dotted quad first. int.tryParse alone is not enough and was the original
  // defect here: "127.0.0.1" is not an integer literal, so every IPv4 address
  // fell through as a hostname and none of the range checks ever ran. A
  // validator that cannot see 192.168.1.1 is worse than no validator, because
  // it looks like one.
  if (value.contains('.')) {
    final parts = value.split('.');
    if (parts.length != 4) return null;
    var packed = 0;
    for (final part in parts) {
      if (part.isEmpty || part.length > 3) return null;
      // Reject leading zeros: "010" is ambiguous (octal to some resolvers) and
      // "127.0.0.01" must not slip past a range check as 127.0.0.1.
      if (part.length > 1 && part[0] == '0') return null;
      final octet = int.tryParse(part);
      if (octet == null || octet < 0 || octet > 255) return null;
      packed = (packed << 8) | octet;
    }
    return _ParseableIp.v4(packed);
  }

  final asInt = int.tryParse(value);
  if (asInt != null) {
    if (asInt < 0 || asInt > 0xFFFFFFFF) return null;
    return _ParseableIp.v4(asInt);
  }

  // A compact IPv6 form such as ::1. Anything with a zone id or a scope
  // suffix is rejected outright rather than half-parsed.
  if (!value.contains(':') || value.contains('%')) return null;
  final parts = value.split(':');
  if (parts.length < 3 || parts.length > 9) return null;
  if (parts.any((p) => p.isNotEmpty && int.tryParse(p, radix: 16) == null)) {
    return null;
  }
  return _ParseableIp.v6(value);
}

class _ParseableIp {
  _ParseableIp.v4(this._v4) : _v6 = null;
  _ParseableIp.v6(this._v6) : _v4 = null;

  final int? _v4;
  final String? _v6;

  bool get isUnspecified {
    final v4 = _v4;
    if (v4 != null) return v4 == 0;
    return _v6!.replaceAll(':', '').replaceAll('0', '').isEmpty;
  }

  bool get isLoopback {
    final v4 = _v4;
    if (v4 != null) return (v4 >> 24) == 127;
    // ::1 and 0:0:0:0:0:0:0:1
    return _v6!.split(':').last == '1';
  }

  bool get isLinkLocal {
    final v4 = _v4;
    if (v4 != null) return (v4 & 0xFFFF0000) == 0xA9FE0000; // 169.254.0.0/16
    // fe80::/10 covers fe80 through febf.
    final head = _v6!.replaceAll(':', '').toLowerCase();
    return head.startsWith('fe8') ||
        head.startsWith('fe9') ||
        head.startsWith('fea') ||
        head.startsWith('feb');
  }

  bool get isPrivate {
    final v4 = _v4;
    if (v4 != null) {
      if ((v4 & 0xFF000000) == 0x0A000000) return true; // 10.0.0.0/8
      if ((v4 & 0xFFF00000) == 0xAC100000) return true; // 172.16.0.0/12
      if ((v4 & 0xFFFF0000) == 0xC0A80000) return true; // 192.168.0.0/16
      if ((v4 & 0xFF000000) == 0x7F000000) return true; // 127.0.0.0/8
      return false;
    }
    // Unique local fc00::/7, i.e. leading fc or fd.
    final head = _v6!.replaceAll(':', '').toLowerCase();
    return head.startsWith('fc') || head.startsWith('fd');
  }
}

/// Convenience for callers that only need a yes/no.
bool isAcceptableMediaUrl(String? raw) => validateMediaUrl(raw) == null;

/// Whether [url]'s host resolves inside this device or its network.
///
/// Same rule as [validateMediaUrl], asked on its own, because the catalog
/// boundary needs it as a *question about the server* rather than about a
/// stream: a user who deliberately points the app at `127.0.0.1` is running
/// their own server, and the LAN restriction that protects against a hostile
/// remote catalog must not also break local development.
///
/// Returns false for an unparseable URL or one without a host.
bool urlPointsAtLocalNetwork(String url) {
  final uri = Uri.tryParse(url.trim());
  if (uri == null || uri.host.isEmpty) return false;
  final host = uri.host.toLowerCase();
  if (_rejectedHostNames.contains(host) || host.endsWith('.localhost')) {
    return true;
  }
  final ip = _parseIp(host);
  if (ip == null) return false; // a name, not an address literal
  return ip.isLoopback || ip.isLinkLocal || ip.isUnspecified || ip.isPrivate;
}