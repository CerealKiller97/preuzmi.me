/// Lightweight Serbian Latin → Cyrillic layer for the whole UI.
///
/// The app's source strings are written in Serbian Latin. When the user picks
/// Ćirilica, [tr] transliterates them at render time. Technical tokens (URLs,
/// acronyms like SMTP/QR/IMAP, tokens with digits like A1/587/06-2026, and
/// `@`/`/`-prefixed handles) are left in Latin so they stay correct.
class I18n {
  I18n._();

  /// Current script, kept in sync with `appConfigProvider.cyrillic` by each
  /// screen's build (via `ref.watch`), so a toggle rebuilds and re-renders.
  static bool cyrillic = false;
}

/// Transliterate [s] to the active script.
String tr(String s) => I18n.cyrillic ? toCyrillic(s) : s;

extension TrString on String {
  /// `'Računi'.t` → 'Рачуни' in Cyrillic mode, unchanged in Latin.
  String get t => tr(this);
}

// Digraphs must be replaced before single letters.
const Map<String, String> _digraphs = {
  'Lj': 'Љ',
  'lj': 'љ',
  'LJ': 'Љ',
  'Nj': 'Њ',
  'nj': 'њ',
  'NJ': 'Њ',
  'Dž': 'Џ',
  'dž': 'џ',
  'DŽ': 'Џ',
};

const Map<String, String> _letters = {
  'a': 'а',
  'b': 'б',
  'v': 'в',
  'g': 'г',
  'd': 'д',
  'đ': 'ђ',
  'e': 'е',
  'ž': 'ж',
  'z': 'з',
  'i': 'и',
  'j': 'ј',
  'k': 'к',
  'l': 'л',
  'm': 'м',
  'n': 'н',
  'o': 'о',
  'p': 'п',
  'r': 'р',
  's': 'с',
  't': 'т',
  'ć': 'ћ',
  'u': 'у',
  'f': 'ф',
  'h': 'х',
  'c': 'ц',
  'č': 'ч',
  'š': 'ш',
  'A': 'А',
  'B': 'Б',
  'V': 'В',
  'G': 'Г',
  'D': 'Д',
  'Đ': 'Ђ',
  'E': 'Е',
  'Ž': 'Ж',
  'Z': 'З',
  'I': 'И',
  'J': 'Ј',
  'K': 'К',
  'L': 'Л',
  'M': 'М',
  'N': 'Н',
  'O': 'О',
  'P': 'П',
  'R': 'Р',
  'S': 'С',
  'T': 'Т',
  'Ć': 'Ћ',
  'U': 'У',
  'F': 'Ф',
  'H': 'Х',
  'C': 'Ц',
  'Č': 'Ч',
  'Š': 'Ш',
};

/// Whitespace-delimited tokens that should stay Latin (technical / brand).
bool _keepLatin(String token) {
  if (token.isEmpty) return true;
  // Strip leading/trailing punctuation (sentence dots, quotes, brackets) so a
  // word like "povezuje." isn't mistaken for a host name.
  final core = token.replaceAll(
    RegExp(r'^[^A-Za-z0-9ČĆŽŠĐčćžšđ]+|[^A-Za-z0-9ČĆŽŠĐčćžšđ]+$'),
    '',
  );
  if (core.isEmpty) return true;
  // URLs, handles, paths, hosts, ports, codes — digits or internal separators.
  if (RegExp(r'[0-9@/:._<>]').hasMatch(core)) return true;
  // Short ALL-CAPS acronyms: SMTP, QR, IMAP, TLS, PDF, ID, IPS, SES, HTTP…
  if (core.length >= 2 &&
      core.length <= 5 &&
      core == core.toUpperCase() &&
      RegExp(r'^[A-Z]+$').hasMatch(core)) {
    return true;
  }
  // Latin-only technical/brand words that read wrong in Cyrillic.
  const brands = {
    'telegram',
    'botfather',
    'gmail',
    'outlook',
    'yahoo',
    'icloud',
    'mailgun',
    'starttls',
    'smtps',
    'getupdates',
    'username',
    'app',
    'password',
    'token',
    'chat',
    'bot',
    'json',
    'host',
    'port',
    'endpoint',
    'bucket',
    'region',
    'off',
    'per_receipt',
    'all_done',
    'local',
    's3',
  };
  if (brands.contains(core.toLowerCase())) return true;
  return false;
}

/// Serbian Latin → Cyrillic transliteration, preserving technical tokens.
String toCyrillic(String input) {
  if (input.isEmpty) return input;
  // splitMapJoin keeps the whitespace separators, so spacing is preserved.
  return input.splitMapJoin(
    RegExp(r'\s+'),
    onMatch: (m) => m.group(0)!,
    onNonMatch: (token) => _keepLatin(token) ? token : _mapToken(token),
  );
}

String _mapToken(String token) {
  var out = token;
  _digraphs.forEach((lat, cyr) => out = out.replaceAll(lat, cyr));
  final sb = StringBuffer();
  for (final ch in out.split('')) {
    sb.write(_letters[ch] ?? ch);
  }
  return sb.toString();
}
