import 'package:intl/intl.dart';

import '../api/models.dart';

/// Formatting + display helpers ported 1:1 from `apps/server/assets/js/app.js`
/// and `script.js`, so the mobile app labels, money, dates and due-badges
/// exactly like the web UI.

/// Latin display-name overrides for provider keys whose uppercased form reads
/// wrong (mirrors `providerNames` in script.js).
const Map<String, String> _providerNames = {
  'esanduce': 'E-SANDUČE',
  'eupravnik': 'E-UPRAVNIK',
};

/// Brand display name for a provider key (Latin). E.g. `mts` → `MTS`,
/// `esanduce` → `E-SANDUČE`.
String providerBrand(String key) {
  final k = key.toLowerCase();
  return _providerNames[k] ?? key.toUpperCase();
}

/// Display name for a provider account. When [label] is set it is appended
/// ("A1 — Mama"); otherwise just the brand. Mirrors `providerLabel` in script.js.
String providerLabel(String key, [String? label]) {
  final brand = providerBrand(key);
  final l = (label ?? '').trim();
  return l.isEmpty ? brand : '$brand — $l';
}

/// Card/header title for a receipt that already carries provider + optional label.
String receiptLabel(Receipt r) => providerLabel(r.provider, r.label);

final NumberFormat _money = NumberFormat.currency(
  locale: 'sr',
  symbol: 'RSD',
  decimalDigits: 0,
);

/// Money like the web's `formatMoney`: Serbian grouping, no decimals, currency
/// code. Falls back to `RSD` when the receipt carries no currency.
String formatMoney(num? amount, String? currency) {
  final value = (amount ?? 0).toDouble();
  final sym = (currency == null || currency.isEmpty) ? 'RSD' : currency;
  if (sym == 'RSD') {
    return _money.format(value);
  }
  final fmt = NumberFormat.currency(
    locale: 'sr',
    symbol: sym,
    decimalDigits: 0,
  );
  return fmt.format(value);
}

/// Unix-seconds → `DD.MM.YYYY`, empty when unset. Matches `formatDate`.
String formatDate(int? ts) {
  final num = ts ?? 0;
  if (num <= 0) return '';
  final d = DateTime.fromMillisecondsSinceEpoch(num * 1000);
  final dd = d.day.toString().padLeft(2, '0');
  final mm = d.month.toString().padLeft(2, '0');
  return '$dd.$mm.${d.year}';
}

/// Human byte size like `formatSize`: `B`, then `KB`/`MB`/`GB` with one decimal.
String formatSize(int? bytes) {
  final num = (bytes ?? 0).toDouble();
  if (num < 1024) return '${num.toInt()} B';
  const units = ['KB', 'MB', 'GB'];
  var value = num / 1024;
  var unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return '${value.toStringAsFixed(1)} ${units[unit]}';
}

/// Whole calendar days from today to the due date; negative when overdue,
/// null when unknown. Matches `daysUntilDue`.
int? daysUntilDue(Receipt r) {
  if (r.dueAt <= 0) return null;
  final due = DateTime.fromMillisecondsSinceEpoch(r.dueAt * 1000);
  final dueDay = DateTime(due.year, due.month, due.day);
  final now = DateTime.now();
  final today = DateTime(now.year, now.month, now.day);
  return dueDay.difference(today).inDays;
}

bool isOverdue(Receipt r) {
  final d = daysUntilDue(r);

  return d != null && d < 0;
}

/// Provider reports the receipt as paid (`status == "plaćeno"`).
bool isProviderPaid(Receipt r) => r.status.trim() == 'plaćeno';

/// Effective paid state: provider-confirmed, or the user marked it paid.
bool isPaid(Receipt r) => r.paid || isProviderPaid(r);

/// Short badge label for unpaid receipts with a known deadline. `''` hides it.
String dueLabel(Receipt r) {
  final days = daysUntilDue(r);
  if (days == null) {
    return '';
  }

  if (days < 0) {
    return 'Dospeo';
  }

  if (days == 0) {
    return 'Danas';
  }

  if (days == 1) {
    return 'Sutra';
  }

  if (days <= 3) {
    return 'Za $days dana';
  }

  return '';
}

/// Long due line under a card ("Dospeo 12.06.2026" / "Dospeće 12.06.2026").
String dueLine(Receipt r) {
  final due = formatDate(r.dueAt);
  if (due.isEmpty) return '';
  return isOverdue(r) ? 'Dospeo $due' : 'Dospeće $due';
}

/// Serbian pluralization: `one` for 1, `few` for 2–4 (excluding 12–14), else
/// `other`.
String _srPlural(int n, String one, String few, String other) {
  if (n == 1) return one;
  final mod10 = n % 10;
  final mod100 = n % 100;
  if (mod10 >= 2 && mod10 <= 4 && !(mod100 >= 12 && mod100 <= 14)) return few;
  return other;
}

/// Serbian-Latin "time ago" like the web header ("pre 41 minuta"), or "Nikad"
/// when never fetched. Mirrors `relative()` in `assets/js/refresh.js`.
String relativeTimeSr(int unixSeconds) {
  if (unixSeconds <= 0) return 'Nikad';
  final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
  var diff = now - unixSeconds;
  if (diff < 5) return 'upravo sada';
  if (diff < 60) {
    return 'pre $diff ${_srPlural(diff, 'sekundu', 'sekunde', 'sekundi')}';
  }
  final mins = (diff / 60).round();
  if (mins < 60) {
    return 'pre $mins ${_srPlural(mins, 'minut', 'minuta', 'minuta')}';
  }
  final hours = (diff / 3600).round();
  if (hours < 24) {
    return 'pre $hours ${_srPlural(hours, 'sat', 'sata', 'sati')}';
  }
  final days = (diff / 86400).round();
  if (days < 30) {
    return 'pre $days ${_srPlural(days, 'dan', 'dana', 'dana')}';
  }
  final months = (days / 30).round();
  if (months < 12) {
    return 'pre $months ${_srPlural(months, 'mesec', 'meseca', 'meseci')}';
  }
  final years = (months / 12).round();
  return 'pre $years ${_srPlural(years, 'godinu', 'godine', 'godina')}';
}
