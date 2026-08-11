import 'dart:convert';

import 'package:dio/dio.dart';

import 'models.dart';

/// Result of a settings PUT (`settingsUpdateResponse`).
class SettingsUpdateResult {
  SettingsUpdateResult({
    required this.ok,
    required this.message,
    required this.error,
    required this.warnings,
  });
  final bool ok;
  final String message;
  final String error;
  final List<String> warnings;
}

/// Thin HTTP client over the preuzmi.me JSON API. All calls are relative to
/// [baseUrl]; construct a new client when the user changes the server URL.
class ApiClient {
  ApiClient(String baseUrl) : baseUrl = _normalize(baseUrl) {
    _dio = Dio(
      BaseOptions(
        baseUrl: this.baseUrl,
        connectTimeout: const Duration(seconds: 10),
        // Refresh talks to several external providers; keep receive generous.
        receiveTimeout: const Duration(seconds: 120),
        // Fetch as text and decode ourselves — some endpoints answer JSON with a
        // text/plain content-type (e.g. 202 from POST /api/refresh), which Dio's
        // JSON transformer would otherwise leave as a raw String.
        responseType: ResponseType.plain,
      ),
    );
  }

  final String baseUrl;
  late final Dio _dio;

  static String _normalize(String url) {
    var u = url.trim();
    if (u.isEmpty) return u;
    if (!u.startsWith('http://') && !u.startsWith('https://')) {
      u = 'http://$u';
    }
    while (u.endsWith('/')) {
      u = u.substring(0, u.length - 1);
    }
    return u;
  }

  // --- lenient decoders -----------------------------------------------------

  static dynamic _decode(dynamic data) {
    if (data is String) {
      final s = data.trim();
      if (s.isEmpty) return null;
      try {
        return jsonDecode(s);
      } catch (_) {
        return data; // not JSON — a plain-text error body
      }
    }
    return data;
  }

  static Map<String, dynamic> _asMap(dynamic data) {
    final d = _decode(data);
    return d is Map ? Map<String, dynamic>.from(d) : <String, dynamic>{};
  }

  static List<dynamic> _asList(dynamic data) {
    final d = _decode(data);
    return d is List ? d : const [];
  }

  /// A human-readable message from an error body, JSON `{error|message}` or
  /// plain text.
  static String _errorText(dynamic data) {
    final d = _decode(data);
    if (d is Map) {
      return (d['error'] ?? d['message'] ?? '').toString();
    }
    if (d is String) return d.trim();
    return '';
  }

  Future<List<String>> providers() async {
    final res = await _dio.get<dynamic>('/api/providers');
    return _asList(res.data).map((e) => e.toString()).toList();
  }

  Future<List<Receipt>> receipts({String? provider, String? period}) async {
    final res = await _dio.get<dynamic>(
      '/api/receipts',
      queryParameters: {
        if (provider != null && provider.isNotEmpty) 'provider': provider,
        if (period != null && period.isNotEmpty) 'period': period,
      },
    );
    return _asList(res.data)
        .map((e) => Receipt.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<void> markPaid(
    String period,
    String provider,
    bool paid, {
    String account = '',
  }) async {
    await _dio.put<dynamic>(
      '/api/receipts/$period/$provider/paid',
      queryParameters: {if (account.isNotEmpty) 'account': account},
      data: {'paid': paid},
    );
  }

  Future<Stats> stats({int? year, String? provider}) async {
    final qp = <String, dynamic>{};
    if (year != null) qp['year'] = year;
    if (provider != null && provider.isNotEmpty) qp['provider'] = provider;
    final res = await _dio.get<dynamic>('/api/stats', queryParameters: qp);
    return Stats.fromJson(_asMap(res.data));
  }

  Future<RefreshState> refreshStatus() async {
    final res = await _dio.get<dynamic>('/api/refresh');
    return RefreshState.fromJson(_asMap(res.data));
  }

  /// Starts a refresh. 202 (started), 200 (already up to date), 409 (running)
  /// and 403 (outside the download window) all carry a valid [RefreshState].
  Future<RefreshState> startRefresh() async {
    final res = await _dio.post<dynamic>(
      '/api/refresh',
      options: Options(validateStatus: (s) => s != null && s < 500),
    );
    return RefreshState.fromJson(_asMap(res.data));
  }

  Future<Settings> settings() async {
    final res = await _dio.get<dynamic>('/api/settings');
    return Settings.fromJson(_asMap(res.data));
  }

  Future<SettingsUpdateResult> updateSettings(Map<String, dynamic> form) async {
    final res = await _dio.put<dynamic>(
      '/api/settings',
      data: form,
      options: Options(validateStatus: (s) => s != null && s < 500),
    );
    final j = _asMap(res.data);
    return SettingsUpdateResult(
      ok: j['ok'] == true,
      message: (j['message'] ?? '').toString(),
      error: j['error'] != null ? j['error'].toString() : _errorText(res.data),
      warnings: ((j['warnings'] as List?) ?? const [])
          .map((e) => e.toString())
          .toList(),
    );
  }

  Future<SettingsUpdateResult> testNotification() async {
    final res = await _dio.post<dynamic>(
      '/api/notifications/test',
      options: Options(validateStatus: (s) => s != null && s < 500),
    );
    final j = _asMap(res.data);
    return SettingsUpdateResult(
      ok: j['ok'] == true,
      message: (j['message'] ?? '').toString(),
      error: j['error'] != null ? j['error'].toString() : _errorText(res.data),
      warnings: const [],
    );
  }

  /// Path-only receipt URL (`/receipt/{period}/{provider}`), without the
  /// `?account=` query — used to build nested QR routes correctly.
  String _receiptPath(Receipt r) => '/receipt/${r.period}/${r.provider}';

  Map<String, dynamic> _accountQuery(Receipt r) => {
    if (r.account.isNotEmpty) 'account': r.account,
  };

  // URL helpers for assets rendered by the browser/PDF viewer. Named accounts
  // carry `?account=<id>` on every receipt route (see accountParam on the
  // server); QR lives under `/qr.png` / `/qr.txt` on the path, with the same
  // query — never append path segments after the query string.
  String pdfUrl(Receipt r) {
    final q = _accountQuery(r);
    final path = _receiptPath(r);
    if (q.isEmpty) return '$baseUrl$path';
    return '$baseUrl$path?account=${Uri.encodeQueryComponent(r.account)}';
  }

  String qrImageUrl(Receipt r) {
    final path = '${_receiptPath(r)}/qr.png';
    if (r.account.isEmpty) return '$baseUrl$path';
    return '$baseUrl$path?account=${Uri.encodeQueryComponent(r.account)}';
  }

  String qrPayloadUrl(Receipt r) {
    final path = '${_receiptPath(r)}/qr.txt';
    if (r.account.isEmpty) return '$baseUrl$path';
    return '$baseUrl$path?account=${Uri.encodeQueryComponent(r.account)}';
  }

  Future<String> qrPayload(Receipt r) async {
    final path = '${_receiptPath(r)}/qr.txt';
    final res = await _dio.get<String>(
      path,
      queryParameters: _accountQuery(r),
      options: Options(responseType: ResponseType.plain),
    );
    return (res.data ?? '').trim();
  }
}
