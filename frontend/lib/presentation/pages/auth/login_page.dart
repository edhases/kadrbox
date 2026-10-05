import 'package:flutter/material.dart';
import 'package:flutter_vector_icons/flutter_vector_icons.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../../data/services/auth_service.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/favorites_service.dart';
import '../../../data/services/settings_service.dart';
import '../../theme/app_theme.dart';
import '../../widgets/dialogs/import_data_dialog.dart';

/// Login page for user authentication
class LoginPage extends StatefulWidget {
  const LoginPage({super.key});

  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  final _formKey = GlobalKey<FormState>();
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  final _authService = GetIt.instance<AuthService>();

  bool _isLoading = false;
  bool _obscurePassword = true;
  String? _error;

  @override
  void dispose() {
    _emailController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  Future<void> _login() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      await _authService.signIn(
        email: _emailController.text.trim(),
        password: _passwordController.text,
      );

      if (mounted) {
        await _checkLocalDataAndSync();
        if (mounted) {
          context.go('/');
        }
      }
    } catch (e) {
      final msg = _authService.error ?? e.toString();
      final unverified =
          msg.contains('не підтверджена') ||
          msg.contains('not verified') ||
          msg.contains('403');
      if (unverified && mounted) {
        context.go(
          '/verify-email?email=${Uri.encodeQueryComponent(_emailController.text.trim())}',
        );
        return;
      }
      setState(() => _error = msg);
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  Future<void> _checkLocalDataAndSync() async {
    final historyService = GetIt.instance<HistoryService>();
    final favoritesService = GetIt.instance<FavoritesService>();
    final settingsService = GetIt.instance<SettingsService>();
    final userId = _authService.userId;
    if (userId == null) return;

    final historyCount = await historyService.count;
    final favoritesCount = await favoritesService.count;

    // Ask only when there is unaccounted-for local data for THIS account.
    // Offering it on every sign-in was the bug: after a successful merge those
    // rows are the account's own history, so the condition stayed true forever
    // and the user was asked to merge their data with their own account on
    // every login.
    final shouldOffer = await settingsService.shouldOfferLocalDataMerge(
      userId: userId,
      historyCount: historyCount,
      favoritesCount: favoritesCount,
    );
    if (!shouldOffer || !mounted) return;

    final shouldMerge = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (context) => ImportDataDialog(
        historyCount: historyCount,
        favoritesCount: favoritesCount,
      ),
    );
    if (!mounted) return;

    if (shouldMerge == true) {
      // Merge: push local to cloud, then pull so the local table reflects the
      // union rather than only what was on the device.
      await historyService.syncNow();
      await favoritesService.syncNow();
    } else {
      // Discard local and repopulate from the account. Ordering matters:
      // clearAll on both services first, then pull. Doing it per-service
      // interleaved syncNow() calls would let one service's push run against
      // the other's freshly-cleared table.
      await historyService.clearAll();
      await favoritesService.clearAll();
      await historyService.syncNow();
      await favoritesService.syncNow();
    }

    // Recorded after the work, and only if it did not throw: marking first
    // would suppress the prompt on the next login even though nothing merged.
    await settingsService.markLocalDataReconciled(userId);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppTheme.backgroundColor,
      body: SafeArea(
        child: LayoutBuilder(
          builder: (context, constraints) {
            return SingleChildScrollView(
              padding: const EdgeInsets.all(24),
              child: ConstrainedBox(
                constraints: BoxConstraints(
                  minHeight: constraints.maxHeight - 48,
                ),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 400),
                    child: Form(
                      key: _formKey,
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          // Logo / Title
                          const Icon(
                            Icons.movie_filter,
                            size: 80,
                            color: AppTheme.primaryColor,
                          ),
                          const SizedBox(height: 16),
                          const Text(
                            'Kadrbox',
                            style: TextStyle(
                              fontSize: 32,
                              fontWeight: FontWeight.bold,
                              color: Colors.white,
                            ),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: 8),
                          Text(
                            'Увійдіть в свій акаунт',
                            style: TextStyle(
                              fontSize: 16,
                              color: Colors.white.withValues(alpha: 0.7),
                            ),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: 40),

                          // Email field
                          TextFormField(
                            controller: _emailController,
                            keyboardType: TextInputType.emailAddress,
                            autocorrect: false,
                            textInputAction: TextInputAction.next,
                            decoration: InputDecoration(
                              labelText: 'Email',
                              hintText: 'example@email.com',
                              prefixIcon: const Icon(Icons.email_outlined),
                              filled: true,
                              fillColor: AppTheme.darkCard,
                              border: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(12),
                                borderSide: BorderSide.none,
                              ),
                              enabledBorder: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(12),
                                borderSide: BorderSide(
                                  color: Colors.white.withValues(alpha: 0.1),
                                ),
                              ),
                              focusedBorder: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(12),
                                borderSide: const BorderSide(
                                  color: AppTheme.primaryColor,
                                ),
                              ),
                            ),
                            validator: (value) {
                              if (value == null || value.isEmpty) {
                                return 'Введіть email';
                              }
                              if (!value.contains('@')) {
                                return 'Невірний формат email';
                              }
                              return null;
                            },
                          ),
                          const SizedBox(height: 16),

                          // Password field
                          TextFormField(
                            controller: _passwordController,
                            obscureText: _obscurePassword,
                            textInputAction: TextInputAction.done,
                            onFieldSubmitted: (_) => _login(),
                            decoration: InputDecoration(
                              labelText: 'Пароль',
                              prefixIcon: const Icon(Icons.lock_outlined),
                              suffixIcon: IconButton(
                                icon: Icon(
                                  _obscurePassword
                                      ? Icons.visibility_outlined
                                      : Icons.visibility_off_outlined,
                                ),
                                onPressed: () {
                                  setState(
                                    () => _obscurePassword = !_obscurePassword,
                                  );
                                },
                              ),
                              filled: true,
                              fillColor: AppTheme.darkCard,
                              border: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(12),
                                borderSide: BorderSide.none,
                              ),
                              enabledBorder: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(12),
                                borderSide: BorderSide(
                                  color: Colors.white.withValues(alpha: 0.1),
                                ),
                              ),
                              focusedBorder: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(12),
                                borderSide: const BorderSide(
                                  color: AppTheme.primaryColor,
                                ),
                              ),
                            ),
                            validator: (value) {
                              if (value == null || value.isEmpty) {
                                return 'Введіть пароль';
                              }
                              if (value.length < 6) {
                                return 'Пароль має бути щонайменше 6 символів';
                              }
                              return null;
                            },
                          ),

                          // Forgot password
                          Align(
                            alignment: Alignment.centerRight,
                            child: TextButton(
                              onPressed: () => _showForgotPasswordDialog(),
                              child: const Text('Забули пароль?'),
                            ),
                          ),

                          // Error message
                          if (_error != null) ...[
                            const SizedBox(height: 8),
                            Container(
                              padding: const EdgeInsets.all(12),
                              decoration: BoxDecoration(
                                color: Colors.red.withValues(alpha: 0.1),
                                borderRadius: BorderRadius.circular(8),
                                border: Border.all(
                                  color: Colors.red.withValues(alpha: 0.3),
                                ),
                              ),
                              child: Row(
                                children: [
                                  const Icon(
                                    Icons.error_outline,
                                    color: Colors.red,
                                    size: 20,
                                  ),
                                  const SizedBox(width: 8),
                                  Expanded(
                                    child: Text(
                                      _error!,
                                      style: const TextStyle(color: Colors.red),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ],

                          const SizedBox(height: 24),

                          // Login button
                          SizedBox(
                            height: 50,
                            child: ElevatedButton(
                              onPressed: _isLoading ? null : _login,
                              style: ElevatedButton.styleFrom(
                                backgroundColor: AppTheme.primaryColor,
                                foregroundColor: Colors.white,
                                shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(12),
                                ),
                                disabledBackgroundColor: AppTheme.primaryColor
                                    .withValues(alpha: 0.5),
                              ),
                              child: _isLoading
                                  ? const SizedBox(
                                      height: 24,
                                      width: 24,
                                      child: CircularProgressIndicator(
                                        strokeWidth: 2,
                                        color: Colors.white,
                                      ),
                                    )
                                  : const Text(
                                      'Увійти',
                                      style: TextStyle(
                                        fontSize: 16,
                                        fontWeight: FontWeight.bold,
                                      ),
                                    ),
                            ),
                          ),

                          const SizedBox(height: 24),

                          // Divider
                          Row(
                            children: [
                              Expanded(
                                child: Divider(
                                  color: Colors.white.withValues(alpha: 0.2),
                                ),
                              ),
                              Padding(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 16,
                                ),
                                child: Text(
                                  'або',
                                  style: TextStyle(
                                    color: Colors.white.withValues(alpha: 0.5),
                                  ),
                                ),
                              ),
                              Expanded(
                                child: Divider(
                                  color: Colors.white.withValues(alpha: 0.2),
                                ),
                              ),
                            ],
                          ),

                          const SizedBox(height: 24),

                          // Register link
                          Row(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Text(
                                'Немає акаунту? ',
                                style: TextStyle(
                                  color: Colors.white.withValues(alpha: 0.7),
                                ),
                              ),
                              GestureDetector(
                                onTap: () => context.go('/register'),
                                child: const Text(
                                  'Зареєструватися',
                                  style: TextStyle(
                                    color: AppTheme.primaryColor,
                                    fontWeight: FontWeight.bold,
                                  ),
                                ),
                              ),
                            ],
                          ),

                          const SizedBox(height: 16),

                          // Skip for now
                          TextButton(
                            onPressed: () => context.go('/'),
                            child: Text(
                              'Продовжити без акаунту',
                              style: TextStyle(
                                color: Colors.white.withValues(alpha: 0.5),
                              ),
                            ),
                          ),

                          const SizedBox(height: 32),

                          // Social login divider
                          Row(
                            children: [
                              Expanded(
                                child: Divider(
                                  color: Colors.white.withValues(alpha: 0.1),
                                ),
                              ),
                              Padding(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 16,
                                ),
                                child: Text(
                                  'Або увійдіть через',
                                  style: TextStyle(
                                    fontSize: 12,
                                    color: Colors.white.withValues(alpha: 0.4),
                                  ),
                                ),
                              ),
                              Expanded(
                                child: Divider(
                                  color: Colors.white.withValues(alpha: 0.1),
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 24),

                          // Social buttons
                          Row(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              _buildSocialButton(
                                icon: Icons.g_mobiledata,
                                label: 'Google',
                                color: Colors.redAccent,
                                onTap: () => _handleSocialLogin('google'),
                              ),
                              const SizedBox(width: 12),
                              _buildSocialButton(
                                icon: MaterialCommunityIcons.discord,
                                label: 'Discord',
                                color: const Color(0xFF5865F2),
                                onTap: () => _handleSocialLogin('discord'),
                              ),
                              const SizedBox(width: 12),
                              _buildSocialButton(
                                icon: FontAwesome.telegram,
                                label: 'Telegram',
                                color: const Color(0xFF229ED9),
                                onTap: () => _handleSocialLogin('telegram'),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _buildSocialButton({
    required IconData icon,
    required String label,
    Color? color,
    required VoidCallback onTap,
  }) {
    return InkWell(
      onTap: _isLoading ? null : onTap,
      borderRadius: BorderRadius.circular(12),
      child: Container(
        width: 95,
        padding: const EdgeInsets.symmetric(vertical: 12),
        decoration: BoxDecoration(
          color: AppTheme.darkCard,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: color != null
                ? color.withValues(alpha: 0.3)
                : Colors.white.withValues(alpha: 0.1),
          ),
        ),
        child: Column(
          children: [
            Icon(icon, color: color ?? Colors.white, size: 28),
            const SizedBox(height: 4),
            Text(
              label,
              style: const TextStyle(fontSize: 12, color: Colors.white),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _handleSocialLogin(String provider) async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      await _authService.socialSignIn(provider);
      if (mounted) {
        await _checkLocalDataAndSync();
        if (mounted) {
          context.go('/');
        }
      }
    } catch (e) {
      setState(() => _error = _authService.error ?? e.toString());
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  void _showForgotPasswordDialog() {
    showDialog(
      context: context,
      builder: (_) => _ForgotPasswordDialog(
        initialEmail: _emailController.text,
        onSubmit: (email) async {
          try {
            await _authService.resetPassword(email);
            return null;
          } catch (e) {
            return _authService.error ?? e.toString();
          }
        },
      ),
    );
  }
}

/// The "reset password" dialog.
///
/// It owns its [TextEditingController] and stays open while the request is in
/// flight, so the controller's lifetime is tied to the [TextField] that listens
/// to it. Creating the controller in the caller (the original version) leaked
/// it; disposing it after `showDialog` would be worse, because the future
/// completes while the dialog is still animating out and the still-mounted
/// TextField would then talk to a disposed controller.
///
/// [onSubmit] returns `null` on success or a human-readable error to show
/// inline, which keeps the retry path inside the dialog.
class _ForgotPasswordDialog extends StatefulWidget {
  const _ForgotPasswordDialog({
    required this.initialEmail,
    required this.onSubmit,
  });

  final String initialEmail;
  final Future<String?> Function(String email) onSubmit;

  @override
  State<_ForgotPasswordDialog> createState() => _ForgotPasswordDialogState();
}

class _ForgotPasswordDialogState extends State<_ForgotPasswordDialog> {
  final _controller = TextEditingController();
  String? _error;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    _controller.text = widget.initialEmail;
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final problem = await widget.onSubmit(_controller.text.trim());
    // mounted, not this State's `mounted`: the awaits below can outlive the
    // dialog if the route is popped from elsewhere.
    if (!mounted) return;
    setState(() => _busy = false);
    if (problem != null) {
      setState(() => _error = problem);
      return;
    }
    Navigator.pop(context);
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text(
          'Якщо акаунт існує, ми надіслали на нього лист для '
          'скидання пароля.',
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppTheme.darkCard,
      title: const Text('Скинути пароль'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Text(
            'Введіть email, на який буде надіслано посилання для скидання пароля',
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _controller,
            enabled: !_busy,
            decoration: InputDecoration(
              labelText: 'Email',
              errorText: _error,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
              ),
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.pop(context),
          child: const Text('Скасувати'),
        ),
        ElevatedButton(
          onPressed: _busy ? null : _submit,
          child: const Text('Надіслати'),
        ),
      ],
    );
  }
}
