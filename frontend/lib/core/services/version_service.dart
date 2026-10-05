import 'dart:convert';
import 'package:flutter/services.dart';

class VersionService {
  static const String _versionFilePath = 'assets/version.json';

  static String? _versionName;
  static int? _versionCode;

  /// Loads version data from version.json
  static Future<void> init() async {
    try {
      final jsonString = await rootBundle.loadString(_versionFilePath);
      final Map<String, dynamic> jsonData = jsonDecode(jsonString);

      _versionName = jsonData['versionName'] as String?;

      // Handle both integer and string version code definitions
      final vc = jsonData['versionCode'];
      if (vc is int) {
        _versionCode = vc;
      } else if (vc is String) {
        _versionCode = int.tryParse(vc);
      }
    } catch (e) {
      // Fallback or log if missing
      _versionName = 'Unknown';
      _versionCode = 0;
    }
  }

  static String get versionName => _versionName ?? '1.0.0';
  static int get versionCode => _versionCode ?? 1;
}
