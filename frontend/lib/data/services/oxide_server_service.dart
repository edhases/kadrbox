import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:window_manager/window_manager.dart';
import '../../core/config/app_config.dart';
import '../../core/error/exceptions.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';

/// Typed, immutable view of the authenticated user.
class SessionUser {
  final String? id;
  final String? email;
  final String? username;
  final String? avatarUrl;
  final String? bio;
  final bool isVerified;

  /// The original JSON map, kept for callers that read provider-specific keys
  /// (`telegram_id`, `discord_id`, `created_at`, ...) without a typed accessor.
  final Map<String, dynamic> raw;

  const SessionUser({
    required this.raw,
    this.id,
    this.email,
    this.username,
    this.avatarUrl,
    this.bio,
    this.isVerified = false,
  });

  /// Coerces defensively: the server may send `id` as a number (SQLite
  /// serialisation) or a string, and a JSON body is untrusted input. Nothing
  /// here throws — a malformed field simply becomes `null`.
  factory SessionUser.fromJson(Object? json) {
    final map = json is Map
        ? Map<String, dynamic>.from(json)
        : <String, dynamic>{};
    return SessionUser(
      raw: map,
      id: _asString(map['id']),
      email: _asString(map['email']),
      username: _asString(map['username']) ?? _asString(map['name']),
      avatarUrl: _asString(map['avatar_url']) ?? _asString(map['avatar']),
      bio: _asString(map['bio']),
      isVerified: _asBool(map['is_verified']),
    );
  }

  /// `num` (int or double) and String both coerce; anything else yields `null`.
  static String? _asString(Object? value) {
    if (value is String) return value.isEmpty ? null : value;
    if (value is num) return value.toString();
    return null;
  }

  static bool _asBool(Object? value) {
    if (value is bool) return value;
    if (value is num) return value != 0;
    if (value is String) return value.toLowerCase() == 'true';
    return false;
  }

  @override
  String toString() => 'SessionUser(id: $id, email: $email)';
}

/// Service for communication with Oxide Go Backend
/// (Go Chi + PostgreSQL 16 + Redis Pub/Sub WebSocket)
class OxideServerService {
  static const _tag = 'OxideServerService';
  static const _tokenKey = 'oxide_jwt_access_token';
  static const _refreshKey = 'oxide_jwt_refresh_token';
  static const _userKey = 'oxide_user_json';

  final SharedPreferences _prefs;
  final ApiClient _apiClient;

  String? _accessToken;
  String? _refreshToken;
  SessionUser? _user;

  /// Bumped by [signOut]. A refresh that started under an older generation must
  /// not re-install a token after the user has signed out.
  int _sessionGeneration = 0;

  /// Callback when session expires (401 on refresh or sync).
  VoidCallback? onAuthExpired;
  Future<bool>? _refreshFuture;

  OxideServerService(this._prefs, this._apiClient) {
    _loadState();
    // The DI container creates ApiClient first, so the 401 refresh/replay
    // interceptor and the proactive refresh timer are attached here.
    _apiClient.attachAuthCallbacks(refresh: refreshAuth, signOut: signOut);
  }

  void _loadState() {
    _accessToken = _prefs.getString(_tokenKey);
    _refreshToken = _prefs.getString(_refreshKey);
    final userJson = _prefs.getString(_userKey);
    if (userJson != null) {
      try {
        _user = SessionUser.fromJson(jsonDecode(userJson));
      } catch (e) {
        Logger.w('Failed to parse cached user JSON: $e', tag: _tag);
      }
    }

    if (_accessToken != null && _accessToken!.isNotEmpty) {
      _apiClient.setAuthToken(_accessToken);
      Logger.i('Restored auth session for: $userEmail', tag: _tag);
    }
  }

  // Quick access properties
  bool get isAuthenticated => _accessToken != null && _accessToken!.isNotEmpty;
  String? get userId => _user?.id;
  String? get userEmail => _user?.email;
  String? get userName => _user?.username;
  String? get avatar => _user?.avatarUrl;
  String? get bio => _user?.bio;
  bool get isVerified => _user?.isVerified ?? false;

  /// Raw user JSON, for callers reading keys without a typed accessor.
  Map<String, dynamic>? get user => _user?.raw;
  String? get accessToken => _accessToken;

  /// A 401 is the *only* thing that counts as an auth failure.
  ///
  /// The previous implementation also substring-matched `toString()`, so a
  /// socket error mentioning "401 bytes" or a URL containing 401 destroyed a
  /// perfectly valid session. `ApiClient` already guarantees a `ServerException`
  /// with a status code for every `badResponse`, and a 503 is a server-side
  /// outage, not an auth problem.
  bool isUnauthorizedError(Object? e) =>
      e is ServerException && e.statusCode == 401;

  /// Defensive `Object? -> String?` coercion for untyped JSON payloads.
  static String? _stringOrNull(Object? value) {
    if (value is String) return value.isEmpty ? null : value;
    if (value is num) return value.toString();
    return null;
  }

  Future<void> _handleAuthExpired() async {
    Logger.w(
      'Auth expired or invalid, clearing session and notifying listeners',
      tag: _tag,
    );
    await signOut();
  }

  /// Refresh-once-then-replay wrapper.
  ///
  /// A 401 used to call [signOut] directly, which meant the first 401 after the
  /// 15-minute access-token TTL silently signed the user out. Now the single
  /// already covers this; the service-level retry is a defence in depth for
  /// call sites running with the interceptor disabled.
  ///
  /// The original error is always rethrown when recovery is impossible, so
  /// callers keep their existing error contract.
  Future<T> _withAuthRecovery<T>(Future<T> Function() call) async {
    try {
      return await call();
    } catch (e) {
      if (isUnauthorizedError(e) && isAuthenticated) {
        Logger.i(
          '401 from server, attempting token refresh then retry',
          tag: _tag,
        );
        final refreshed = await refreshAuth();
        if (refreshed) {
          return call();
        }
        if (isAuthenticated) await _handleAuthExpired();
      }
      rethrow;
    }
  }

  // ===========================================================================
  // Authentication & Profile
  // ===========================================================================

  /// Sign in with email and password
  Future<void> signIn(String email, String password) async {
    final url = '${AppConfig.serverApiUrl}/auth/login';
    try {
      final res = await _apiClient.postJson(
        url,
        data: {'email': email.trim(), 'password': password},
      );

      if (res.isEmpty) {
        throw Exception('Неочікувана відповідь від сервера');
      }
      await _saveAuthData(res);
      Logger.i('User signed in successfully: $email', tag: _tag);
    } catch (e) {
      Logger.e('Sign in failed for $email', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign up with email, password, and display name
  Future<void> signUp({
    required String email,
    required String password,
    required String name,
  }) async {
    final url = '${AppConfig.serverApiUrl}/auth/register';
    try {
      final res = await _apiClient.postJson(
        url,
        data: {
          'email': email.trim(),
          'password': password,
          'username': name.trim(),
        },
      );

      if (res.isEmpty) {
        throw Exception('Неочікувана відповідь від сервера');
      }
      await _saveAuthData(res);
      Logger.i('User registered successfully: $email', tag: _tag);
    } catch (e) {
      Logger.e('Sign up failed for $email', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign in with Google ID token
  Future<void> signInWithGoogle(String idToken) async {
    final url = '${AppConfig.serverApiUrl}/auth/google';
    try {
      final res = await _apiClient.postJson(
        url,
        data: {'id_token': idToken.trim()},
      );

      if (res.isEmpty) {
        throw Exception('Неочікувана відповідь від сервера');
      }
      await _saveAuthData(res);
      Logger.i('User logged in with Google: $userEmail', tag: _tag);
    } catch (e) {
      Logger.e('Google sign in failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign in with OAuth provider (discord, telegram, google) using loopback HTTP server.
  /// If [linkToken] is provided (access token of the current session), the
  /// provider is linked to the existing account instead of a fresh sign-in.
  Future<void> signInWithOAuthLoopback(
    String provider, {
    String? linkToken,
  }) async {
    HttpServer? server;
    try {
      server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      var redirectUrl = 'http://127.0.0.1:${server.port}/callback';
      if (linkToken != null && linkToken.isNotEmpty) {
        redirectUrl += '?link_token=${Uri.encodeQueryComponent(linkToken)}';
      }

      final authUri = Uri.parse(
        '${AppConfig.serverApiUrl}/auth/$provider/login',
      ).replace(queryParameters: {'redirect_to': redirectUrl});

      Logger.i(
        'Launching OAuth for $provider with loopback: $redirectUrl',
        tag: _tag,
      );
      if (!await launchUrl(authUri, mode: LaunchMode.externalApplication)) {
        throw Exception(
          'Не вдалося відкрити браузер для авторизації $provider',
        );
      }

      final completer = Completer<HttpRequest>();
      final sub = server.listen((req) {
        if (req.uri.path == '/callback') {
          if (!completer.isCompleted) {
            completer.complete(req);
          }
        } else {
          req.response.statusCode = HttpStatus.notFound;
          req.response.close();
        }
      });

      final request = await completer.future.timeout(
        const Duration(minutes: 5),
        onTimeout: () =>
            throw TimeoutException('Час очікування авторизації вичерпано'),
      );
      await sub.cancel();

      final params = request.uri.queryParameters;
      final authError = params['error'];
      final accessToken = params['access_token'];
      final refreshToken = params['refresh_token'];

      if (authError != null && authError.isNotEmpty) {
        request.response
          ..statusCode = HttpStatus.badRequest
          ..headers.contentType = ContentType.html
          ..write(
            _oauthResultPage(
              success: false,
              title: 'Не вдалося увійти',
              message: authError,
            ),
          );
        await request.response.close();
        throw Exception('Авторизація $provider скасована: $authError');
      }

      final providerName = provider.isEmpty
          ? provider
          : provider[0].toUpperCase() + provider.substring(1);
      request.response
        ..statusCode = HttpStatus.ok
        ..headers.contentType = ContentType.html
        ..write(
          _oauthResultPage(
            success: true,
            title: 'Ви увійшли',
            message: 'Вхід через $providerName пройшов успішно.',
          ),
        );
      await request.response.close();

      if (accessToken != null && refreshToken != null) {
        await setTokensFromOAuth(
          accessToken: accessToken,
          refreshToken: refreshToken,
        );
        Logger.i('OAuth $provider sign in successful: $userEmail', tag: _tag);
        if (!kIsWeb &&
            (defaultTargetPlatform == TargetPlatform.windows ||
                defaultTargetPlatform == TargetPlatform.linux ||
                defaultTargetPlatform == TargetPlatform.macOS)) {
          try {
            await windowManager.show();
            await windowManager.focus();
          } catch (_) {}
        }
      } else {
        throw Exception('Токени не отримано від сервера авторизації');
      }
    } finally {
      await server?.close(force: true);
    }
  }

  /// Branded HTML page shown in the browser after OAuth loopback completes.
  /// Served by the temporary local server, so no external assets allowed —
  /// everything (styles, icons) is inline.
  static String _oauthResultPage({
    required bool success,
    required String title,
    required String message,
  }) {
    final accent = success ? '#10b981' : '#ef4444';
    final icon = success
        ? '<svg width="44" height="44" viewBox="0 0 24 24" fill="none" stroke="#10b981" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="8.5 12.5 11 15 15.5 9.5"/></svg>'
        : '<svg width="44" height="44" viewBox="0 0 24 24" fill="none" stroke="#ef4444" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>';
    final safeTitle = htmlEscape.convert(title);
    final safeMessage = htmlEscape.convert(message);
    final actionHtml = success
        ? '<p class="hint" id="hint-text">Тепер ви можете повернутися до програми Oxide Film.</p>\n'
              '    <button class="btn" id="close-btn" onclick="attemptClose()">Закрити вкладку</button>\n'
              '    <script>\n'
              '      function attemptClose() {\n'
              '        try {\n'
              '          window.opener = null;\n'
              '          window.open("", "_self");\n'
              '          window.close();\n'
              '        } catch (e) {}\n'
              '        setTimeout(function() {\n'
              '          var hint = document.getElementById("hint-text");\n'
              '          if (hint) {\n'
              '            hint.innerHTML = "Браузер блокує закриття з міркувань безпеки.<br>Будь ласка, закрийте її хрестиком або натисніть <b>Ctrl + W</b>.";\n'
              '            hint.style.color = "#cbd5e1";\n'
              '          }\n'
              '          var btn = document.getElementById("close-btn");\n'
              '          if (btn) {\n'
              '            btn.innerHTML = "Натисніть Ctrl + W";\n'
              '            btn.style.borderColor = "#6366f1";\n'
              '          }\n'
              '        }, 150);\n'
              '      }\n'
              '      try { window.close(); } catch(e) {}\n'
              '    </script>'
        : '<button class="btn" onclick="window.close()">Закрити вкладку</button>';
    return '''<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>$safeTitle — Oxide Film</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0; min-height: 100vh;
      display: flex; align-items: center; justify-content: center;
      background: #0b0f1a;
      color: #f1f5f9;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    .card {
      width: 400px; max-width: calc(100vw - 48px);
      background: #111827;
      border: 1px solid #1f2937;
      border-top: 3px solid $accent;
      border-radius: 20px;
      padding: 40px 36px 32px;
      text-align: center;
      box-shadow: 0 24px 48px -12px rgba(0, 0, 0, 0.6);
    }
    .brand {
      display: flex; align-items: center; justify-content: center; gap: 10px;
      margin-bottom: 24px;
    }
    .brand-mark {
      width: 32px; height: 32px; border-radius: 9px;
      background: #6366f1;
      display: inline-flex; align-items: center; justify-content: center;
      font-weight: 800; font-size: 16px; color: #fff;
    }
    .brand-name {
      font-size: 13px; font-weight: 700;
      letter-spacing: 3px; color: #94a3b8;
    }
    .icon { margin-bottom: 16px; }
    h1 { margin: 0 0 8px; font-size: 22px; font-weight: 700; }
    p.msg { margin: 0 0 8px; font-size: 14px; line-height: 1.6; color: #94a3b8; }
    p.hint { margin: 12px 0 0; font-size: 13px; color: #64748b; min-height: 20px; }
    .btn {
      margin-top: 20px; padding: 11px 28px;
      background: transparent; color: #e2e8f0;
      border: 1px solid #334155; border-radius: 10px;
      font-size: 14px; font-weight: 600; cursor: pointer;
    }
    .btn:hover { background: #1e293b; border-color: #475569; }
  </style>
</head>
<body>
  <div class="card">
    <div class="brand"><span class="brand-mark">O</span><span class="brand-name">OXIDE FILM</span></div>
    <div class="icon">$icon</div>
    <h1>$safeTitle</h1>
    <p class="msg">$safeMessage</p>
    $actionHtml
  </div>
</body>
</html>''';
  }

  /// Direct token setup from OAuth callback
  Future<void> setTokensFromOAuth({
    required String accessToken,
    required String refreshToken,
  }) async {
    _accessToken = accessToken;
    _refreshToken = refreshToken;
    await _prefs.setString(_tokenKey, accessToken);
    await _prefs.setString(_refreshKey, refreshToken);
    _apiClient.setAuthToken(accessToken);
    await fetchMe();
  }

  /// Sign in with Discord code (direct API)
  Future<void> signInWithDiscordCode(String code, {String? redirectUri}) async {
    final url = '${AppConfig.serverApiUrl}/auth/discord';
    try {
      final body = <String, dynamic>{'code': code.trim()};
      if (redirectUri != null) body['redirect_uri'] = redirectUri;
      final res = await _apiClient.postJson(url, data: body);
      if (res.isEmpty) {
        throw Exception('Неочікувана відповідь від сервера');
      }
      await _saveAuthData(res);
      Logger.i('User logged in with Discord: $userEmail', tag: _tag);
    } catch (e) {
      Logger.e('Discord sign in failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign in with Telegram Auth data (direct API)
  Future<void> signInWithTelegramData(Map<String, dynamic> telegramData) async {
    final url = '${AppConfig.serverApiUrl}/auth/telegram';
    try {
      final res = await _apiClient.postJson(url, data: telegramData);
      if (res.isEmpty) {
        throw Exception('Неочікувана відповідь від сервера');
      }
      await _saveAuthData(res);
      Logger.i('User logged in with Telegram: $userEmail', tag: _tag);
    } catch (e) {
      Logger.e('Telegram sign in failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Підтверджує email за токеном з листа
  Future<void> verifyEmail(String token) async {
    final url = '${AppConfig.serverApiUrl}/auth/verify-email';
    try {
      await _apiClient.post(url, data: {'token': token});
      // Оновлюємо локальний стан
      if (_user != null) {
        _user = SessionUser.fromJson({..._user!.raw, 'is_verified': true});
        await _prefs.setString(_userKey, jsonEncode(_user!.raw));
      }
      Logger.i('Email verified successfully', tag: _tag);
    } catch (e) {
      Logger.e('Email verification failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Повторно надсилає лист підтвердження
  Future<void> resendVerification(String email) async {
    final url = '${AppConfig.serverApiUrl}/auth/resend-verification';
    try {
      await _apiClient.post(url, data: {'email': email});
      Logger.i('Verification email resent to $email', tag: _tag);
    } catch (e) {
      Logger.e('Resend verification failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign out and clear stored tokens
  Future<void> signOut() async {
    // Invalidate any refresh already in flight: otherwise the startup refresh in
    // `main.dart` completes after this method and re-installs the token, so the
    // user looks signed out until the next launch restores the session.
    _sessionGeneration++;
    _accessToken = null;
    _refreshToken = null;
    _user = null;
    _apiClient.setAuthToken(null);
    await _prefs.remove(_tokenKey);
    await _prefs.remove(_refreshKey);
    await _prefs.remove(_userKey);
    Logger.i('User signed out and tokens cleared', tag: _tag);
    onAuthExpired?.call();
  }

  /// Refresh auth access token with single-flight deduplication
  Future<bool> refreshAuth() {
    if (_refreshFuture != null) {
      return _refreshFuture!;
    }
    final future = _doRefreshAuth();
    _refreshFuture = future;
    return future.whenComplete(() {
      _refreshFuture = null;
    });
  }

  Future<bool> _doRefreshAuth() async {
    final generation = _sessionGeneration;
    if (_refreshToken == null || _refreshToken!.isEmpty) {
      await _handleAuthExpired();
      return false;
    }

    final url = '${AppConfig.serverApiUrl}/auth/refresh';
    try {
      // Deliberately the untyped `post`: `postJson` is a thin wrapper over it,
      // and this endpoint's response shape is validated defensively below
      // anyway. It is also the seam `auth_401_retry_storm_test.dart` mocks.
      final res = await _apiClient.post(
        url,
        data: {'refresh_token': _refreshToken},
      );

      final accessToken = _stringOrNull(
        res is Map ? res['access_token'] : null,
      );
      if (accessToken != null) {
        final refreshToken = _stringOrNull(
          res is Map ? res['refresh_token'] : null,
        );

        // A sign-out happened while this request was on the wire: discard the
        // result instead of resurrecting the session.
        if (generation != _sessionGeneration) {
          Logger.d(
            'Refresh result discarded: session was signed out',
            tag: _tag,
          );
          return false;
        }

        _accessToken = accessToken;
        await _prefs.setString(_tokenKey, accessToken);
        if (refreshToken != null && refreshToken.isNotEmpty) {
          _refreshToken = refreshToken;
          await _prefs.setString(_refreshKey, refreshToken);
        }
        _apiClient.setAuthToken(accessToken);
        Logger.d('Auth token refreshed successfully', tag: _tag);
        return true;
      }
      await _handleAuthExpired();
      return false;
    } catch (e) {
      Logger.w('Failed to refresh token: $e', tag: _tag);
      if (isUnauthorizedError(e)) {
        Logger.i(
          'Refresh token is invalid or expired, clearing session',
          tag: _tag,
        );
        await _handleAuthExpired();
      }
      return false;
    }
  }

  /// Get current user profile from server
  Future<Map<String, dynamic>?> fetchMe() async {
    if (!isAuthenticated) return null;
    final url = '${AppConfig.serverApiUrl}/auth/me';
    try {
      final res = await _withAuthRecovery(() => _apiClient.getJson(url));
      if (res.isNotEmpty) {
        await _setUserFromServer(res);
        return _user?.raw;
      }
    } catch (e) {
      Logger.w('Failed to fetch user profile: $e', tag: _tag);
    }
    return _user?.raw;
  }

  Future<void> _setUserFromServer(Map<String, dynamic> json) async {
    _user = SessionUser.fromJson(json);
    await _prefs.setString(_userKey, jsonEncode(_user!.raw));
  }

  /// Update user profile
  Future<void> updateProfile({
    String? name,
    String? bio,
    String? avatar,
  }) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/profile';

    final body = <String, dynamic>{};
    if (name != null) body['name'] = name;
    if (bio != null) body['bio'] = bio;
    if (avatar != null) body['avatar'] = avatar;

    final res = await _withAuthRecovery(
      () => _apiClient.putJson(url, data: body),
    );
    if (res.isNotEmpty) {
      await _setUserFromServer(res);
      Logger.i('Profile updated', tag: _tag);
    }
  }

  /// Upload user avatar
  Future<String?> uploadAvatar(String filePath) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/avatar';

    final file = File(filePath);
    if (!await file.exists()) {
      throw Exception('Файл не знайдено: $filePath');
    }

    final filename = file.path.split(Platform.pathSeparator).last;
    final formData = FormData.fromMap({
      'avatar': await MultipartFile.fromFile(file.path, filename: filename),
    });

    try {
      final res = await _withAuthRecovery(
        () => _apiClient.postJson(url, data: formData),
      );
      if (res.isNotEmpty) {
        await _setUserFromServer(res);
        Logger.i('Avatar uploaded successfully', tag: _tag);
        return avatar;
      }
    } catch (e) {
      Logger.e('Failed to upload avatar', tag: _tag, error: e);
      rethrow;
    }
    return null;
  }

  /// Request password reset email
  Future<void> requestPasswordReset(String email) async {
    final url = '${AppConfig.serverApiUrl}/auth/forgot-password';
    try {
      await _apiClient.post(url, data: {'email': email.trim()});
      Logger.i('Password reset requested for $email', tag: _tag);
    } catch (e) {
      Logger.e('Password reset request failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Reset password with token from email
  Future<void> resetPassword(String token, String newPassword) async {
    final url = '${AppConfig.serverApiUrl}/auth/reset-password';
    try {
      await _apiClient.post(
        url,
        data: {'token': token.trim(), 'password': newPassword},
      );
      Logger.i('Password reset successfully', tag: _tag);
    } catch (e) {
      Logger.e('Password reset failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Change password for authenticated user
  Future<void> changePassword(String oldPassword, String newPassword) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/change-password';
    try {
      await _apiClient.post(
        url,
        data: {'old_password': oldPassword, 'new_password': newPassword},
      );
      Logger.i('Password changed successfully', tag: _tag);
    } catch (e) {
      Logger.e('Password change failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Delete user account
  Future<void> deleteAccount() async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/account';
    try {
      await _apiClient.delete(url);
      await signOut();
      Logger.i('Account deleted successfully', tag: _tag);
    } catch (e) {
      Logger.e('Account deletion failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Unlink OAuth provider (telegram / discord) from the current account.
  /// Server refuses to unlink the last login method — the error is rethrown.
  Future<void> unlinkProvider(String provider) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/unlink';
    try {
      final res = await _withAuthRecovery(
        () => _apiClient.postJson(url, data: {'provider': provider}),
      );
      if (res.isNotEmpty) {
        await _setUserFromServer(res);
        Logger.i('Provider $provider unlinked', tag: _tag);
      }
    } catch (e) {
      Logger.e('Unlink $provider failed', tag: _tag, error: e);
      rethrow;
    }
  }

  Future<void> _saveAuthData(Map<String, dynamic> res) async {
    _accessToken = res['access_token']?.toString();
    _refreshToken = res['refresh_token']?.toString();

    final rawUser = res['user'];
    if (rawUser is Map) {
      _user = SessionUser.fromJson(rawUser);
      await _prefs.setString(_userKey, jsonEncode(_user!.raw));
    }

    if (_accessToken != null) {
      await _prefs.setString(_tokenKey, _accessToken!);
      _apiClient.setAuthToken(_accessToken);
    }
    if (_refreshToken != null) {
      await _prefs.setString(_refreshKey, _refreshToken!);
    }
  }

  // ===========================================================================
  // Cloud Sync: Watch History
  // ===========================================================================

  /// Get watch history from server
  Future<List<Map<String, dynamic>>> getHistory({
    int limit = 100,
    int offset = 0,
  }) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/history';
    try {
      return await _withAuthRecovery(
        () => _apiClient.getJsonMapList(
          url,
          queryParameters: {'limit': limit, 'offset': offset},
        ),
      );
    } catch (e) {
      Logger.w('Failed to get history from server: $e', tag: _tag);
      if (isUnauthorizedError(e)) rethrow;
      return [];
    }
  }

  /// Save progress / update watch history item on server
  Future<void> saveHistoryProgress({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    required int positionMs,
    required int durationMs,
    int? season,
    int? episode,
    String? episodeTitle,
    String? lastStreamUrl,
    String? voiceover,
    DateTime? watchedAt,
  }) async {
    if (!isAuthenticated) return;
    final url = '${AppConfig.serverApiUrl}/sync/history';
    final payload = {
      'media_id': mediaId,
      'provider_id': providerId,
      'title': title,
      'poster_url': posterUrl,
      'year': year,
      'media_type': mediaType,
      'position_ms': positionMs,
      'duration_ms': durationMs,
      'season': season,
      'episode': episode,
      'episode_title': episodeTitle,
      'last_stream_url': lastStreamUrl,
      'voiceover': voiceover,
      'watched_at': (watchedAt ?? DateTime.now()).toUtc().toIso8601String(),
    };

    try {
      await _withAuthRecovery(() => _apiClient.postJson(url, data: payload));
    } catch (e) {
      Logger.w('Failed to sync history item to server: $e', tag: _tag);
      if (isUnauthorizedError(e)) rethrow;
    }
  }

  /// Get continue watching items from server
  Future<List<Map<String, dynamic>>> getContinueWatching({
    int limit = 20,
  }) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/continue-watching';
    try {
      return await _withAuthRecovery(
        () => _apiClient.getJsonMapList(url, queryParameters: {'limit': limit}),
      );
    } catch (e) {
      Logger.w('Failed to get continue-watching from server: $e', tag: _tag);
      if (isUnauthorizedError(e)) rethrow;
      return [];
    }
  }

  // ===========================================================================
  // Cloud Sync: Favorites
  // ===========================================================================

  /// Get favorites from server
  Future<List<Map<String, dynamic>>> getFavorites({
    int limit = 200,
    int offset = 0,
  }) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/favorites';
    try {
      return await _withAuthRecovery(
        () => _apiClient.getJsonMapList(
          url,
          queryParameters: {'limit': limit, 'offset': offset},
        ),
      );
    } catch (e) {
      Logger.w('Failed to get favorites from server: $e', tag: _tag);
      if (isUnauthorizedError(e)) rethrow;
      return [];
    }
  }

  /// Toggle favorite on server
  Future<bool> toggleFavorite({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    double? rating,
    String? ratingSource,
  }) async {
    if (!isAuthenticated) return false;
    final url = '${AppConfig.serverApiUrl}/sync/favorites/toggle';
    try {
      final res = await _withAuthRecovery(
        () => _apiClient.postJson(
          url,
          data: {
            'media_id': mediaId,
            'provider_id': providerId,
            'title': title,
            'poster_url': posterUrl,
            'year': year,
            'media_type': mediaType,
            'rating': rating,
            'rating_source': ratingSource,
          },
        ),
      );
      final isFavorite = res['is_favorite'];
      if (isFavorite != null) return isFavorite == true || isFavorite == 1;
    } catch (e) {
      Logger.w('Failed to toggle favorite on server: $e', tag: _tag);
      if (isUnauthorizedError(e)) rethrow;
    }
    return false;
  }

  /// Explicitly remove favorite from server
  Future<void> removeFavorite({
    required String mediaId,
    required String providerId,
  }) async {
    if (!isAuthenticated) return;
    final url = '${AppConfig.serverApiUrl}/sync/favorites';
    try {
      await _withAuthRecovery(
        () => _apiClient.deleteJson(
          url,
          queryParameters: {'media_id': mediaId, 'provider_id': providerId},
        ),
      );
    } catch (e) {
      Logger.w('Failed to remove favorite from server: $e', tag: _tag);
      if (isUnauthorizedError(e)) rethrow;
    }
  }

  // ===========================================================================
  // Watch Party: WebSocket
  // ===========================================================================

  /// Connect to Watch Party room via WebSocket
  Future<WebSocket> connectWatchParty({
    required String roomCode,
    required String userId,
    required String userName,
  }) async {
    final baseUrl = AppConfig.serverWsUrl;
    final uri = Uri.parse(baseUrl).replace(
      queryParameters: {
        'room': roomCode,
        'user_id': userId,
        'user_name': userName,
      },
    );

    Logger.i('Connecting to Watch Party WebSocket: $uri', tag: _tag);
    final ws = await WebSocket.connect(uri.toString()).timeout(
      const Duration(seconds: 10),
      onTimeout: () {
        throw TimeoutException(
          'Watch Party WebSocket connection timed out after 10s',
        );
      },
    );
    Logger.i('Connected to Watch Party WebSocket', tag: _tag);
    return ws;
  }
}
