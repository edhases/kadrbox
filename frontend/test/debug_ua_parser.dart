import 'dart:io';

void main() {
  final file = File('test/useragents_me.html');
  if (!file.existsSync()) {
    print('File not found: ${file.path}');
    return;
  }
  final html = file.readAsStringSync();

  print('Testing Current Logic...');
  final uaRegex = RegExp(r'Mozilla/5\.0 [^"<> \n\r\t\x27]+');
  final matches = uaRegex.allMatches(html);
  final newUAs = matches
      .map((m) => m.group(0)!.trim())
      .where((ua) => ua.length > 50 && ua.length < 300)
      .toSet()
      .toList();
  print('Found ${newUAs.length} UAs with current logic.');

  print('\nTesting New Logic (Textarea Regex)...');
  final taRegex = RegExp(
    r'<textarea class="form-control ua-textarea">([^<]+)</textarea>',
  );
  final taMatches = taRegex.allMatches(html);
  final parsedUAs = taMatches
      .map((m) => m.group(1)!.trim())
      .where((ua) => ua.length > 50 && ua.length < 300)
      .toSet()
      .toList();

  print('Found ${parsedUAs.length} UAs with new logic.');
  if (parsedUAs.isNotEmpty) {
    print('Sample UAs:');
    parsedUAs.take(3).forEach((ua) => print(' - $ua'));

    // Check for Chrome
    final chromeUAs = parsedUAs
        .where((ua) => ua.contains('Chrome') && !ua.contains('Mobile'))
        .toList();
    print('Found ${chromeUAs.length} Desktop Chrome UAs.');
  }
}
