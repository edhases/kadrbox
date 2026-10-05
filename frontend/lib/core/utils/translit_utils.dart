/// Utility class for transliterating Ukrainian and Russian characters to English.
/// Useful for generating safe filenames.
class TranslitUtils {
  static const Map<String, String> _mapping = {
    'а': 'a',
    'б': 'b',
    'в': 'v',
    'г': 'g',
    'ґ': 'g',
    'д': 'd',
    'е': 'e',
    'є': 'ye',
    'ж': 'zh',
    'з': 'z',
    'и': 'y',
    'і': 'i',
    'ї': 'yi',
    'й': 'y',
    'к': 'k',
    'л': 'l',
    'м': 'm',
    'н': 'n',
    'о': 'o',
    'п': 'p',
    'р': 'r',
    'с': 's',
    'т': 't',
    'у': 'u',
    'ф': 'f',
    'х': 'kh',
    'ц': 'ts',
    'ч': 'ch',
    'ш': 'sh',
    'щ': 'shch',
    'ь': '',
    'ю': 'yu',
    'я': 'ya',
    'А': 'A',
    'Б': 'B',
    'В': 'V',
    'Г': 'G',
    'Ґ': 'G',
    'Д': 'D',
    'Е': 'E',
    'Є': 'Ye',
    'Ж': 'Zh',
    'З': 'Z',
    'И': 'Y',
    'І': 'I',
    'Ї': 'Yi',
    'Й': 'Y',
    'К': 'K',
    'Л': 'L',
    'М': 'M',
    'Н': 'N',
    'О': 'O',
    'П': 'P',
    'Р': 'R',
    'С': 'S',
    'Т': 'T',
    'У': 'U',
    'Ф': 'F',
    'Х': 'Kh',
    'Ц': 'Ts',
    'Ч': 'Ch',
    'Ш': 'Sh',
    'Щ': 'Shch',
    'Ь': '',
    'Ю': 'Yu',
    'Я': 'Ya',
  };

  /// Transliterates text and sanitizes it for use in filenames.
  static String translit(String text) {
    var result = '';
    for (var i = 0; i < text.length; i++) {
      final char = text[i];
      result += _mapping[char] ?? char;
    }

    // Sanitize: remove non-alphanumeric (except dots, dashes, underscores)
    // Replace spaces with underscores
    return result
        .replaceAll(' ', '_')
        .replaceAll(RegExp(r'[^a-zA-Z0-9\.\-_]'), '')
        .replaceAll(RegExp(r'_{2,}'), '_') // Remove double underscores
        .replaceAll(RegExp(r'^\.|\.$'), ''); // Remove leading/trailing dots
  }
}
