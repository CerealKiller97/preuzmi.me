import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_state.dart';
import '../util/i18n.dart';
import 'receipts_screen.dart';
import 'settings_screen.dart';
import 'stats_screen.dart';

/// Root shell with the bottom navigation between the three screens, mirroring
/// the web header nav: Računi · Statistika · Podešavanja.
class HomeShell extends ConsumerStatefulWidget {
  const HomeShell({super.key});

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell> {
  // Dev override: `--dart-define=INITIAL_TAB=1` starts on Stats/Settings, so
  // each screen can be verified without tapping. Defaults to Receipts.
  int _index = int.tryParse(const String.fromEnvironment('INITIAL_TAB')) ?? 0;

  static const _screens = [ReceiptsScreen(), StatsScreen(), SettingsScreen()];

  // Drives the one-time fade-in on first launch.
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) setState(() => _ready = true);
    });
  }

  @override
  Widget build(BuildContext context) {
    I18n.cyrillic = ref.watch(appConfigProvider.select((x) => x.cyrillic));
    return Scaffold(
      // Cross-fade between tabs (all screens stay mounted, so filter/scroll
      // state is preserved) — matching the web UI's fade-in feel.
      body: Stack(
        children: [
          for (var i = 0; i < _screens.length; i++)
            AnimatedOpacity(
              opacity: (_ready && _index == i) ? 1 : 0,
              duration: const Duration(milliseconds: 300),
              curve: Curves.easeOutCubic,
              child: IgnorePointer(
                ignoring: _index != i,
                child: TickerMode(enabled: _index == i, child: _screens[i]),
              ),
            ),
        ],
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
        destinations: [
          NavigationDestination(
            icon: const Icon(Icons.receipt_long_outlined),
            selectedIcon: const Icon(Icons.receipt_long),
            label: 'Računi'.t,
          ),
          NavigationDestination(
            icon: const Icon(Icons.bar_chart_outlined),
            selectedIcon: const Icon(Icons.bar_chart),
            label: 'Statistika'.t,
          ),
          NavigationDestination(
            icon: const Icon(Icons.settings_outlined),
            selectedIcon: const Icon(Icons.settings),
            label: 'Podešavanja'.t,
          ),
        ],
      ),
    );
  }
}
