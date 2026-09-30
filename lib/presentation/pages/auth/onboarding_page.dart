import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../theme/app_theme.dart';

/// First-launch screen: sign in, register, or continue without an account.
class OnboardingPage extends StatelessWidget {
  static const prefsKey = 'onboarding_seen_v1';

  /// True when the user already made the first-launch choice (or signed in).
  static bool isSeen(SharedPreferences prefs) =>
      prefs.getBool(prefsKey) ?? false;

  static Future<void> markSeen() async {
    try {
      await GetIt.instance<SharedPreferences>().setBool(prefsKey, true);
    } catch (_) {
      // Prefs unavailable in tests — ignore.
    }
  }

  const OnboardingPage({super.key});

  Future<void> _choose(BuildContext context, String location) async {
    await markSeen();
    if (context.mounted) context.go(location);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final isDark = theme.brightness == Brightness.dark;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.symmetric(horizontal: 32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 72,
                  height: 72,
                  decoration: BoxDecoration(
                    color: AppTheme.primaryColor,
                    borderRadius: BorderRadius.circular(20),
                  ),
                  alignment: Alignment.center,
                  child: const Text(
                    'O',
                    style: TextStyle(
                      fontSize: 36,
                      fontWeight: FontWeight.w800,
                      color: Colors.white,
                    ),
                  ),
                ),
                const SizedBox(height: 24),
                const Text(
                  'Oxide Film',
                  style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800),
                ),
                const SizedBox(height: 8),
                Text(
                  'Фільми, серіали та аніме українською.\nУвійдіть, щоб синхронізувати історію й обране між пристроями.',
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 14,
                    height: 1.5,
                    color: isDark ? Colors.white70 : Colors.black54,
                  ),
                ),
                const SizedBox(height: 36),
                SizedBox(
                  width: double.infinity,
                  child: ElevatedButton(
                    onPressed: () => _choose(context, '/login'),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppTheme.primaryColor,
                      padding: const EdgeInsets.symmetric(vertical: 16),
                    ),
                    child: const Text('Увійти'),
                  ),
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  child: OutlinedButton(
                    onPressed: () => _choose(context, '/register'),
                    style: OutlinedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 16),
                    ),
                    child: const Text('Створити акаунт'),
                  ),
                ),
                const SizedBox(height: 8),
                TextButton(
                  onPressed: () => _choose(context, '/'),
                  child: const Text('Продовжити без акаунту'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
