import 'dart:io';
import 'package:equatable/equatable.dart';

/// Information about a new app update
class UpdateInfo extends Equatable {
  final Map<String, PlatformUpdateInfo> platforms;

  const UpdateInfo({required this.platforms});

  factory UpdateInfo.fromJson(Map<String, dynamic> json) {
    final platformsMap = <String, PlatformUpdateInfo>{};
    if (json['platforms'] != null) {
      (json['platforms'] as Map<String, dynamic>).forEach((key, value) {
        platformsMap[key] = PlatformUpdateInfo.fromJson(
          value as Map<String, dynamic>,
        );
      });
    }
    return UpdateInfo(platforms: platformsMap);
  }

  PlatformUpdateInfo? get forCurrentPlatform {
    if (Platform.isAndroid) return platforms['android'];
    if (Platform.isWindows) return platforms['windows'];
    return null;
  }

  @override
  List<Object?> get props => [platforms];
}

class PlatformUpdateInfo extends Equatable {
  final int versionCode;
  final String versionName;
  final String releaseNotes;
  final String url;
  final String sha256;
  final int? minVersionCode;

  const PlatformUpdateInfo({
    required this.versionCode,
    required this.versionName,
    required this.releaseNotes,
    required this.url,
    required this.sha256,
    this.minVersionCode,
  });

  factory PlatformUpdateInfo.fromJson(Map<String, dynamic> json) {
    return PlatformUpdateInfo(
      versionCode: json['versionCode'] as int,
      versionName: json['versionName'] as String,
      releaseNotes: json['releaseNotes'] as String? ?? '',
      url: json['url'] as String,
      sha256: json['sha256'] as String? ?? '',
      minVersionCode: json['minVersionCode'] as int?,
    );
  }

  bool isNewerThan(int currentCode) => versionCode > currentCode;

  bool isForcedUpdate(int currentCode) {
    if (minVersionCode == null) return false;
    return currentCode < minVersionCode!;
  }

  @override
  List<Object?> get props => [
    versionCode,
    versionName,
    releaseNotes,
    url,
    sha256,
    minVersionCode,
  ];
}
