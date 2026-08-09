import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import '../theme/app_colors.dart';
import '../util/i18n.dart';
import '../util/format.dart' as fmt;
import '../widgets/app_toast.dart';
import '../widgets/qr_dialog.dart';
import '../widgets/receipt_card.dart';
import '../widgets/ui.dart';

/// The Receipts (dashboard) screen — mirrors `templates/index.html`: search +
/// period + provider + paid filters, the count summary, and the receipt cards.
class ReceiptsScreen extends ConsumerStatefulWidget {
  const ReceiptsScreen({super.key});

  @override
  ConsumerState<ReceiptsScreen> createState() => _ReceiptsScreenState();
}

class _ReceiptsScreenState extends ConsumerState<ReceiptsScreen> {
  final _searchController = TextEditingController();
  String _query = '';
  String _selectedPeriod = '';
  final Set<String> _selectedProviders = {};
  String _paidFilter = 'all'; // all | paid | unpaid
  bool _refreshing = false;
  Timer? _pollTimer;
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    // Keep the "Poslednje preuzimanje" relative label ticking, like the web.
    _ticker = Timer.periodic(const Duration(seconds: 30), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _searchController.dispose();
    _pollTimer?.cancel();
    _ticker?.cancel();
    super.dispose();
  }

  bool get _hasActiveFilters =>
      _selectedProviders.isNotEmpty ||
      _selectedPeriod.isNotEmpty ||
      _paidFilter != 'all' ||
      _query.trim().isNotEmpty;

  void _resetFilters() {
    setState(() {
      _selectedProviders.clear();
      _selectedPeriod = '';
      _paidFilter = 'all';
      _query = '';
      _searchController.clear();
    });
  }

  List<String> _periodsOf(List<Receipt> receipts) {
    final set = <String>{for (final r in receipts) r.period};
    final list = set.toList();
    list.sort((a, b) {
      final pa = _periodKey(a);
      final pb = _periodKey(b);
      return pb.compareTo(pa); // newest first
    });
    return list;
  }

  int _periodKey(String period) {
    final parts = period.replaceAll('/', '-').split('-');
    if (parts.length != 2) return 0;
    final m = int.tryParse(parts[0]) ?? 0;
    final y = int.tryParse(parts[1]) ?? 0;
    return y * 100 + m;
  }

  List<Receipt> _filter(List<Receipt> receipts) {
    var items = receipts.toList();
    if (_selectedProviders.isNotEmpty) {
      items = items
          .where((r) => _selectedProviders.contains(r.provider.toLowerCase()))
          .toList();
    }
    if (_selectedPeriod.isNotEmpty) {
      items = items
          .where((r) => r.period.toLowerCase() == _selectedPeriod.toLowerCase())
          .toList();
    }
    if (_paidFilter != 'all') {
      final wantPaid = _paidFilter == 'paid';
      items = items.where((r) => fmt.isPaid(r) == wantPaid).toList();
    }
    final q = _query.trim().toLowerCase();
    if (q.isNotEmpty) {
      items = items
          .where(
            (r) =>
                r.provider.toLowerCase().contains(q) ||
                r.filename.toLowerCase().contains(q) ||
                r.period.toLowerCase().contains(q),
          )
          .toList();
    }
    items.sort((a, b) {
      final byMod = b.modified.compareTo(a.modified);
      if (byMod != 0) return byMod;
      return a.provider.compareTo(b.provider);
    });
    return items;
  }

  Future<void> _runRefresh() async {
    if (_refreshing) return;
    setState(() => _refreshing = true);
    final api = ref.read(apiClientProvider);
    try {
      final state = await api.startRefresh();
      if (!state.allowed) {
        _snack('Osvežavanje trenutno nije dozvoljeno.', error: true);
        setState(() => _refreshing = false);
        return;
      }
      if (state.running) {
        await _pollUntilDone(api);
      }
      ref.invalidate(receiptsProvider);
      ref.invalidate(statsProvider);
      ref.invalidate(refreshStatusProvider);
      if (mounted) _snack('Računi su osveženi.');
    } catch (e) {
      if (mounted) _snack('Osvežavanje nije uspelo: $e', error: true);
    } finally {
      if (mounted) setState(() => _refreshing = false);
    }
  }

  Future<void> _pollUntilDone(ApiClient api) async {
    final completer = Completer<void>();
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 2), (t) async {
      try {
        final s = await api.refreshStatus();
        if (!s.running) {
          t.cancel();
          if (!completer.isCompleted) completer.complete();
        }
      } catch (_) {
        t.cancel();
        if (!completer.isCompleted) completer.complete();
      }
    });
    return completer.future;
  }

  void _snack(String msg, {bool error = false}) {
    if (error) {
      AppToast.error(context, 'Greška', message: msg);
    } else {
      AppToast.success(context, 'Uspešno', message: msg);
    }
  }

  /// "Poslednje preuzimanje / pre N minuta" — mirrors the web header.
  Widget _lastFetchedLabel(AppColors c) {
    final finished =
        ref.watch(refreshStatusProvider).asData?.value.finishedAt ?? 0;
    return Column(
      mainAxisSize: MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.center,
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        Text(
          'Poslednje preuzimanje'.t,
          style: TextStyle(fontSize: 11, height: 1.2, color: c.mutedForeground),
        ),
        Text(
          fmt.relativeTimeSr(finished),
          style: TextStyle(
            fontSize: 12,
            height: 1.3,
            fontWeight: FontWeight.w500,
            color: c.foreground,
          ),
        ),
      ],
    );
  }

  Future<void> _openQr(Receipt r) async {
    final api = ref.read(apiClientProvider);
    await QrDialog.show(
      context,
      api: api,
      receipt: r,
      onMarkPaid: () async {
        await api.markPaid(r.period, r.provider, true);
        ref.invalidate(receiptsProvider);
        ref.invalidate(statsProvider);
      },
    );
  }

  Future<void> _openPdf(Receipt r) async {
    final url = ref.read(apiClientProvider).pdfUrl(r);
    final uri = Uri.parse(url);
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (mounted) _snack('Ne mogu da otvorim račun.', error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    I18n.cyrillic = ref.watch(appConfigProvider.select((x) => x.cyrillic));
    final receiptsAsync = ref.watch(receiptsProvider);
    final providersAsync = ref.watch(providersProvider);

    return Scaffold(
      appBar: AppBar(
        titleSpacing: 16,
        title: Text('Računi'.t),
        actions: [
          _lastFetchedLabel(c),
          const SizedBox(width: 4),
          Padding(
            padding: const EdgeInsets.only(right: 8),
            child: IconButton(
              tooltip: 'Osveži'.t,
              onPressed: _refreshing ? null : _runRefresh,
              icon: _refreshing
                  ? SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: c.foreground,
                      ),
                    )
                  : const Icon(Icons.refresh_rounded),
            ),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => ref.invalidate(receiptsProvider),
        child: receiptsAsync.when(
          loading: () => _loadingList(),
          error: (e, _) => _errorState(c, e),
          data: (receipts) {
            final providers =
                providersAsync.asData?.value ??
                (receipts.map((r) => r.provider.toLowerCase()).toSet().toList()
                  ..sort());
            final periods = _periodsOf(receipts);
            final filtered = _filter(receipts);
            final unpaid = receipts.where((r) => !fmt.isPaid(r)).length;

            return ListView(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
              children: [
                Text(
                  'Svi preuzeti računi, grupisani po provajderu i periodu.',
                  style: TextStyle(fontSize: 14, color: c.mutedForeground),
                ),
                const SizedBox(height: 16),
                _searchField(c),
                const SizedBox(height: 12),
                _periodDropdown(c, periods),
                const SizedBox(height: 12),
                _providerPills(providers),
                const SizedBox(height: 8),
                _paidPills(),
                const SizedBox(height: 12),
                Divider(color: c.border),
                const SizedBox(height: 8),
                _summary(c, filtered.length, receipts.length, unpaid),
                const SizedBox(height: 8),
                if (filtered.isEmpty)
                  _emptyState(c)
                else
                  ...filtered.map(
                    (r) => Padding(
                      padding: const EdgeInsets.only(bottom: 16),
                      child: ReceiptCard(
                        receipt: r,
                        onOpenQr: () => _openQr(r),
                        onOpenPdf: () => _openPdf(r),
                      ),
                    ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _searchField(AppColors c) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        FieldLabel('Pretraga'.t),
        TextField(
          controller: _searchController,
          onChanged: (v) => setState(() => _query = v),
          decoration: InputDecoration(
            prefixIcon: Icon(Icons.search_rounded, size: 20),
            hintText: 'Provajder, period, naziv fajla...'.t,
          ),
        ),
      ],
    );
  }

  Widget _periodDropdown(AppColors c, List<String> periods) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        FieldLabel('Period'.t),
        DropdownButtonFormField<String>(
          initialValue: _selectedPeriod.isEmpty ? '' : _selectedPeriod,
          isExpanded: true,
          decoration: InputDecoration(),
          items: [
            DropdownMenuItem(value: '', child: Text('Svi periodi'.t)),
            for (final p in periods) DropdownMenuItem(value: p, child: Text(p)),
          ],
          onChanged: (v) => setState(() => _selectedPeriod = v ?? ''),
        ),
      ],
    );
  }

  Widget _providerPills(List<String> providers) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        Pill(
          label: 'Svi provajderi'.t,
          active: _selectedProviders.isEmpty,
          onTap: () => setState(_selectedProviders.clear),
        ),
        for (final p in providers)
          Pill(
            label: fmt.providerLabel(p),
            active: _selectedProviders.contains(p.toLowerCase()),
            onTap: () => setState(() {
              final key = p.toLowerCase();
              if (!_selectedProviders.remove(key)) _selectedProviders.add(key);
            }),
          ),
      ],
    );
  }

  Widget _paidPills() {
    const filters = [
      ('all', 'Sve'),
      ('paid', 'Plaćeno'),
      ('unpaid', 'Neplaćeno'),
    ];
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final f in filters)
          Pill(
            label: f.$2,
            active: _paidFilter == f.$1,
            onTap: () => setState(() => _paidFilter = f.$1),
          ),
      ],
    );
  }

  Widget _summary(AppColors c, int shown, int total, int unpaid) {
    return Row(
      children: [
        Expanded(
          child: Wrap(
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              Text(
                'Prikazano '.t,
                style: TextStyle(fontSize: 14, color: c.mutedForeground),
              ),
              Text(
                '$shown'.t,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: c.foreground,
                ),
              ),
              Text(
                ' od '.t,
                style: TextStyle(fontSize: 14, color: c.mutedForeground),
              ),
              Text(
                '$total'.t,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: c.foreground,
                ),
              ),
              Text(
                ' računa'.t,
                style: TextStyle(fontSize: 14, color: c.mutedForeground),
              ),
              if (unpaid > 0) ...[
                Text(
                  ' · '.t,
                  style: TextStyle(fontSize: 14, color: c.mutedForeground),
                ),
                Text(
                  '$unpaid'.t,
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w500,
                    color: c.warning,
                  ),
                ),
                Text(
                  ' neplaćeno'.t,
                  style: TextStyle(fontSize: 14, color: c.mutedForeground),
                ),
              ],
            ],
          ),
        ),
        if (_hasActiveFilters)
          AppButton(
            label: 'Poništi filtere'.t,
            variant: ButtonVariant.ghost,
            onPressed: _resetFilters,
          ),
      ],
    );
  }

  Widget _emptyState(AppColors c) {
    return Container(
      margin: const EdgeInsets.only(top: 8),
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 48),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(AppColors.radiusXl),
        border: Border.all(color: c.border, style: BorderStyle.solid, width: 1),
      ),
      child: Column(
        children: [
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(color: c.muted, shape: BoxShape.circle),
            child: Icon(
              Icons.description_outlined,
              color: c.mutedForeground,
              size: 20,
            ),
          ),
          const SizedBox(height: 16),
          Text(
            'Nema računa'.t,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: c.foreground,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            'Nijedan račun ne odgovara izabranim filterima. Pokušajte da promenite pretragu ili period.',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 14, color: c.mutedForeground),
          ),
          if (_hasActiveFilters) ...[
            const SizedBox(height: 16),
            AppButton(
              label: 'Poništi filtere'.t,
              variant: ButtonVariant.outline,
              onPressed: _resetFilters,
            ),
          ],
        ],
      ),
    );
  }

  Widget _loadingList() {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
      children: List.generate(
        5,
        (_) => const Padding(
          padding: EdgeInsets.only(bottom: 16),
          child: _SkeletonCard(),
        ),
      ),
    );
  }

  Widget _errorState(AppColors c, Object e) {
    return ListView(
      children: [
        Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            children: [
              const SizedBox(height: 40),
              Icon(Icons.cloud_off_rounded, size: 40, color: c.mutedForeground),
              const SizedBox(height: 16),
              Text(
                'Ne mogu da učitam račune'.t,
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  color: c.foreground,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                'Proverite adresu servera u Podešavanjima.\n$e',
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 13, color: c.mutedForeground),
              ),
              const SizedBox(height: 16),
              AppButton(
                label: 'Pokušaj ponovo'.t,
                variant: ButtonVariant.outline,
                onPressed: () => ref.invalidate(receiptsProvider),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _SkeletonCard extends StatelessWidget {
  const _SkeletonCard();
  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    Widget bar(double w, double h) => Container(
      width: w,
      height: h,
      decoration: BoxDecoration(
        color: c.muted,
        borderRadius: BorderRadius.circular(6),
      ),
    );
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          bar(64, 20),
          const SizedBox(height: 16),
          bar(112, 28),
          const SizedBox(height: 8),
          bar(80, 16),
          const SizedBox(height: 20),
          bar(double.infinity, 36),
        ],
      ),
    );
  }
}
