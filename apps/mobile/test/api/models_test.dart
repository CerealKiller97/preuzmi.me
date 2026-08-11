import 'package:flutter_test/flutter_test.dart';
import 'package:preuzmi_mobile/api/models.dart';

void main() {
  group('Receipt.fromJson', () {
    test('parses a full payload', () {
      final r = Receipt.fromJson({
        'provider': 'a1',
        'account': 'mama',
        'label': 'Mama',
        'period': '06-2026',
        'url': 'https://x/r.pdf',
        'filename': 'r.pdf',
        'currency': 'RSD',
        'status': 'plaćeno',
        'size': 2048,
        'modified': 1000,
        'downloaded_at': 2000,
        'amount': 1234.5,
        'paid_at': 3000,
        'confirmed_at': 4000,
        'due_at': 5000,
        'paid': true,
        'confirmed': true,
      });
      expect(r.provider, 'a1');
      expect(r.account, 'mama');
      expect(r.label, 'Mama');
      expect(r.size, 2048);
      expect(r.amount, 1234.5);
      expect(r.paid, isTrue);
      expect(r.confirmed, isTrue);
    });

    test('applies safe defaults for missing and null fields', () {
      final r = Receipt.fromJson({});
      expect(r.provider, '');
      expect(r.account, '');
      expect(r.size, 0);
      expect(r.amount, 0.0);
      expect(r.paid, isFalse);
    });

    test('coerces numeric types (int amount, double size)', () {
      final r = Receipt.fromJson({'amount': 10, 'size': 3.0});
      expect(r.amount, 10.0);
      expect(r.size, 3);
    });

    test('paid is false for any non-true value', () {
      expect(Receipt.fromJson({'paid': 'true'}).paid, isFalse);
      expect(Receipt.fromJson({'paid': 1}).paid, isFalse);
      expect(Receipt.fromJson({'paid': true}).paid, isTrue);
    });
  });

  group('Receipt.key', () {
    test('omits account for solo deployments', () {
      final r = Receipt.fromJson({'period': '06-2026', 'provider': 'mts'});
      expect(r.key, '06-2026/mts');
    });

    test('includes account when present', () {
      final r = Receipt.fromJson({
        'period': '06-2026',
        'provider': 'a1',
        'account': 'mama',
      });
      expect(r.key, '06-2026/a1/mama');
    });
  });

  group('ProviderStatus.secretKey', () {
    test('is the name alone for solo providers', () {
      final p = ProviderStatus.fromJson({'name': 'mts'});
      expect(p.secretKey, 'mts');
    });

    test('joins name and account for multi-account providers', () {
      final p = ProviderStatus.fromJson({'name': 'a1', 'account': 'mama'});
      expect(p.secretKey, 'a1/mama');
    });
  });

  group('Stats.fromJson', () {
    test('parses monthly arrays and nested provider rows', () {
      final s = Stats.fromJson({
        'currency': 'RSD',
        'monthly': [1, 2.5, 3],
        'monthly_counts': [1, 0, 2],
        'by_provider': [
          {'provider': 'mts', 'amount': 100},
        ],
        'monthly_by_provider': [
          {
            'provider': 'mts',
            'monthly': [1, 2, 3],
          },
        ],
        'year': 2026,
        'total': 6.5,
        'average': 2.1,
      });
      expect(s.monthly, [1.0, 2.5, 3.0]);
      expect(s.monthlyCounts, [1, 0, 2]);
      expect(s.byProvider.single.provider, 'mts');
      expect(s.byProvider.single.amount, 100.0);
      expect(s.monthlyByProvider.single.monthly, [1.0, 2.0, 3.0]);
      expect(s.year, 2026);
    });

    test('empty() yields a zeroed 12-month year', () {
      final s = Stats.empty(2026);
      expect(s.year, 2026);
      expect(s.currency, 'RSD');
      expect(s.monthly, List.filled(12, 0.0));
      expect(s.monthlyCounts, List.filled(12, 0));
      expect(s.total, 0.0);
    });

    test('tolerates missing arrays', () {
      final s = Stats.fromJson({'year': 2026});
      expect(s.monthly, isEmpty);
      expect(s.byProvider, isEmpty);
    });
  });

  group('Settings.fromJson', () {
    test('keeps the raw form map for round-tripping', () {
      final s = Settings.fromJson({
        'form': {'download_path': '/data', 'extra': 42},
        'version': '1.2.3',
        'providers': [
          {'name': 'mts', 'configured': true},
        ],
        'ignored_keys': ['06-2026/eps'],
        'receipt_count': 7,
        'has_tg_token': true,
      });
      expect(s.form['download_path'], '/data');
      expect(s.form['extra'], 42);
      expect(s.version, '1.2.3');
      expect(s.providers.single.name, 'mts');
      expect(s.ignoredKeys, ['06-2026/eps']);
      expect(s.receiptCount, 7);
      expect(s.hasTgToken, isTrue);
    });

    test('defaults form to an empty mutable map when absent', () {
      final s = Settings.fromJson({});
      expect(s.form, isEmpty);
      s.form['k'] = 'v'; // must not throw — map is mutable.
      expect(s.form['k'], 'v');
    });
  });

  group('RefreshState.fromJson', () {
    test('parses results and flags', () {
      final st = RefreshState.fromJson({
        'results': [
          {'provider': 'mts', 'ok': true, 'duration_ms': 120},
          {'provider': 'eps', 'ok': false, 'error': 'boom'},
        ],
        'started_at': 100,
        'finished_at': 200,
        'running': false,
        'allowed': true,
      });
      expect(st.results, hasLength(2));
      expect(st.results.first.ok, isTrue);
      expect(st.results.last.error, 'boom');
      expect(st.running, isFalse);
      expect(st.allowed, isTrue);
    });
  });
}
