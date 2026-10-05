import 'dart:io';
import 'package:dio/dio.dart';

void main() async {
  final dio = Dio();
  final url = 'https://zetvideo.net/vod/4303';
  final filename = 'test/zetvideo_result.html';

  print('Fetching $url...');
  try {
    final response = await dio.get(
      url,
      options: Options(
        headers: {
          'User-Agent':
              'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
          'Referer': 'https://uafix.net/',
        },
        responseType: ResponseType.plain,
        validateStatus: (status) => true,
      ),
    );

    await File(filename).writeAsString(response.data.toString());
    print('Saved to $filename');
    print('Status: ${response.statusCode}');

    final content = response.data.toString();
    if (content.contains('ashdi')) print('FOUND: ashdi');
    if (content.contains('playerjs')) print('FOUND: playerjs');
    if (content.contains('.m3u8')) print('FOUND: .m3u8');
    if (content.contains('.mp4')) print('FOUND: .mp4');
  } catch (e) {
    print('Error: $e');
  }
}
