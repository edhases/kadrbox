import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../core/config/app_config.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';

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
  Map<String, dynamic>? _user;

  OxideServerService(this._prefs, this._apiClient) {
    _loadState();
  }

  void _loadState() {
    _accessToken = _prefs.getString(_tokenKey);
    _refreshToken = _prefs.getString(_refreshKey);
    final userJson = _prefs.getString(_userKey);
    if (userJson != null) {
      try {
        _user = jsonDecode(userJson) as Map<String, dynamic>;
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
  String? get userId => _user?['id'];
  String? get userEmail => _user?['email'];
  String? get userName => _user?['username'];
  String? get avatar => _user?['avatar_url'];
  String? get bio => _user?['bio'];
  bool get isVerified => (_user?['is_verified'] as bool?) ?? false;
  Map<String, dynamic>? get user => _user;
  String? get accessToken => _accessToken;

  // ===========================================================================
  // Authentication & Profile
  // ===========================================================================

  /// Sign in with email and password
  Future<void> signIn(String email, String password) async {
    final url = '${AppConfig.serverApiUrl}/auth/login';
    try {
      final res = await _apiClient.post(url, data: {
        'email': email.trim(),
        'password': password,
      });

      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User signed in successfully: $email', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
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
      final res = await _apiClient.post(url, data: {
        'email': email.trim(),
        'password': password,
        'username': name.trim(),
      });

      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User registered successfully: $email', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
    } catch (e) {
      Logger.e('Sign up failed for $email', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign in with Google ID token
  Future<void> signInWithGoogle(String idToken) async {
    final url = '${AppConfig.serverApiUrl}/auth/google';
    try {
      final res = await _apiClient.post(url, data: {
        'id_token': idToken.trim(),
      });

      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User logged in with Google: $userEmail', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
    } catch (e) {
      Logger.e('Google sign in failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign in with OAuth provider (discord, telegram) using loopback HTTP server
  Future<void> signInWithOAuthLoopback(String provider) async {
    HttpServer? server;
    try {
      server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      final redirectUrl = 'http://127.0.0.1:${server.port}/callback';

      final authUri = Uri.parse('${AppConfig.serverApiUrl}/auth/$provider/login').replace(
        queryParameters: {'redirect_to': redirectUrl},
      );

      Logger.i('Launching OAuth for $provider with loopback: $redirectUrl', tag: _tag);
      if (!await launchUrl(authUri, mode: LaunchMode.externalApplication)) {
        throw Exception('Не вдалося відкрити браузер для авторизації $provider');
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
        onTimeout: () => throw TimeoutException('Час очікування авторизації вичерпано'),
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
          ..write('''<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <title>Помилка авторизації — Oxide Film</title>
  <style>
    body {
      background: #0f172a; color: #fff; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0;
    }
    .box {
      background: #1e293b; border: 1px solid #334155; padding: 40px; border-radius: 16px; text-align: center; max-width: 400px;
      box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    h2 { margin: 0 0 12px; color: #ef4444; }
    p { color: #94a3b8; font-size: 15px; margin: 0; }
  </style>
</head>
<body>
  <div class="box">
    <h2>❌ Авторизацію скасовано</h2>
    <p>${htmlEscape.convert(authError)}</p>
  </div>
</body>
</html>''');
        await request.response.close();
        throw Exception('Авторизація $provider скасована: $authError');
      }

      request.response
        ..statusCode = HttpStatus.ok
        ..headers.contentType = ContentType.html
        ..write('''<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <title>Успішний вхід — Oxide Film</title>
  <style>
    body {
      background: #0f172a; color: #fff; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0;
    }
    .box {
      background: #1e293b; border: 1px solid #334155; padding: 40px; border-radius: 16px; text-align: center; max-width: 400px;
      box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    h2 { margin: 0 0 12px; color: #10b981; }
    p { color: #94a3b8; font-size: 15px; margin: 0; }
  </style>
</head>
<body>
  <div class="box">
    <h2>✅ Авторизація успішна!</h2>
    <p>Ви можете закрити цю вкладку та повернутися у додаток Oxide Film.</p>
  </div>
  <script>setTimeout(() => window.close(), 1500);</script>
</body>
</html>''');
      await request.response.close();

      if (accessToken != null && refreshToken != null) {
        await setTokensFromOAuth(accessToken: accessToken, refreshToken: refreshToken);
        Logger.i('OAuth $provider sign in successful: $userEmail', tag: _tag);
      } else {
        throw Exception('Токени не отримано від сервера авторизації');
      }
    } finally {
      await server?.close(force: true);
    }
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
      final res = await _apiClient.post(url, data: body);
      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User logged in with Discord: $userEmail', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
    } catch (e) {
      Logger.e('Discord sign in failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign in with Telegram Auth data (direct API)
  Future<void> signInWithTelegramData(Map<String, dynamic> telegramData) async {
    final url = '${AppConfig.serverApiUrl}/auth/telegram';
    try {
      final res = await _apiClient.post(url, data: telegramData);
      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User logged in with Telegram: $userEmail', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
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
        _user = Map<String, dynamic>.from(_user!)..['is_verified'] = true;
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
    _accessToken = null;
    _refreshToken = null;
    _user = null;
    _apiClient.setAuthToken(null);
    await _prefs.remove(_tokenKey);
    await _prefs.remove(_refreshKey);
    await _prefs.remove(_userKey);
    Logger.i('User signed out and tokens cleared', tag: _tag);
  }

  /// Refresh auth access token
  Future<bool> refreshAuth() async {
    if (_refreshToken == null || _refreshToken!.isEmpty) return false;

    final url = '${AppConfig.serverApiUrl}/auth/refresh';
    try {
      final res = await _apiClient.post(url, data: {
        'refresh_token': _refreshToken,
      });

      if (res is Map && res['access_token'] != null) {
        _accessToken = res['access_token'] as String;
        await _prefs.setString(_tokenKey, _accessToken!);
        if (res['refresh_token'] != null) {
          _refreshToken = res['refresh_token'] as String;
          await _prefs.setString(_refreshKey, _refreshToken!);
        }
        _apiClient.setAuthToken(_accessToken);
        Logger.d('Auth token refreshed successfully', tag: _tag);
        return true;
      }
      return false;
    } catch (e) {
      Logger.w('Failed to refresh token: $e', tag: _tag);
      return false;
    }
  }

  /// Get current user profile from server
  Future<Map<String, dynamic>?> fetchMe() async {
    if (!isAuthenticated) return null;
    final url = '${AppConfig.serverApiUrl}/auth/me';
    try {
      final res = await _apiClient.getJson(url);
      if (res.isNotEmpty) {
        _user = res;
        await _prefs.setString(_userKey, jsonEncode(_user));
        return _user;
      }
    } catch (e) {
      Logger.w('Failed to fetch user profile: $e', tag: _tag);
    }
    return _user;
  }

  /// Update user profile
  Future<void> updateProfile({String? name, String? bio, String? avatar}) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/profile';

    final body = <String, dynamic>{};
    if (name != null) body['name'] = name;
    if (bio != null) body['bio'] = bio;
    if (avatar != null) body['avatar'] = avatar;

    final res = await _apiClient.put(url, data: body);
    if (res is Map) {
      _user = Map<String, dynamic>.from(res);
      await _prefs.setString(_userKey, jsonEncode(_user));
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
      'avatar': await MultipartFile.fromFile(
        file.path,
        filename: filename,
      ),
    });

    try {
      final res = await _apiClient.post(url, data: formData);
      if (res is Map) {
        _user = Map<String, dynamic>.from(res);
        await _prefs.setString(_userKey, jsonEncode(_user));
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
      await _apiClient.post(url, data: {
        'token': token.trim(),
        'password': newPassword,
      });
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
      await _apiClient.post(url, data: {
        'old_password': oldPassword,
        'new_password': newPassword,
      });
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

  Future<void> _saveAuthData(Map<dynamic, dynamic> res) async {
    _accessToken = res['access_token'] as String?;
    _refreshToken = res['refresh_token'] as String?;

    if (res['user'] != null && res['user'] is Map) {
      _user = Map<String, dynamic>.from(res['user'] as Map);
      await _prefs.setString(_userKey, jsonEncode(_user));
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
  Future<List<Map<String, dynamic>>> getHistory({int limit = 100, int offset = 0}) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/history';
    try {
      final list = await _apiClient.getJsonList(url, queryParameters: {
        'limit': limit,
        'offset': offset,
      });
      return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
    } catch (e) {
      Logger.w('Failed to get history from server: $e', tag: _tag);
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
      await _apiClient.post(url, data: payload);
    } catch (e) {
      Logger.w('Failed to sync history item to server: $e', tag: _tag);
    }
  }

  /// Get continue watching items from server
  Future<List<Map<String, dynamic>>> getContinueWatching({int limit = 20}) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/continue-watching';
    try {
      final list = await _apiClient.getJsonList(url, queryParameters: {
        'limit': limit,
      });
      return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
    } catch (e) {
      Logger.w('Failed to get continue-watching from server: $e', tag: _tag);
      return [];
    }
  }

  // ===========================================================================
  // Cloud Sync: Favorites
  // ===========================================================================

  /// Get favorites from server
  Future<List<Map<String, dynamic>>> getFavorites({int limit = 200, int offset = 0}) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/favorites';
    try {
      final list = await _apiClient.getJsonList(url, queryParameters: {
        'limit': limit,
        'offset': offset,
      });
      return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
    } catch (e) {
      Logger.w('Failed to get favorites from server: $e', tag: _tag);
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
      final res = await _apiClient.post(url, data: {
        'media_id': mediaId,
        'provider_id': providerId,
        'title': title,
        'poster_url': posterUrl,
        'year': year,
        'media_type': mediaType,
        'rating': rating,
        'rating_source': ratingSource,
      });
      if (res is Map && res['is_favorite'] != null) {
        return res['is_favorite'] as bool;
      }
    } catch (e) {
      Logger.w('Failed to toggle favorite on server: $e', tag: _tag);
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
      await _apiClient.delete(url, queryParameters: {
        'media_id': mediaId,
        'provider_id': providerId,
      });
    } catch (e) {
      Logger.w('Failed to remove favorite from server: $e', tag: _tag);
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
    final uri = Uri.parse(baseUrl).replace(queryParameters: {
      'room': roomCode,
      'user_id': userId,
      'user_name': userName,
    });

    Logger.i('Connecting to Watch Party WebSocket: $uri', tag: _tag);
    final ws = await WebSocket.connect(
      uri.toString(),
    ).timeout(
      const Duration(seconds: 10),
      onTimeout: () {
        throw TimeoutException('Watch Party WebSocket connection timed out after 10s');
      },
    );
    Logger.i('Connected to Watch Party WebSocket', tag: _tag);
    return ws;
  }
}
