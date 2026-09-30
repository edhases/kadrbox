/// Service for transliterating Ukrainian text to Latin script
///
/// Uses official Ukrainian transliteration standard (KMU 2010 / ISO 9).
/// Removes semantic English-Ukrainian pseudo-dictionaries and lossy Latin->Cyrillic
/// character replacement to prevent query corruption during search.
class TransliterationService {
  // Ukrainian to Latin mapping (KMU 2010 / Passport standard)
  static const Map<String, String> _ukToLat = {
    'а': 'a',
    'б': 'b',
    'в': 'v',
    'г': 'h',
    'ґ': 'g',
    'д': 'd',
    'е': 'e',
    'є': 'ie',
    'ж': 'zh',
    'з': 'z',
    'и': 'y',
    'і': 'i',
    'ї': 'i',
    'й': 'i',
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
    'ю': 'iu',
    'я': 'ia',
    // Russian letters that might appear in mixed metadata
    'ы': 'y',
    'э': 'e',
    'ё': 'io',
    'ъ': '',
  };

  /// Detect if text is primarily Cyrillic (Ukrainian/Cyrillic block)
  bool isCyrillic(String text) {
    if (text.isEmpty) return false;

    final cyrillicPattern = RegExp(r'[\u0400-\u04FF]');
    final latinPattern = RegExp(r'[a-zA-Z]');

    final cyrillicCount = cyrillicPattern.allMatches(text.toLowerCase()).length;
    final latinCount = latinPattern.allMatches(text.toLowerCase()).length;

    return cyrillicCount > latinCount;
  }

  /// Detect if text is primarily Latin
  bool isLatin(String text) {
    return !isCyrillic(text) && RegExp(r'[a-zA-Z]').hasMatch(text);
  }

  /// Transliterate Ukrainian text to Latin
  String ukrainianToLatin(String text) {
    final buffer = StringBuffer();
    final lower = text.toLowerCase();

    for (var i = 0; i < lower.length; i++) {
      final char = lower[i];
      buffer.write(_ukToLat[char] ?? char);
    }

    return buffer.toString();
  }

  /// Generate search variants for a query
  ///
  /// Returns list of valid interpretations of the query:
  /// - Original normalized query
  /// - For Cyrillic queries: clean Latin transliteration for latin-indexed providers
  List<String> generateSearchVariants(String query) {
    final normalized = normalizeQuery(query);
    if (normalized.isEmpty) return const [];
    final variants = <String>{normalized};

    if (isCyrillic(normalized)) {
      final latinVariant = ukrainianToLatin(normalized);
      if (latinVariant.isNotEmpty && latinVariant != normalized) {
        variants.add(latinVariant);
      }
    }

    return variants.toList();
  }

  /// Normalize query for consistent comparison
  ///
  /// - Lowercase
  /// - Trim whitespace
  /// - Collapse multiple spaces
  /// - Retain alphanumeric characters (Unicode Ukrainian/Cyrillic & Latin) and spaces
  String normalizeQuery(String query) {
    return query
        .toLowerCase()
        .trim()
        .replaceAll(RegExp(r'\s+'), ' ')
        .replaceAll(RegExp(r'[^\w\s\u0400-\u04FF]', unicode: true), '');
  }

  /// Check if two strings are likely referring to the same title
  bool areSimilarQueries(String a, String b) {
    final normalA = normalizeQuery(a);
    final normalB = normalizeQuery(b);

    if (normalA == normalB) return true;

    if (isCyrillic(a) && isLatin(b)) {
      return normalizeQuery(ukrainianToLatin(a)) == normalB;
    }
    if (isLatin(a) && isCyrillic(b)) {
      return normalA == normalizeQuery(ukrainianToLatin(b));
    }

    return false;
  }
}
