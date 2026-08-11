import 'package:flutter_test/flutter_test.dart';
import 'package:preuzmi_mobile/util/i18n.dart';

void main() {
  // Keep the global script flag from leaking between tests.
  tearDown(() => I18n.cyrillic = false);

  group('toCyrillic', () {
    test('maps basic Serbian Latin words', () {
      expect(toCyrillic('Računi'), 'Рачуни');
      expect(toCyrillic('Podešavanja'), 'Подешавања');
    });

    test('handles digraphs before single letters', () {
      expect(toCyrillic('Ljubav'), 'Љубав');
      expect(toCyrillic('njiva'), 'њива');
      expect(toCyrillic('džak'), 'џак');
    });

    test('preserves whitespace and spacing', () {
      expect(toCyrillic('novi  račun'), 'нови  рачун');
    });

    test('returns empty string unchanged', () {
      expect(toCyrillic(''), '');
    });

    test('keeps technical tokens with digits, handles, and hosts in Latin', () {
      expect(toCyrillic('A1'), 'A1');
      expect(toCyrillic('587'), '587');
      expect(toCyrillic('06-2026'), '06-2026');
      expect(toCyrillic('@BotFather'), '@BotFather');
      expect(toCyrillic('smtp.gmail.com'), 'smtp.gmail.com');
    });

    // KNOWN GAP: `_keepLatin` strips leading punctuation before its checks, so a
    // bare command handle like `/newbot` loses the `/` that would mark it
    // technical and gets transliterated. `@BotFather` only survives because
    // `botfather` is in the brand list. This test documents current behavior;
    // if the handle-preservation is fixed, flip these expectations.
    test('does not (yet) preserve bare slash-prefixed command handles', () {
      expect(toCyrillic('/newbot'), '/неwбот');
    });

    test('keeps short ALL-CAPS acronyms in Latin', () {
      expect(toCyrillic('SMTP'), 'SMTP');
      expect(toCyrillic('QR'), 'QR');
      expect(toCyrillic('IMAP'), 'IMAP');
    });

    test('keeps known brand words in Latin regardless of case', () {
      expect(toCyrillic('telegram'), 'telegram');
      expect(toCyrillic('Gmail'), 'Gmail');
      expect(toCyrillic('token'), 'token');
    });

    test('transliterates around preserved tokens and trailing punctuation', () {
      // "poruku" transliterates, "@BotFather" stays Latin.
      expect(toCyrillic('poruku @BotFather'), 'поруку @BotFather');
      // Trailing sentence punctuation must not turn a word into a "host".
      expect(toCyrillic('povezuje.'), 'повезује.');
    });
  });

  group('tr / TrString.t', () {
    test('is a no-op in Latin mode', () {
      I18n.cyrillic = false;
      expect(tr('Računi'), 'Računi');
      expect('Računi'.t, 'Računi');
    });

    test('transliterates in Cyrillic mode', () {
      I18n.cyrillic = true;
      expect(tr('Računi'), 'Рачуни');
      expect('Računi'.t, 'Рачуни');
    });
  });
}
