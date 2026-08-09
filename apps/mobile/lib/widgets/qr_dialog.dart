import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../theme/app_colors.dart';
import '../util/i18n.dart';
import '../util/format.dart' as fmt;
import 'ui.dart';

/// Decoded NBS IPS QR fields (a subset — what the modal shows).
class _Ips {
  String recipient = '';
  String account = '';
  String reference = '';
  String code = '';
  String purpose = '';
  double? amount;

  bool get hasDetails =>
      recipient.isNotEmpty || account.isNotEmpty || reference.isNotEmpty;

  /// Parses the pipe-delimited `KEY:VALUE` IPS payload.
  static _Ips parse(String payload) {
    final ips = _Ips();
    for (final part in payload.split('|')) {
      final i = part.indexOf(':');
      if (i < 0) continue;
      final key = part.substring(0, i).toUpperCase();
      final value = part.substring(i + 1).trim();
      switch (key) {
        case 'N':
          ips.recipient = value.replaceAll('\n', ', ');
        case 'R':
          ips.account = value;
        case 'RO':
          ips.reference = value;
        case 'SF':
          ips.code = value;
        case 'S':
          ips.purpose = value;
        case 'I':
          // e.g. "RSD1234,56" → strip the 3-letter currency, comma decimal.
          var v = value;
          if (v.length > 3 && RegExp(r'^[A-Za-z]{3}').hasMatch(v)) {
            v = v.substring(3);
          }
          ips.amount = double.tryParse(v.replaceAll(',', '.'));
      }
    }
    return ips;
  }
}

/// The "Plati skeniranjem" modal, mirroring the QR modal in `index.html`:
/// receipt summary, decoded IPS fields, the QR image, and a mark-as-paid
/// action. Returns `true` (via Navigator.pop) when the receipt was marked paid.
class QrDialog extends StatefulWidget {
  const QrDialog({
    super.key,
    required this.api,
    required this.receipt,
    required this.onMarkPaid,
  });

  final ApiClient api;
  final Receipt receipt;
  final Future<void> Function() onMarkPaid;

  static Future<bool?> show(
    BuildContext context, {
    required ApiClient api,
    required Receipt receipt,
    required Future<void> Function() onMarkPaid,
  }) {
    return showDialog<bool>(
      context: context,
      barrierColor: Colors.black.withValues(alpha: 0.7),
      builder: (_) =>
          QrDialog(api: api, receipt: receipt, onMarkPaid: onMarkPaid),
    );
  }

  @override
  State<QrDialog> createState() => _QrDialogState();
}

class _QrDialogState extends State<QrDialog> {
  _Ips? _ips;
  bool _missing = false;
  bool _saving = false;
  bool _copied = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final payload = await widget.api.qrPayload(widget.receipt);
      if (!mounted) return;
      if (payload.isEmpty) {
        setState(() => _missing = true);
      } else {
        setState(() => _ips = _Ips.parse(payload));
      }
    } catch (_) {
      if (mounted) setState(() => _missing = true);
    }
  }

  Future<void> _markPaid() async {
    setState(() => _saving = true);
    try {
      await widget.onMarkPaid();
      if (mounted) Navigator.of(context).pop(true);
    } catch (_) {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    final r = widget.receipt;
    final displayAmount = _ips?.amount ?? (r.amount != 0 ? r.amount : null);

    return Dialog(
      backgroundColor: c.card,
      insetPadding: const EdgeInsets.all(16),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppColors.radiusXl),
        side: BorderSide(color: c.border),
      ),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 480, maxHeight: 720),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Header.
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 12, 16),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Plati skeniranjem'.t,
                          style: TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w600,
                            color: c.foreground,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          '${fmt.receiptLabel(r)} · ${r.period}'.t,
                          style: TextStyle(
                            fontSize: 14,
                            color: c.mutedForeground,
                          ),
                        ),
                      ],
                    ),
                  ),
                  IconButton(
                    icon: Icon(Icons.close_rounded, color: c.mutedForeground),
                    onPressed: () => Navigator.of(context).pop(false),
                  ),
                ],
              ),
            ),
            Divider(height: 1, color: c.border),
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: Column(
                  children: [
                    const SizedBox(height: 16),
                    _row(
                      c,
                      'Iznos'.t,
                      displayAmount != null
                          ? fmt.formatMoney(displayAmount, r.currency)
                          : '—',
                      emphasize: true,
                    ),
                    _row(c, 'Period'.t, r.period),
                    _row(
                      c,
                      'Fajl'.t,
                      r.filename +
                          (r.size > 0 ? ' · ${fmt.formatSize(r.size)}' : ''),
                    ),
                    _row(
                      c,
                      'Preuzeto'.t,
                      fmt.formatDate(
                        r.downloadedAt > 0 ? r.downloadedAt : r.modified,
                      ),
                    ),
                    if (_ips != null && _ips!.hasDetails) ...[
                      const SizedBox(height: 12),
                      Divider(height: 1, color: c.border),
                      const SizedBox(height: 12),
                      if (_ips!.recipient.isNotEmpty)
                        _row(c, 'Primalac'.t, _ips!.recipient),
                      if (_ips!.account.isNotEmpty)
                        _row(c, 'Račun'.t, _ips!.account, mono: true),
                      if (_ips!.reference.isNotEmpty)
                        _row(c, 'Poziv na broj'.t, _ips!.reference, mono: true),
                      if (_ips!.code.isNotEmpty)
                        _row(c, 'Šifra plaćanja'.t, _ips!.code, mono: true),
                      if (_ips!.purpose.isNotEmpty)
                        _row(c, 'Svrha'.t, _ips!.purpose),
                    ],
                    const SizedBox(height: 16),
                    Divider(height: 1, color: c.border),
                    const SizedBox(height: 16),
                    _qrArea(c),
                    const SizedBox(height: 16),
                  ],
                ),
              ),
            ),
            Divider(height: 1, color: c.border),
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Expanded(
                    child: AppButton(
                      label: 'Zatvori'.t,
                      variant: ButtonVariant.outline,
                      fullWidth: true,
                      onPressed: () => Navigator.of(context).pop(false),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: AppButton(
                      label: 'Označi kao plaćeno'.t,
                      variant: ButtonVariant.primary,
                      fullWidth: true,
                      loading: _saving,
                      onPressed: _markPaid,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _qrArea(AppColors c) {
    if (_missing) {
      return Text(
        'Ovaj račun nema čitljiv QR kôd za plaćanje.'.t,
        textAlign: TextAlign.center,
        style: TextStyle(fontSize: 14, color: c.mutedForeground),
      );
    }
    return Column(
      children: [
        Container(
          width: 240,
          height: 240,
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(AppColors.radiusMd),
          ),
          child: Image.network(
            widget.api.qrImageUrl(widget.receipt),
            fit: BoxFit.contain,
            errorBuilder: (context, error, stack) {
              WidgetsBinding.instance.addPostFrameCallback((_) {
                if (mounted) setState(() => _missing = true);
              });
              return const SizedBox.shrink();
            },
            loadingBuilder: (context, child, progress) => progress == null
                ? child
                : const Center(child: CircularProgressIndicator()),
          ),
        ),
        const SizedBox(height: 12),
        Text(
          'Skenirajte kôd u mobilnoj aplikaciji banke da platite ovaj račun.'.t,
          textAlign: TextAlign.center,
          style: TextStyle(fontSize: 14, color: c.mutedForeground),
        ),
        const SizedBox(height: 8),
        AppButton(
          label: _copied ? 'Kopirano ✓' : 'Kopiraj IPS podatke',
          variant: ButtonVariant.ghost,
          onPressed: _ips == null
              ? null
              : () async {
                  await Clipboard.setData(
                    ClipboardData(text: _ipsClipboardText()),
                  );
                  if (mounted) setState(() => _copied = true);
                },
        ),
      ],
    );
  }

  String _ipsClipboardText() {
    final i = _ips;
    if (i == null) return '';
    return [
      if (i.recipient.isNotEmpty) 'Primalac: ${i.recipient}',
      if (i.account.isNotEmpty) 'Račun: ${i.account}',
      if (i.reference.isNotEmpty) 'Poziv na broj: ${i.reference}',
      if (i.code.isNotEmpty) 'Šifra plaćanja: ${i.code}',
      if (i.purpose.isNotEmpty) 'Svrha: ${i.purpose}',
      if (i.amount != null) 'Iznos: ${i.amount}',
    ].join('\n');
  }

  Widget _row(
    AppColors c,
    String label,
    String value, {
    bool emphasize = false,
    bool mono = false,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: TextStyle(fontSize: 14, color: c.mutedForeground)),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              value,
              textAlign: TextAlign.right,
              style: TextStyle(
                fontSize: emphasize ? 18 : 14,
                fontWeight: emphasize ? FontWeight.w600 : FontWeight.w500,
                fontFamily: mono ? 'monospace' : null,
                color: c.foreground,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
