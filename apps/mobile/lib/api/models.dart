// Dart mirrors of the Go JSON API types. Field names track the `json:` tags in
// `apps/server/pkg/http/*.go`.

int _asInt(dynamic v) => v is int ? v : (v is num ? v.toInt() : 0);
double _asDouble(dynamic v) => v is num ? v.toDouble() : 0.0;
bool _asBool(dynamic v) => v == true;
String _asStr(dynamic v) => v == null ? '' : v.toString();

/// One receipt row (`APIReceipt` in receipts.go).
class Receipt {
  Receipt({
    required this.provider,
    required this.account,
    required this.label,
    required this.period,
    required this.url,
    required this.filename,
    required this.currency,
    required this.status,
    required this.size,
    required this.modified,
    required this.downloadedAt,
    required this.amount,
    required this.paidAt,
    required this.confirmedAt,
    required this.dueAt,
    required this.paid,
    required this.confirmed,
  });

  final String provider;

  /// Provider account id ("" for a solo deployment).
  final String account;

  /// Display name for [account] ("Mama"); empty for solo / unlabeled.
  final String label;
  final String period;
  final String url;
  final String filename;
  final String currency;
  final String status;
  final int size;
  final int modified;
  final int downloadedAt;
  final double amount;
  final int paidAt;
  final int confirmedAt;
  final int dueAt;
  final bool paid;
  final bool confirmed;

  factory Receipt.fromJson(Map<String, dynamic> j) => Receipt(
    provider: _asStr(j['provider']),
    account: _asStr(j['account']),
    label: _asStr(j['label']),
    period: _asStr(j['period']),
    url: _asStr(j['url']),
    filename: _asStr(j['filename']),
    currency: _asStr(j['currency']),
    status: _asStr(j['status']),
    size: _asInt(j['size']),
    modified: _asInt(j['modified']),
    downloadedAt: _asInt(j['downloaded_at']),
    amount: _asDouble(j['amount']),
    paidAt: _asInt(j['paid_at']),
    confirmedAt: _asInt(j['confirmed_at']),
    dueAt: _asInt(j['due_at']),
    paid: _asBool(j['paid']),
    confirmed: _asBool(j['confirmed']),
  );

  /// Unique key used for list identity (period + provider + account).
  String get key =>
      account.isEmpty ? '$period/$provider' : '$period/$provider/$account';
}

/// Aggregate stats for a year (`StatsResponse` in stats.go).
class Stats {
  Stats({
    required this.currency,
    required this.monthly,
    required this.monthlyCounts,
    required this.byProvider,
    required this.monthlyByProvider,
    required this.year,
    required this.total,
    required this.average,
  });

  final String currency;
  final List<double> monthly;
  final List<int> monthlyCounts;
  final List<ProviderTotal> byProvider;
  final List<ProviderMonthly> monthlyByProvider;
  final int year;
  final double total;
  final double average;

  factory Stats.fromJson(Map<String, dynamic> j) => Stats(
    currency: _asStr(j['currency']),
    monthly: ((j['monthly'] as List?) ?? const [])
        .map(_asDouble)
        .toList(growable: false),
    monthlyCounts: ((j['monthly_counts'] as List?) ?? const [])
        .map(_asInt)
        .toList(growable: false),
    byProvider: ((j['by_provider'] as List?) ?? const [])
        .map((e) => ProviderTotal.fromJson(e as Map<String, dynamic>))
        .toList(),
    monthlyByProvider: ((j['monthly_by_provider'] as List?) ?? const [])
        .map((e) => ProviderMonthly.fromJson(e as Map<String, dynamic>))
        .toList(),
    year: _asInt(j['year']),
    total: _asDouble(j['total']),
    average: _asDouble(j['average']),
  );

  static Stats empty(int year) => Stats(
    currency: 'RSD',
    monthly: List.filled(12, 0),
    monthlyCounts: List.filled(12, 0),
    byProvider: const [],
    monthlyByProvider: const [],
    year: year,
    total: 0,
    average: 0,
  );
}

class ProviderTotal {
  ProviderTotal(this.provider, this.amount);
  final String provider;
  final double amount;
  factory ProviderTotal.fromJson(Map<String, dynamic> j) =>
      ProviderTotal(_asStr(j['provider']), _asDouble(j['amount']));
}

class ProviderMonthly {
  ProviderMonthly(this.provider, this.monthly);
  final String provider;
  final List<double> monthly;
  factory ProviderMonthly.fromJson(Map<String, dynamic> j) => ProviderMonthly(
    _asStr(j['provider']),
    ((j['monthly'] as List?) ?? const [])
        .map(_asDouble)
        .toList(growable: false),
  );
}

/// Refresh run state (`refresh.State().WithWindow(...)`).
class RefreshState {
  RefreshState({
    required this.results,
    required this.startedAt,
    required this.finishedAt,
    required this.checkUntil,
    required this.running,
    required this.allowed,
  });

  final List<RefreshResult> results;
  final int startedAt;
  final int finishedAt;
  final int checkUntil;
  final bool running;
  final bool allowed;

  factory RefreshState.fromJson(Map<String, dynamic> j) => RefreshState(
    results: ((j['results'] as List?) ?? const [])
        .map((e) => RefreshResult.fromJson(e as Map<String, dynamic>))
        .toList(),
    startedAt: _asInt(j['started_at']),
    finishedAt: _asInt(j['finished_at']),
    checkUntil: _asInt(j['check_until']),
    running: _asBool(j['running']),
    allowed: _asBool(j['allowed']),
  );
}

class RefreshResult {
  RefreshResult({
    required this.provider,
    required this.account,
    required this.label,
    required this.durationMs,
    required this.ok,
    required this.error,
    required this.empty,
  });
  final String provider;
  final String account;
  final String label;
  final int durationMs;
  final bool ok;
  final String error;
  final bool empty;
  factory RefreshResult.fromJson(Map<String, dynamic> j) => RefreshResult(
    provider: _asStr(j['provider']),
    account: _asStr(j['account']),
    label: _asStr(j['label']),
    durationMs: _asInt(j['duration_ms']),
    ok: _asBool(j['ok']),
    error: _asStr(j['error']),
    empty: _asBool(j['empty']),
  );
}

/// Provider row for the settings screen (`APIProviderStatus`).
class ProviderStatus {
  ProviderStatus({
    required this.name,
    required this.account,
    required this.label,
    required this.identifier,
    required this.hasPassword,
    required this.configured,
    required this.implemented,
  });

  final String name;

  /// Account id ("" for solo); matches config Account.ID.
  final String account;
  final String label;
  final String identifier;
  final bool hasPassword;
  final bool configured;
  final bool implemented;

  factory ProviderStatus.fromJson(Map<String, dynamic> j) => ProviderStatus(
    name: _asStr(j['name']),
    account: _asStr(j['account']),
    label: _asStr(j['label']),
    identifier: _asStr(j['identifier']),
    hasPassword: _asBool(j['has_password']),
    configured: _asBool(j['configured']),
    implemented: _asBool(j['implemented']),
  );

  /// Secret-hint key matching the web settings UI / server providerStatusKey.
  String get secretKey => account.isEmpty ? name : '$name/$account';
}

/// The full settings payload (`APISettings`). [form] is kept as a mutable JSON
/// map so edits round-trip back to `PUT /api/settings` verbatim (preserving any
/// field the UI does not render), exactly like the web settings page.
class Settings {
  Settings({
    required this.form,
    required this.version,
    required this.storage,
    required this.storageTarget,
    required this.downloadPath,
    required this.providers,
    required this.ignoredKeys,
    required this.receiptCount,
    required this.downloadPathExists,
    required this.downloadPathWritable,
    required this.notifyCanTest,
    required this.hasS3Access,
    required this.hasS3Secret,
    required this.hasSmtpPass,
    required this.hasTgToken,
  });

  final Map<String, dynamic> form;
  final String version;
  final String storage;
  final String storageTarget;
  final String downloadPath;
  final List<ProviderStatus> providers;
  final List<String> ignoredKeys;
  final int receiptCount;
  final bool downloadPathExists;
  final bool downloadPathWritable;
  final bool notifyCanTest;
  final bool hasS3Access;
  final bool hasS3Secret;
  final bool hasSmtpPass;
  final bool hasTgToken;

  factory Settings.fromJson(Map<String, dynamic> j) => Settings(
    form: Map<String, dynamic>.from(j['form'] as Map? ?? const {}),
    version: _asStr(j['version']),
    storage: _asStr(j['storage']),
    storageTarget: _asStr(j['storage_target']),
    downloadPath: _asStr(j['download_path']),
    providers: ((j['providers'] as List?) ?? const [])
        .map((e) => ProviderStatus.fromJson(e as Map<String, dynamic>))
        .toList(),
    ignoredKeys: ((j['ignored_keys'] as List?) ?? const [])
        .map(_asStr)
        .toList(),
    receiptCount: _asInt(j['receipt_count']),
    downloadPathExists: _asBool(j['download_path_exists']),
    downloadPathWritable: _asBool(j['download_path_writable']),
    notifyCanTest: _asBool(j['notify_can_test']),
    hasS3Access: _asBool(j['has_s3_access']),
    hasS3Secret: _asBool(j['has_s3_secret']),
    hasSmtpPass: _asBool(j['has_smtp_pass']),
    hasTgToken: _asBool(j['has_tg_token']),
  );
}
