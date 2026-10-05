// Helpers to mock file system and platform APIs for tests.
// Examples: path_provider mock values, FilePicker stubs, share_plus stub.

import 'dart:io';

import 'package:path_provider_platform_interface/path_provider_platform_interface.dart';

// Set a mock documents directory by pointing to a temporary directory
Future<void> setMockDocumentsDirectory(Directory dir) async {
  PathProviderPlatform.instance = _FakePathProvider(dir);
}

class _FakePathProvider extends PathProviderPlatform {
  final Directory _dir;

  _FakePathProvider(this._dir);

  @override
  Future<String?> getApplicationDocumentsPath() async {
    return _dir.path;
  }
}
