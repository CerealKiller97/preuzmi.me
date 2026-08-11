import 'package:flutter_test/flutter_test.dart';
import 'package:preuzmi_mobile/util/format.dart';

import '../support/fixtures.dart';

void main() {
  group('providerBrand', () {
    test('uppercases plain keys', () {
      expect(providerBrand('mts'), 'MTS');
      expect(providerBrand('eps'), 'EPS');
    });

    test('applies Latin overrides for keys that read wrong uppercased', () {
      expect(providerBrand('esanduce'), 'E-SANDUČE');
      expect(providerBrand('eupravnik'), 'E-UPRAVNIK');
    });

    test('is case-insensitive for override lookup', () {
      expect(providerBrand('ESANDUCE'), 'E-SANDUČE');
      expect(providerBrand('Esanduce'), 'E-SANDUČE');
    });
  });

  group('providerLabel', () {
    test('returns brand only when label is empty, null, or whitespace', () {
      expect(providerLabel('a1'), 'A1');
      expect(providerLabel('a1', ''), 'A1');
      expect(providerLabel('a1', '   '), 'A1');
    });

    test('appends a trimmed label', () {
      expect(providerLabel('a1', 'Mama'), 'A1 — Mama');
      expect(providerLabel('a1', '  Mama  '), 'A1 — Mama');
    });
  });

  group('receiptLabel', () {
    test('combines provider and label from the receipt', () {
      expect(
        receiptLabel(buildReceipt(provider: 'a1', label: 'Tata')),
        'A1 — Tata',
      );
      expect(receiptLabel(buildReceipt(provider: 'mts', label: '')), 'MTS');
    });
  });

  group('formatMoney', () {
    test('defaults to RSD with no decimals for null currency', () {
      expect(formatMoney(1234, null), contains('RSD'));
      expect(formatMoney(1234, null), isNot(contains(',00')));
      expect(formatMoney(1234, ''), contains('RSD'));
    });

    test('treats null amount as zero', () {
      expect(formatMoney(null, 'RSD'), formatMoney(0, 'RSD'));
    });

    test('honours a non-RSD currency symbol', () {
      final eur = formatMoney(50, 'EUR');
      expect(eur, contains('EUR'));
      expect(eur, isNot(contains('RSD')));
    });
  });

  group('formatDate', () {
    test('renders DD.MM.YYYY for a known timestamp', () {
      // 2026-06-12 12:00:00 UTC — pinned by constructing from local parts.
      final ts = DateTime(2026, 6, 12, 12).millisecondsSinceEpoch ~/ 1000;
      expect(formatDate(ts), '12.06.2026');
    });

    test('pads single-digit day and month', () {
      final ts = DateTime(2026, 1, 5, 12).millisecondsSinceEpoch ~/ 1000;
      expect(formatDate(ts), '05.01.2026');
    });

    test('returns empty for zero, negative, and null', () {
      expect(formatDate(0), '');
      expect(formatDate(-10), '');
      expect(formatDate(null), '');
    });
  });

  group('formatSize', () {
    test('renders bytes below 1 KB without decimals', () {
      expect(formatSize(0), '0 B');
      expect(formatSize(512), '512 B');
      expect(formatSize(1023), '1023 B');
    });

    test('scales into KB, MB, GB with one decimal', () {
      expect(formatSize(1536), '1.5 KB');
      expect(formatSize(1024 * 1024), '1.0 MB');
      expect(formatSize(1024 * 1024 * 1024), '1.0 GB');
    });

    test('caps the unit at GB for very large sizes', () {
      expect(formatSize(5 * 1024 * 1024 * 1024), '5.0 GB');
    });

    test('treats null as zero', () {
      expect(formatSize(null), '0 B');
    });
  });

  group('daysUntilDue / isOverdue', () {
    test('returns null when there is no due date', () {
      expect(daysUntilDue(buildReceipt(dueAt: 0)), isNull);
      expect(isOverdue(buildReceipt(dueAt: 0)), isFalse);
    });

    test('counts whole calendar days for future due dates', () {
      expect(daysUntilDue(buildReceipt(dueAt: dueInDays(2))), 2);
    });

    test('is zero for today and negative when overdue', () {
      expect(daysUntilDue(buildReceipt(dueAt: dueInDays(0))), 0);
      expect(daysUntilDue(buildReceipt(dueAt: dueInDays(-3))), -3);
    });

    test('isOverdue is true only for past due dates', () {
      expect(isOverdue(buildReceipt(dueAt: dueInDays(-1))), isTrue);
      expect(isOverdue(buildReceipt(dueAt: dueInDays(0))), isFalse);
      expect(isOverdue(buildReceipt(dueAt: dueInDays(1))), isFalse);
    });
  });

  group('paid state', () {
    test('isProviderPaid matches the exact "plaćeno" status', () {
      expect(isProviderPaid(buildReceipt(status: 'plaćeno')), isTrue);
      expect(isProviderPaid(buildReceipt(status: '  plaćeno  ')), isTrue);
      expect(isProviderPaid(buildReceipt(status: 'nije plaćeno')), isFalse);
      expect(isProviderPaid(buildReceipt(status: '')), isFalse);
    });

    test('isPaid combines user flag and provider status', () {
      expect(isPaid(buildReceipt(paid: false, status: '')), isFalse);
      expect(isPaid(buildReceipt(paid: true, status: '')), isTrue);
      expect(isPaid(buildReceipt(paid: false, status: 'plaćeno')), isTrue);
    });
  });

  group('dueLabel', () {
    test('is empty without a due date', () {
      expect(dueLabel(buildReceipt(dueAt: 0)), '');
    });

    test('uses Serbian labels for the near window', () {
      expect(dueLabel(buildReceipt(dueAt: dueInDays(-1))), 'Dospeo');
      expect(dueLabel(buildReceipt(dueAt: dueInDays(0))), 'Danas');
      expect(dueLabel(buildReceipt(dueAt: dueInDays(1))), 'Sutra');
      expect(dueLabel(buildReceipt(dueAt: dueInDays(2))), 'Za 2 dana');
      expect(dueLabel(buildReceipt(dueAt: dueInDays(3))), 'Za 3 dana');
    });

    test('hides the badge beyond three days out', () {
      expect(dueLabel(buildReceipt(dueAt: dueInDays(4))), '');
      expect(dueLabel(buildReceipt(dueAt: dueInDays(10))), '');
    });
  });

  group('dueLine', () {
    test('is empty without a due date', () {
      expect(dueLine(buildReceipt(dueAt: 0)), '');
    });

    test('prefixes with Dospeo when overdue and Dospeće otherwise', () {
      final overdue = buildReceipt(dueAt: dueInDays(-2));
      final upcoming = buildReceipt(dueAt: dueInDays(5));
      expect(dueLine(overdue), startsWith('Dospeo '));
      expect(dueLine(upcoming), startsWith('Dospeće '));
    });
  });

  group('relativeTimeSr', () {
    int ago(int seconds) =>
        DateTime.now().millisecondsSinceEpoch ~/ 1000 - seconds;

    test('returns Nikad for unset timestamps', () {
      expect(relativeTimeSr(0), 'Nikad');
      expect(relativeTimeSr(-5), 'Nikad');
    });

    test('returns upravo sada for the last few seconds', () {
      expect(relativeTimeSr(ago(1)), 'upravo sada');
    });

    test('pluralizes seconds, minutes, hours, and days in Serbian', () {
      expect(relativeTimeSr(ago(41 * 60)), 'pre 41 minuta');
      expect(relativeTimeSr(ago(1 * 60)), 'pre 1 minut');
      expect(relativeTimeSr(ago(2 * 3600)), 'pre 2 sata');
      expect(relativeTimeSr(ago(1 * 3600)), 'pre 1 sat');
      expect(relativeTimeSr(ago(5 * 86400)), 'pre 5 dana');
      expect(relativeTimeSr(ago(1 * 86400)), 'pre 1 dan');
    });

    test('rolls into months and years', () {
      expect(relativeTimeSr(ago(60 * 86400)), 'pre 2 meseca');
      expect(relativeTimeSr(ago(400 * 86400)), 'pre 1 godinu');
    });
  });
}
