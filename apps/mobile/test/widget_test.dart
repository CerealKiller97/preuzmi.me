import 'package:flutter_test/flutter_test.dart';
import 'package:preuzmi_mobile/util/format.dart';

void main() {
  test('formatSize renders human-readable byte sizes', () {
    expect(formatSize(512), '512 B');
    expect(formatSize(1536), '1.5 KB');
  });

  test('formatDate renders empty for unset timestamps', () {
    expect(formatDate(0), '');
    expect(formatDate(null), '');
  });

  test('providerLabel uppercases, applies overrides, and appends labels', () {
    expect(providerLabel('mts'), 'MTS');
    expect(providerLabel('esanduce'), 'E-SANDUČE');
    expect(providerLabel('a1', 'Mama'), 'A1 — Mama');
    expect(providerLabel('eps', ''), 'EPS');
  });
}
