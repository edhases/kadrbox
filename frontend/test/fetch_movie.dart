import 'dart:io';
import 'package:dio/dio.dart';

void main() async {
  final dio = Dio();
  final url = 'https://uafix.net/films/nepruyemnosti-z-garri/';
  final filename = 'test/movie_result.html';

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
        validateStatus: (status) => true,
      ),
    );

    await File(filename).writeAsString(response.data.toString());
    print('Saved to $filename');
    print('Status: ${response.statusCode}');

    final content = response.data.toString();
    if (content.contains('<iframe')) print('FOUND: iframe');
    if (content.contains('ashdi')) print('FOUND: ashdi');
    if (content.contains('AMSP')) print('FOUND: AMSP');
    if (content.contains('playerjs')) print('FOUND: playerjs');
  } catch (e) {
    print('Error: $e');
  }
}
