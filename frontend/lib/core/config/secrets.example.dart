/// Secrets configuration template
///
/// Copy this file to secrets.dart and fill in your API keys
/// The secrets.dart file is listed in .gitignore
class Secrets {
  /// Supabase project URL
  /// Get from: https://supabase.com/dashboard/project/YOUR_PROJECT/settings/api
  static const String supabaseUrl = 'https://YOUR_PROJECT.supabase.co';

  /// Supabase anonymous/public key (safe to use with RLS enabled)
  /// Get from: https://supabase.com/dashboard/project/YOUR_PROJECT/settings/api
  static const String supabaseAnonKey = 'your_anon_key_here';

  // Add other API keys here as needed
  // static const String tmdbApiKey = 'your_tmdb_key';
}
