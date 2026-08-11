import 'package:preuzmi_mobile/api/models.dart';

/// Builds a [Receipt] with sensible defaults so tests only set the fields they
/// care about.
Receipt buildReceipt({
  String provider = 'mts',
  String account = '',
  String label = '',
  String period = '06-2026',
  String url = 'https://example.com/r.pdf',
  String filename = 'r.pdf',
  String currency = 'RSD',
  String status = '',
  int size = 0,
  int modified = 0,
  int downloadedAt = 0,
  double amount = 0,
  int paidAt = 0,
  int confirmedAt = 0,
  int dueAt = 0,
  bool paid = false,
  bool confirmed = false,
}) {
  return Receipt(
    provider: provider,
    account: account,
    label: label,
    period: period,
    url: url,
    filename: filename,
    currency: currency,
    status: status,
    size: size,
    modified: modified,
    downloadedAt: downloadedAt,
    amount: amount,
    paidAt: paidAt,
    confirmedAt: confirmedAt,
    dueAt: dueAt,
    paid: paid,
    confirmed: confirmed,
  );
}

/// Unix seconds for `daysFromToday` calendar days away from now, anchored at
/// noon so DST / hour-of-day never flips the day boundary.
int dueInDays(int daysFromToday) {
  final now = DateTime.now();
  final target = DateTime(now.year, now.month, now.day + daysFromToday, 12);
  return target.millisecondsSinceEpoch ~/ 1000;
}
