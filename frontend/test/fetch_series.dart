import 'dart:io';
import 'package:http/http.dart' as http;

void main() async {
  final url = Uri.parse('https://uafix.net/serials/garri-gole/');
  final headers = {
    'User-Agent':
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    'Accept':
        'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7',
    'Accept-Language': 'uk-UA,uk;q=0.9,en-US;q=0.8,en;q=0.7',
  };

  print('Fetching $url...');
  try {
    final response = await http.get(url, headers: headers);
    print('Status: ${response.statusCode}');

    final file = File('test/series_result.html');
    await file.writeAsString(response.body);
    print('Saved to ${file.path}');

    if (response.body.contains('iframe')) {
      print('FOUND: iframe');
      final iframeRegExp = RegExp(r'<iframe[^>]+src="([^"]+)"');
      final match = iframeRegExp.firstMatch(response.body);
      if (match != null) {
        print('Iframe SRC: ${match.group(1)}');
      }
    } else {
      print('NOT FOUND: iframe');
    }

    if (response.body.contains('AMSP')) {
      print('FOUND: AMSP');
    }

    if (response.body.contains('playerjs')) {
      print('FOUND: playerjs');
    }
  } catch (e) {
    print('Error: $e');
  }
}
