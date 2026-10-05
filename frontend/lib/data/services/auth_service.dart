import 'package:flutter/foundation.dart';
import '../../core/utils/logger.dart';
import 'kadrbox_server_service.dart';

/// Authentication service supporting Kadrbox Go Server (Chi/PostgreSQL)
///
/// Note: Cloud sync functionality (watch history, favorites) is managed by
/// HistoryService and FavoritesService with offline-first approach.
/// This service handles authentication and user profiles via Kadrbox Server.
class AuthService extends ChangeNotifier {
  static const _tag = 'AuthService';

  final KadrboxServerService _server;

  // Cached state
  bool _isLoading = false;
  String? _error;

  // Getters
  bool get isAuthenticated => _server.isAuthenticated;
  String? get userId => _server.userId;
  String? get userEmail => _server.userEmail;
  String? get userName => _server.userName;
  bool get isLoading => _isLoading;
  bool get isGuest => !isAuthenticated;
  bool get isVerified => _server.isVerified;
  String? get error => _error;
  List<String> _linkedProviders = [];
  List<String> get linkedProviders => _linkedProviders;

  /// Get current user info (for cloud sync)
  ({String id, String email, String? name})? get currentUser {
    if (!isAuthenticated || userId == null || userEmail == null) return null;
    return (id: userId!, email: userEmail!, name: userName);
  }

  /// Get user profile data (for UI pages)
  Map<String, dynamic>? get profile {
    if (!isAuthenticated) return null;
    return {
      'display_name': _server.userName ?? displayName,
      'bio': _server.bio ?? '',
      'avatar': _server.avatar ?? '',
      'created_at': _server.user?['created_at'] ?? '',
      'watched_count': 0,
      'favorites_count': 0,
    };
  }

  /// Get avatar URL from backend
  String? get avatarUrl {
    if (!isAuthenticated) return null;
    if (_server.avatar != null && _server.avatar!.isNotEmpty) {
      return _server.avatar;
    }
    return null;
  }

  String get displayName => userName ?? userEmail?.split('@').first ?? 'Гість';

  AuthService(this._server) {
    _server.onAuthExpired = _onAuthExpired;
    if (isAuthenticated) {
      Logger.i('User already authenticated: $userEmail', tag: _tag);
      fetchLinkedProviders();
    }
  }

  void _onAuthExpired() {
    Logger.w(
      'Auth expired notification received from KadrboxServerService',
      tag: _tag,
    );
    _linkedProviders = [];
    _error = null;
    notifyListeners();
  }

  /// Sign up with email and password
  Future<void> signUp({
    required String email,
    required String password,
    String? displayName,
  }) async {
    Logger.i('Attempting sign up for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.signUp(
        email: email,
        password: password,
        name: displayName ?? email.split('@').first,
      );
      Logger.i('Sign up successful for: $email', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Sign up failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Sign in with email and password
  Future<void> signIn({required String email, required String password}) async {
    Logger.i('Attempting sign in for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.signIn(email, password);
      Logger.i('Sign in successful for: $email', tag: _tag);
      await fetchLinkedProviders();
      notifyListeners();
    } catch (e) {
      Logger.e('Sign in failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Sign in with social provider (Google, Discord, Telegram) via Kadrbox Server loopback
  Future<void> socialSignIn(String provider) async {
    Logger.i('Attempting social sign in for provider: $provider', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (provider == 'discord' ||
          provider == 'telegram' ||
          provider == 'google') {
        await _server.signInWithOAuthLoopback(provider);
      } else {
        throw Exception('Непідтримуваний провайдер авторизації: $provider');
      }
      Logger.i('Social sign in successful: $userEmail', tag: _tag);
      await fetchLinkedProviders();
      notifyListeners();
    } catch (e) {
      Logger.e('Social sign in failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Sign in with Google ID token directly (Kadrbox Server)
  Future<void> signInWithGoogle(String idToken) async {
    Logger.i('Attempting Google sign in via Kadrbox Server', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.signInWithGoogle(idToken);
      Logger.i('Google sign in successful: $userEmail', tag: _tag);
      await fetchLinkedProviders();
      notifyListeners();
    } catch (e) {
      Logger.e('Google sign in failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Link social provider to existing account via loopback OAuth flow.
  /// Passes the current access token as link_token so the server attaches
  /// the provider to this account instead of creating a new one.
  Future<void> linkSocialAccount(String provider) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    Logger.i('Attempting to link provider: $provider', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (provider == 'discord' ||
          provider == 'telegram' ||
          provider == 'google') {
        await _server.signInWithOAuthLoopback(
          provider,
          linkToken: _server.accessToken,
        );
        await fetchLinkedProviders();
        notifyListeners();
        Logger.i('Provider $provider linked successfully', tag: _tag);
      } else {
        throw Exception('Непідтримуваний провайдер: $provider');
      }
    } catch (e) {
      Logger.e('Linking failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Unlink social provider via Kadrbox Server (keeps at least one login method).
  Future<void> unlinkSocialAccount(String provider) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    Logger.i('Attempting to unlink provider: $provider', tag: _tag);
    _setLoading(true);
    _error = null;
    try {
      await _server.unlinkProvider(provider);
      await fetchLinkedProviders();
      notifyListeners();
    } catch (e) {
      Logger.e('Unlinking failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Fetch list of linked providers from Kadrbox Server
  Future<void> fetchLinkedProviders() async {
    if (!isAuthenticated) return;
    try {
      final user = _server.user;
      final list = <String>[];
      if (user != null) {
        if (user['telegram_id'] != null) list.add('telegram');
        if (user['discord_id'] != null) list.add('discord');
        final email = (user['email'] as String? ?? '').toLowerCase();
        if (email.endsWith('@gmail.com') || user['is_google'] == true) {
          list.add('google');
        }
      }
      _linkedProviders = list;
      notifyListeners();
    } catch (e) {
      Logger.e('Failed to fetch linked providers', tag: _tag, error: e);
    }
  }

  /// Sign out
  Future<void> signOut() async {
    Logger.i('Signing out user: $userEmail', tag: _tag);
    try {
      await _server.signOut();
      _linkedProviders = [];
      notifyListeners();
      Logger.i('Sign out successful', tag: _tag);
    } catch (e) {
      Logger.e('Sign out failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Update user profile
  Future<void> updateProfile({String? displayName, String? bio}) async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Updating profile for: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.updateProfile(name: displayName, bio: bio);
      Logger.i('Profile updated successfully', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Profile update failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Request password reset email
  Future<void> resetPassword(String email) async {
    Logger.i('Requesting password reset for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.requestPasswordReset(email);
      Logger.i('Password reset email sent to: $email', tag: _tag);
    } catch (e) {
      Logger.e('Password reset failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Request email verification
  Future<void> requestVerification(String email) async {
    Logger.i('Requesting email verification for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.resendVerification(email);
      Logger.i('Verification email sent via Kadrbox Server: $email', tag: _tag);
    } catch (e) {
      Logger.e('Email verification request failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Verify email with token from letter
  Future<void> verifyEmail(String token) async {
    Logger.i('Verifying email token', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.verifyEmail(token);
      notifyListeners();
    } catch (e) {
      Logger.e('Email verification failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Refresh authentication token
  Future<bool> refreshAuth() async {
    if (_server.isAuthenticated) {
      final success = await _server.refreshAuth();
      if (!success && !_server.isAuthenticated) {
        _linkedProviders = [];
        notifyListeners();
      }
      return success;
    }
    return false;
  }

  /// Reload user profile from server (verification status, linked providers)
  Future<void> refreshProfile() async {
    if (!isAuthenticated) return;
    await _server.fetchMe();
    await fetchLinkedProviders();
    notifyListeners();
  }

  /// Update user avatar
  Future<void> updateAvatar(String filePath) async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Updating avatar for: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.uploadAvatar(filePath);
      Logger.i('Avatar updated successfully', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Avatar update failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Change password
  Future<void> changePassword(
    String oldPassword,
    String newPassword,
    String newPasswordConfirm,
  ) async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Changing password for: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.changePassword(oldPassword, newPassword);
      Logger.i('Password changed successfully', tag: _tag);
    } catch (e) {
      Logger.e('Password change failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Delete account
  Future<void> deleteAccount() async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Deleting account: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.deleteAccount();
      Logger.i('Account deleted successfully', tag: _tag);
      await signOut();
    } catch (e) {
      Logger.e('Account deletion failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  // ============================================================================
  // HELPERS
  // ============================================================================

  void _setLoading(bool value) {
    _isLoading = value;
    notifyListeners();
  }

  String _translateError(String message) {
    final lowerMsg = message.toLowerCase();

    if (lowerMsg.contains('invalid login credentials') ||
        lowerMsg.contains('invalid username or password') ||
        lowerMsg.contains('invalid email or password')) {
      return 'Невірний email або пароль';
    }
    if (lowerMsg.contains('email not confirmed') ||
        lowerMsg.contains('not verified') ||
        lowerMsg.contains('email not verified')) {
      return 'Електронна пошта не підтверджена. Перевірте свою скриньку.';
    }
    if (lowerMsg.contains('failed to send password reset')) {
      return 'Не вдалося надіслати лист для скидання пароля. Перевірте налаштування пошти.';
    }
    if (lowerMsg.contains('identity already exists')) {
      return 'Цей email уже використовується іншим акаунтом.';
    }
    if (lowerMsg.contains('missing or invalid') ||
        lowerMsg.contains('validation failed')) {
      return 'Помилка валідації. Перевірте введені дані.';
    }
    if (lowerMsg.contains('already exists') ||
        lowerMsg.contains('user already registered')) {
      return 'Користувач з таким email вже існує';
    }
    if (lowerMsg.contains('password') && lowerMsg.contains('at least')) {
      return 'Пароль має бути щонайменше 6 символів';
    }
    if (lowerMsg.contains('invalid email')) {
      return 'Невірний формат email';
    }
    if (lowerMsg.contains('cannot unlink') ||
        lowerMsg.contains('last login method')) {
      return 'Не можна відключити останній спосіб входу';
    }
    if (lowerMsg.contains('already linked') ||
        lowerMsg.contains('already прив')) {
      return 'Цей акаунт вже прив\'язано до іншого користувача';
    }
    if (lowerMsg.contains('rate limit') || lowerMsg.contains('too many')) {
      return 'Забагато спроб. Спробуйте пізніше';
    }

    return message;
  }
}
