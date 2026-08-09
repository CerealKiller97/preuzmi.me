import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/models.dart';
import '../state/app_state.dart';
import '../theme/app_colors.dart';
import '../util/i18n.dart';
import '../util/format.dart' as fmt;
import '../widgets/ui.dart';

const List<String> _months = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'Maj',
  'Jun',
  'Jul',
  'Avg',
  'Sep',
  'Okt',
  'Nov',
  'Dec',
];

/// The Statistics screen — mirrors `templates/stats.html`: year + provider
/// filters, four stat cards, the monthly bar/line chart, the provider×month
/// table, and the per-provider breakdown.
class StatsScreen extends ConsumerStatefulWidget {
  const StatsScreen({super.key});

  @override
  ConsumerState<StatsScreen> createState() => _StatsScreenState();
}

class _StatsScreenState extends ConsumerState<StatsScreen> {
  bool _bar = true;
  int _touchedPie = -1;

  // Chart tooltip surface — matches the web's --chart-tooltip-* tokens
  // (a dark card in both themes).
  static const Color _tipBg = Color(0xFF18181B);
  static const Color _tipFg = Color(0xFFFAFAFA);

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    I18n.cyrillic = ref.watch(appConfigProvider.select((x) => x.cyrillic));
    final statsAsync = ref.watch(statsProvider);
    final providersAsync = ref.watch(providersProvider);
    final years = ref.watch(availableYearsProvider);
    final selectedYear = ref.watch(selectedYearProvider);
    final selectedProviders = ref.watch(statsProviderFilterProvider);

    return Scaffold(
      appBar: AppBar(title: Text('Statistika'.t)),
      body: RefreshIndicator(
        onRefresh: () async => ref.invalidate(statsProvider),
        child: ListView(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
          children: [
            Text(
              'Pregled troškova po mesecima i provajderima.'.t,
              style: TextStyle(fontSize: 14, color: c.mutedForeground),
            ),
            const SizedBox(height: 16),
            _yearDropdown(c, years, selectedYear),
            const SizedBox(height: 12),
            _providerPills(
              providersAsync.asData?.value ?? const [],
              selectedProviders,
            ),
            const SizedBox(height: 16),
            statsAsync.when(
              loading: () => const Padding(
                padding: EdgeInsets.symmetric(vertical: 60),
                child: Center(child: CircularProgressIndicator()),
              ),
              error: (e, _) => _error(c, e),
              data: (s) => _content(c, s, selectedProviders),
            ),
          ],
        ),
      ),
    );
  }

  Widget _content(AppColors c, Stats s, Set<String> selectedProviders) {
    final peak = _peakMonth(s);
    final count = s.monthlyCounts.fold<int>(0, (a, b) => a + b);
    final providersLabel = selectedProviders.isEmpty
        ? 'Svi provajderi'.t
        : selectedProviders.map(fmt.providerLabel).join(', ');

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Stat cards — 2-up grid.
        Row(
          children: [
            Expanded(
              child: _statCard(
                c,
                'Ukupno godišnje',
                fmt.formatMoney(s.total, s.currency),
                'Za ${s.year}.',
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: _statCard(
                c,
                'Prosek mesečno',
                fmt.formatMoney(s.average, s.currency),
                'Prosek preko 12 meseci.',
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: _statCard(
                c,
                'Najskuplji mesec',
                fmt.formatMoney(peak.$2, s.currency),
                peak.$1.isEmpty ? '—' : peak.$1,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: _statCard(c, 'Broj računa', '$count', providersLabel),
            ),
          ],
        ),
        const SizedBox(height: 16),
        // Monthly chart.
        SectionCard(
          title: 'Mesečni troškovi'.t,
          subtitle: 'Prikaz za ${s.year}.'.t,
          trailing: _chartToggle(c),
          child: Column(
            children: [
              SizedBox(
                height: 240,
                child: s.total == 0
                    ? _noData(c)
                    : (_bar ? _barChart(c, s) : _lineChart(c, s)),
              ),
              if (s.total != 0 && s.monthlyByProvider.length > 1) ...[
                const SizedBox(height: 16),
                _seriesLegend(c, s),
              ],
            ],
          ),
        ),
        const SizedBox(height: 16),
        // Provider × month table.
        SectionCard(
          title: 'Cene po provajderu i mesecu'.t,
          subtitle: 'Iznosi računa za ${s.year}, sa godišnjim zbirom.'.t,
          child: s.monthlyByProvider.isEmpty
              ? _noData(c)
              : _providerTable(c, s),
        ),
        const SizedBox(height: 16),
        // Per-provider breakdown.
        SectionCard(
          title: 'Po provajderu'.t,
          subtitle: 'Udeo svakog provajdera u ukupnom trošku.'.t,
          child: s.byProvider.isEmpty ? _noData(c) : _providerBreakdown(c, s),
        ),
      ],
    );
  }

  (String, double) _peakMonth(Stats s) {
    var best = -1;
    for (var i = 0; i < s.monthly.length; i++) {
      if (s.monthly[i] > 0 && (best < 0 || s.monthly[i] > s.monthly[best])) {
        best = i;
      }
    }
    if (best < 0) return ('', 0);
    return (_months[best], s.monthly[best]);
  }

  Widget _yearDropdown(AppColors c, List<int> years, int selected) {
    final value = years.contains(selected) ? selected : years.first;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        FieldLabel('Godina'.t),
        SizedBox(
          width: 160,
          child: DropdownButtonFormField<int>(
            initialValue: value,
            isExpanded: true,
            items: [
              for (final y in years)
                DropdownMenuItem(value: y, child: Text('$y'.t)),
            ],
            onChanged: (v) {
              if (v != null) {
                ref.read(selectedYearProvider.notifier).state = v;
              }
            },
          ),
        ),
      ],
    );
  }

  Widget _providerPills(List<String> providers, Set<String> selected) {
    void toggle(String? p) {
      final notifier = ref.read(statsProviderFilterProvider.notifier);
      if (p == null) {
        notifier.state = {};
        return;
      }
      final next = Set<String>.from(selected);
      final key = p.toLowerCase();
      if (!next.remove(key)) next.add(key);
      notifier.state = next;
    }

    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        Pill(
          label: 'Svi provajderi'.t,
          active: selected.isEmpty,
          onTap: () => toggle(null),
        ),
        for (final p in providers)
          Pill(
            label: fmt.providerLabel(p),
            active: selected.contains(p.toLowerCase()),
            onTap: () => toggle(p),
          ),
      ],
    );
  }

  Widget _statCard(AppColors c, String label, String value, String sub) {
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label.t,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: c.mutedForeground,
            ),
          ),
          const SizedBox(height: 8),
          FittedBox(
            fit: BoxFit.scaleDown,
            alignment: Alignment.centerLeft,
            child: Text(
              value,
              style: TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w600,
                letterSpacing: -0.4,
                color: c.foreground,
              ),
            ),
          ),
          const SizedBox(height: 4),
          Text(
            sub.t,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(fontSize: 12, color: c.mutedForeground),
          ),
        ],
      ),
    );
  }

  Widget _chartToggle(AppColors c) {
    Widget seg(String label, bool active, VoidCallback onTap) => Material(
      color: active ? c.background : Colors.transparent,
      borderRadius: BorderRadius.circular(AppColors.radiusSm),
      child: InkWell(
        borderRadius: BorderRadius.circular(AppColors.radiusSm),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
          child: Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w500,
              color: active ? c.foreground : c.mutedForeground,
            ),
          ),
        ),
      ),
    );
    return Container(
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        color: c.muted,
        borderRadius: BorderRadius.circular(AppColors.radiusMd),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          seg('Stubovi', _bar, () => setState(() => _bar = true)),
          seg('Linija', !_bar, () => setState(() => _bar = false)),
        ],
      ),
    );
  }

  double _maxMonthly(Stats s) {
    var m = 0.0;
    for (final v in s.monthly) {
      if (v > m) m = v;
    }
    return m == 0 ? 1 : m;
  }

  FlTitlesData _titles(AppColors c, Stats s) {
    return FlTitlesData(
      topTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
      rightTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
      leftTitles: AxisTitles(
        sideTitles: SideTitles(
          showTitles: true,
          reservedSize: 44,
          getTitlesWidget: (value, meta) {
            if (value == meta.min || value == meta.max) {
              return const SizedBox.shrink();
            }
            return Text(
              _short(value),
              style: TextStyle(fontSize: 10, color: c.mutedForeground),
            );
          },
        ),
      ),
      bottomTitles: AxisTitles(
        sideTitles: SideTitles(
          showTitles: true,
          reservedSize: 24,
          interval: 1,
          getTitlesWidget: (value, meta) {
            final i = value.toInt();
            if (i < 0 || i > 11) return const SizedBox.shrink();
            return Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text(
                _months[i],
                style: TextStyle(fontSize: 9, color: c.mutedForeground),
              ),
            );
          },
        ),
      ),
    );
  }

  String _short(double v) {
    if (v >= 1000) return '${(v / 1000).toStringAsFixed(0)}k';
    return v.toStringAsFixed(0);
  }

  double _valueAt(ProviderMonthly row, int i) =>
      i < row.monthly.length ? row.monthly[i] : 0;

  /// Stacked bars, one brand-coloured segment per provider per month —
  /// matching the web `barDatasets()` and the donut's colours.
  Widget _barChart(AppColors c, Stats s) {
    final max = _maxMonthly(s);
    final rows = s.monthlyByProvider;
    return BarChart(
      BarChartData(
        maxY: max * 1.15,
        gridData: FlGridData(
          show: true,
          drawVerticalLine: false,
          getDrawingHorizontalLine: (v) =>
              FlLine(color: c.border, strokeWidth: 1),
        ),
        borderData: FlBorderData(show: false),
        titlesData: _titles(c, s),
        barTouchData: BarTouchData(
          touchTooltipData: BarTouchTooltipData(
            getTooltipColor: (_) => _tipBg,
            tooltipBorderRadius: BorderRadius.circular(8),
            tooltipPadding: const EdgeInsets.symmetric(
              horizontal: 10,
              vertical: 8,
            ),
            getTooltipItem: (group, groupIndex, rod, rodIndex) =>
                _barTooltip(c, s, rows, group.x),
          ),
        ),
        barGroups: [for (var i = 0; i < 12; i++) _barGroup(c, s, rows, i)],
      ),
    );
  }

  BarTooltipItem _barTooltip(
    AppColors c,
    Stats s,
    List<ProviderMonthly> rows,
    int i,
  ) {
    final children = <TextSpan>[];
    for (var p = 0; p < rows.length; p++) {
      final v = _valueAt(rows[p], i);
      if (v <= 0) continue;
      children.add(
        TextSpan(
          text:
              '\n${fmt.providerLabel(rows[p].provider)}: '
              '${fmt.formatMoney(v, s.currency)}',
          style: TextStyle(
            color: c.providerColor(rows[p].provider, p),
            fontSize: 12,
            fontWeight: FontWeight.w500,
          ),
        ),
      );
    }
    final total = i < s.monthly.length ? s.monthly[i] : 0;
    children.add(
      TextSpan(
        text: '\nUkupno: ${fmt.formatMoney(total, s.currency)}',
        style: const TextStyle(
          color: _tipFg,
          fontSize: 12,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
    return BarTooltipItem(
      _months[i],
      const TextStyle(color: _tipFg, fontSize: 13, fontWeight: FontWeight.w600),
      children: children,
    );
  }

  BarChartGroupData _barGroup(
    AppColors c,
    Stats s,
    List<ProviderMonthly> rows,
    int i,
  ) {
    // No per-provider breakdown (empty year): a single-colour total bar.
    if (rows.isEmpty) {
      return BarChartGroupData(
        x: i,
        barRods: [
          BarChartRodData(
            toY: i < s.monthly.length ? s.monthly[i] : 0,
            color: c.chartSeries.first,
            width: 14,
            borderRadius: const BorderRadius.vertical(top: Radius.circular(4)),
          ),
        ],
      );
    }
    var acc = 0.0;
    final stack = <BarChartRodStackItem>[];
    for (var p = 0; p < rows.length; p++) {
      final v = _valueAt(rows[p], i);
      if (v <= 0) continue;
      stack.add(
        BarChartRodStackItem(
          acc,
          acc + v,
          c.providerColor(rows[p].provider, p),
        ),
      );
      acc += v;
    }
    return BarChartGroupData(
      x: i,
      barRods: [
        BarChartRodData(
          toY: acc,
          color: Colors.transparent,
          width: 16,
          borderRadius: const BorderRadius.vertical(top: Radius.circular(4)),
          rodStackItems: stack,
        ),
      ],
    );
  }

  /// The total as a filled area, with one thin brand-coloured line per provider
  /// on top — matching the web `lineDatasets()`.
  Widget _lineChart(AppColors c, Stats s) {
    final max = _maxMonthly(s);
    final rows = s.monthlyByProvider;
    return LineChart(
      LineChartData(
        maxY: max * 1.15,
        minY: 0,
        gridData: FlGridData(
          show: true,
          drawVerticalLine: false,
          getDrawingHorizontalLine: (v) =>
              FlLine(color: c.border, strokeWidth: 1),
        ),
        borderData: FlBorderData(show: false),
        titlesData: _titles(c, s),
        lineTouchData: LineTouchData(
          touchTooltipData: LineTouchTooltipData(
            getTooltipColor: (_) => _tipBg,
            tooltipBorderRadius: BorderRadius.circular(8),
            getTooltipItems: (touchedSpots) {
              return touchedSpots.map((spot) {
                final isTotal = spot.barIndex == 0;
                final row = isTotal ? null : rows[spot.barIndex - 1];
                final color = isTotal
                    ? _tipFg
                    : c.providerColor(row!.provider, spot.barIndex - 1);
                final label = isTotal
                    ? 'Ukupno'
                    : fmt.providerLabel(row!.provider);
                return LineTooltipItem(
                  '$label: ${fmt.formatMoney(spot.y, s.currency)}',
                  TextStyle(
                    color: color,
                    fontSize: 12,
                    fontWeight: isTotal ? FontWeight.w600 : FontWeight.w500,
                  ),
                );
              }).toList();
            },
          ),
        ),
        lineBarsData: [
          LineChartBarData(
            spots: [
              for (var i = 0; i < 12; i++)
                FlSpot(i.toDouble(), i < s.monthly.length ? s.monthly[i] : 0),
            ],
            isCurved: true,
            color: c.foreground,
            barWidth: 2.5,
            dotData: const FlDotData(show: false),
            belowBarData: BarAreaData(
              show: true,
              color: c.foreground.withValues(alpha: 0.10),
            ),
          ),
          for (var p = 0; p < rows.length; p++)
            LineChartBarData(
              spots: [
                for (var i = 0; i < 12; i++)
                  FlSpot(i.toDouble(), _valueAt(rows[p], i)),
              ],
              isCurved: true,
              color: c.providerColor(rows[p].provider, p),
              barWidth: 1.5,
              dotData: const FlDotData(show: false),
            ),
        ],
      ),
    );
  }

  /// Colour key for the monthly chart's provider series.
  Widget _seriesLegend(AppColors c, Stats s) {
    return Wrap(
      spacing: 16,
      runSpacing: 8,
      children: [
        for (var p = 0; p < s.monthlyByProvider.length; p++)
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 10,
                height: 10,
                decoration: BoxDecoration(
                  color: c.providerColor(s.monthlyByProvider[p].provider, p),
                  shape: BoxShape.circle,
                ),
              ),
              const SizedBox(width: 6),
              Text(
                fmt.providerLabel(s.monthlyByProvider[p].provider),
                style: TextStyle(fontSize: 12, color: c.mutedForeground),
              ),
            ],
          ),
      ],
    );
  }

  Widget _providerTable(AppColors c, Stats s) {
    final rows = s.monthlyByProvider;
    final headerStyle = TextStyle(
      fontSize: 12,
      fontWeight: FontWeight.w500,
      color: c.mutedForeground,
    );
    final cellMuted = TextStyle(fontSize: 12, color: c.mutedForeground);
    final cellStrong = TextStyle(
      fontSize: 12,
      fontWeight: FontWeight.w500,
      color: c.foreground,
    );

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: DataTable(
        headingRowHeight: 34,
        dataRowMinHeight: 34,
        dataRowMaxHeight: 42,
        columnSpacing: 20,
        horizontalMargin: 0,
        dividerThickness: 1,
        columns: [
          DataColumn(label: Text('Mesec'.t, style: headerStyle)),
          for (final r in rows)
            DataColumn(
              label: Text(fmt.providerLabel(r.provider), style: headerStyle),
              numeric: true,
            ),
          DataColumn(
            label: Text('Ukupno'.t, style: headerStyle),
            numeric: true,
          ),
        ],
        rows: [
          for (var i = 0; i < 12; i++)
            DataRow(
              cells: [
                DataCell(Text(_months[i], style: cellStrong)),
                for (final r in rows)
                  DataCell(
                    Text(
                      i < r.monthly.length && r.monthly[i] != 0
                          ? fmt.formatMoney(r.monthly[i], s.currency)
                          : '—',
                      style: cellMuted,
                    ),
                  ),
                DataCell(
                  Text(
                    i < s.monthly.length && s.monthly[i] != 0
                        ? fmt.formatMoney(s.monthly[i], s.currency)
                        : '—',
                    style: cellStrong,
                  ),
                ),
              ],
            ),
          DataRow(
            cells: [
              DataCell(
                Text(
                  'Godišnje'.t,
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    color: c.foreground,
                  ),
                ),
              ),
              for (final r in rows)
                DataCell(
                  Text(
                    fmt.formatMoney(
                      r.monthly.fold<double>(0, (a, b) => a + b),
                      s.currency,
                    ),
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: c.foreground,
                    ),
                  ),
                ),
              DataCell(
                Text(
                  fmt.formatMoney(s.total, s.currency),
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    color: c.foreground,
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _providerBreakdown(AppColors c, Stats s) {
    final total = s.total;
    return Column(
      children: [
        SizedBox(
          height: 200,
          child: Stack(
            alignment: Alignment.center,
            children: [
              PieChart(
                PieChartData(
                  sectionsSpace: 2,
                  centerSpaceRadius: 56,
                  pieTouchData: PieTouchData(
                    touchCallback: (event, resp) {
                      setState(() {
                        if (!event.isInterestedForInteractions ||
                            resp == null ||
                            resp.touchedSection == null) {
                          _touchedPie = -1;
                          return;
                        }
                        _touchedPie = resp.touchedSection!.touchedSectionIndex;
                      });
                    },
                  ),
                  sections: [
                    for (var i = 0; i < s.byProvider.length; i++)
                      PieChartSectionData(
                        value: s.byProvider[i].amount,
                        color: c.providerColor(s.byProvider[i].provider, i),
                        radius: _touchedPie == i ? 48 : 40,
                        showTitle: false,
                      ),
                  ],
                ),
              ),
              _donutCenter(c, s),
            ],
          ),
        ),
        const SizedBox(height: 16),
        for (var i = 0; i < s.byProvider.length; i++)
          Builder(
            builder: (_) {
              final p = s.byProvider[i];
              return Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: Row(
                  children: [
                    Container(
                      width: 12,
                      height: 12,
                      decoration: BoxDecoration(
                        color: c.providerColor(p.provider, i),
                        borderRadius: BorderRadius.circular(3),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        fmt.providerLabel(p.provider),
                        style: TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w500,
                          color: c.foreground,
                        ),
                      ),
                    ),
                    Text(
                      fmt.formatMoney(p.amount, s.currency),
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w500,
                        color: c.foreground,
                      ),
                    ),
                    const SizedBox(width: 8),
                    SizedBox(
                      width: 44,
                      child: Text(
                        total > 0
                            ? '${(p.amount / total * 100).toStringAsFixed(0)}%'
                            : '0%',
                        textAlign: TextAlign.right,
                        style: TextStyle(
                          fontSize: 12,
                          color: c.mutedForeground,
                        ),
                      ),
                    ),
                  ],
                ),
              );
            },
          ),
      ],
    );
  }

  /// The donut's centre readout: the total by default, or the touched
  /// provider's share while a section is pressed (acts as the tooltip).
  Widget _donutCenter(AppColors c, Stats s) {
    if (_touchedPie >= 0 && _touchedPie < s.byProvider.length) {
      final p = s.byProvider[_touchedPie];
      final pct = s.total > 0 ? p.amount / s.total * 100 : 0;
      return Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            fmt.providerLabel(p.provider),
            style: TextStyle(fontSize: 12, color: c.mutedForeground),
          ),
          const SizedBox(height: 2),
          Text(
            fmt.formatMoney(p.amount, s.currency),
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: c.foreground,
            ),
          ),
          Text(
            '${pct.toStringAsFixed(0)}%'.t,
            style: TextStyle(fontSize: 12, color: c.mutedForeground),
          ),
        ],
      );
    }
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          'Ukupno'.t,
          style: TextStyle(fontSize: 12, color: c.mutedForeground),
        ),
        const SizedBox(height: 2),
        Text(
          fmt.formatMoney(s.total, s.currency),
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w600,
            color: c.foreground,
          ),
        ),
      ],
    );
  }

  Widget _noData(AppColors c) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 24),
    child: Text(
      'Nema podataka za izabranu godinu.'.t,
      style: TextStyle(fontSize: 14, color: c.mutedForeground),
    ),
  );

  Widget _error(AppColors c, Object e) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 40),
    child: Column(
      children: [
        Icon(Icons.cloud_off_rounded, size: 36, color: c.mutedForeground),
        const SizedBox(height: 12),
        Text(
          'Ne mogu da učitam statistiku'.t,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w600,
            color: c.foreground,
          ),
        ),
        const SizedBox(height: 12),
        AppButton(
          label: 'Pokušaj ponovo'.t,
          variant: ButtonVariant.outline,
          onPressed: () => ref.invalidate(statsProvider),
        ),
      ],
    ),
  );
}
