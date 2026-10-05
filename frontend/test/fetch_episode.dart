import 'dart:io';
import 'package:dio/dio.dart';

void main() async {
  final dio = Dio();
  final url =
      'https://uafix.net/serials/gra-v-kalmara-viprobuvannja-squid-game-the-challenge/season-01-episode-01/';
  final filename = 'test/episode_01_result.html';

  print('Fetching $url...');
  try {
    final response = await dio.get(
      url,
      options: Options(
        headers: {
          'User-Agent':
              'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
          'Accept-Language': 'uk-UA,uk;q=0.9,en-US;q=0.8,en;q=0.7',
        },
        responseType: ResponseType.plain,
      ),
    );

    await File(filename).writeAsString(response.data.toString());
    print('Saved to $filename');

    final content = response.data.toString();
    if (content.contains('ashdi')) {
      print('FOUND: ashdi');
    } else {
      print('NOT FOUND: ashdi');
    }

    if (content.contains('<iframe')) {
      print('FOUND: iframe');
    } else {
      print('NOT FOUND: iframe');
    }

    if (content.contains('AMSP')) {
      print('FOUND: AMSP');
    } else {
      print('NOT FOUND: AMSP');
    }
  } catch (e) {
    print('Error: $e');
  }
}
