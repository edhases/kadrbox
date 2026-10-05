@Tags(['integration'])
@Skip('Live network fetch script that mutates fixtures')
import 'dart:io';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

final mobileUA =
    'Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1';
final desktopUA =
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36';

Future<void> fetchAndSave(String url, String ua, String filename) async {
  print('Fetching $url with UA: ${ua.substring(0, 20)}...');
  try {
    final dio = Dio();
    final response = await dio.get(
      url,
      options: Options(
        headers: {'User-Agent': ua},
        responseType: ResponseType.plain,
      ),
    );

    if (response.statusCode == 200) {
      final content = response.data.toString();
      final file = File(filename);
      await file.writeAsString(content);
      print('Saved to $filename (${content.length} bytes)');

      // Check for AMSP
      if (content.contains('AMSP')) {
        print('  -> Contains AMSP protection');
      } else {
        print('  -> NO AMSP found');
      }

      // Check for iframes
      if (content.contains('<iframe')) {
        print('  -> Contains <iframe>');
      } else {
        print('  -> NO <iframe> found');
      }
    } else {
      print('Failed to fetch: ${response.statusCode}');
    }
  } catch (e) {
    print('Error: $e');
  }
}

void main() async {
  final targetUrl =
      'https://src-a.example/serials/gra-v-kalmara-viprobuvannja-squid-game-the-challenge/'; // Specific movie

  await fetchAndSave(targetUrl, desktopUA, 'test/desktop_ua_result.html');
  await fetchAndSave(targetUrl, mobileUA, 'test/mobile_ua_result.html');

  print('\n--- Pagination Test ---');
  // Search for common term "Harry"
  final searchUrlBase =
      'https://src-a.example/index.php?do=search&subaction=search&story=Harry';

  await fetchAndSave(
    '$searchUrlBase&search_start=1',
    desktopUA,
    'test/search_page_1.html',
  );
  await fetchAndSave(
    '$searchUrlBase&search_start=2',
    desktopUA,
    'test/search_page_2.html',
  );

  // Compare page 1 and 2
  final p1 = await File('test/search_page_1.html').readAsString();
  final p2 = await File('test/search_page_2.html').readAsString();

  if (p1 == p2) {
    print(
      'Pagination FAIL: Page 1 and 2 are identical. search_start might be ignored or handled incorrectly.',
    );
  } else {
    print('Pagination SUCCESS: Page 1 and 2 content differs.');
  }
}
