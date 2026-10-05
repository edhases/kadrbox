import 'package:dio/dio.dart';
import 'dart:io';
import 'dart:convert';

void main() async {
  final dio = Dio();
  final htmlFile = File('test/desktop_ua_lang_result.html');

  try {
    print('Fetching main page with Accept-Language...');
    final response = await dio.get(
      'https://uafix.net/serials/gra-v-kalmara-viprobuvannja-squid-game-the-challenge/',
      options: Options(
        headers: {
          'User-Agent':
              'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
          'Accept-Language': 'uk-UA,uk;q=0.9,en-US;q=0.8,en;q=0.7',
        },
        responseType: ResponseType.plain,
      ),
    );
    await htmlFile.writeAsString(response.data.toString());
    print('Saved HTML to ${htmlFile.path}');

    // Continue with hash extraction from THIS new file
    final htmlContent = response.data.toString();

    // Extract AMSP.loadAsset hashes
    final regex = RegExp(
      r'AMSP\.loadAsset\s*\(\s*["'
      ']([0-9a-f]{32})["'
      ']',
      caseSensitive: false,
    );
    final matches = regex.allMatches(htmlContent);
    final hashesList = matches
        .map((m) => m.group(1)!.toLowerCase())
        .toSet()
        .toList(); // Unique and lower case params

    print('Found hashes: $hashesList');

    if (hashesList.isEmpty) {
      print('No hashes found.');
      return;
    }

    final relPlc = base64.encode(utf8.encode(hashesList.join(',')));
    print('relPlc: $relPlc');

    // Loop through all hashes
    final domains = ['franecki.net', 'ad2the.net', 'franeski.net'];
    for (var targetHash in hashesList) {
      for (var domain in domains) {
        print('\nFetching pack for hash: $targetHash from $domain');

        // Construct query parameters...
        final adwuid = '69875411067e812944873658';

        final StringBuffer qs = StringBuffer('1');
        qs.write('&dmpguid=$adwuid');
        qs.write('&adwuid=$adwuid');
        qs.write('&ct=4g');
        qs.write('&webp=1');
        qs.write('&sw=1920');
        qs.write('&sh=1080');
        qs.write('&ww=1920');
        qs.write('&wh=1080');
        qs.write('&fp=fake_fp_value');
        qs.write('&fp3=0');
        qs.write('&libjs=1');
        qs.write('&dc_rid=698753bb0af0432637636286');
        qs.write('&relPlc=$relPlc');

        final url =
            'https://$domain/assets/pack/$targetHash.js?${qs.toString()}';
        print('Fetching URL: $url');

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

          print('Response status: ${response.statusCode}');
          if (response.statusCode == 200) {
            final len = response.data.toString().length;
            print('Response length: $len');
            if (len > 0) {
              print(
                'Content preview: ${response.data.toString().substring(0, min(200, len))}',
              );
              final successFile = File('test/pack_${targetHash}_$domain.js');
              await successFile.writeAsString(response.data.toString());
              print('Saved to ${successFile.path}');
            }
          }
        } catch (e) {
          print('Error fetching hash $targetHash from $domain: $e');
        }
      }
    }

    // 3. Fetch collateral APIs
    print('\nFetching s.schulist.link ...');
    try {
      final resp1 = await dio.get(
        'https://s.schulist.link/dc?rid=VUE=::698753bb0af0432637636286',
      );
      print('s.schulist.link status: ${resp1.statusCode}');
      print('s.schulist.link body: ${resp1.data}');
    } catch (e) {
      print('Error fetching s.schulist.link: $e');
    }

    print('\nFetching reichelcormier.bid ...');
    try {
      final resp2 = await dio.get(
        'https://reichelcormier.bid/candy/?method=adwuid&c=&r=0.123456',
        options: Options(
          headers: {
            'User-Agent':
                'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'Origin': 'https://uafix.net',
            'Referer': 'https://uafix.net/',
          },
          responseType: ResponseType.plain, // expecting JSON probably
        ),
      );
      print('reichelcormier.bid status: ${resp2.statusCode}');
      print('reichelcormier.bid body: ${resp2.data}');
    } catch (e) {
      print('Error fetching reichelcormier.bid: $e');
    }
  } catch (e) {
    print('Error: $e');
    if (e is DioException) {
      print('DioError response: ${e.response?.data}');
      print('DioError headers: ${e.response?.headers}');
    }
  }
}

int min(int a, int b) => a < b ? a : b;
