import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
// Riverpod 3 moved StateProvider / StateNotifier(Provider) to the legacy library.
import 'package:flutter_riverpod/legacy.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/api_client.dart';
import '../api/models.dart';

/// Persisted app preferences: the server URL the client talks to, the theme
/// mode, and the Serbian script (which is also pushed to the server's `lang`).
class AppConfig {
  const AppConfig({
    required this.baseUrl,
    required this.themeMode,
    required this.cyrillic,
  });

  final String baseUrl;
  final ThemeMode themeMode;
  final bool cyrillic;

  AppConfig copyWith({String? baseUrl, ThemeMode? themeMode, bool? cyrillic}) =>
      AppConfig(
        baseUrl: baseUrl ?? this.baseUrl,
        themeMode: themeMode ?? this.themeMode,
        cyrillic: cyrillic ?? this.cyrillic,
      );

  static const String defaultBaseUrl = 'http://localhost:5500';
}

/// Injected in `main` after SharedPreferences loads.
final sharedPrefsProvider = Provider<SharedPreferences>(
  (ref) => throw UnimplementedError('override in main'),
);

class AppConfigNotifier extends StateNotifier<AppConfig> {
  AppConfigNotifier(this._prefs)
    : super(
        AppConfig(
          baseUrl: _prefs.getString(_kBaseUrl) ?? AppConfig.defaultBaseUrl,
          themeMode: _themeFromString(_prefs.getString(_kThemeMode)),
          cyrillic: _prefs.getBool(_kCyrillic) ?? false,
        ),
      );

  final SharedPreferences _prefs;

  static const _kBaseUrl = 'base_url';
  static const _kThemeMode = 'theme_mode';
  static const _kCyrillic = 'cyrillic';

  void setBaseUrl(String url) {
    _prefs.setString(_kBaseUrl, url);
    state = state.copyWith(baseUrl: url);
  }

  void setThemeMode(ThemeMode mode) {
    _prefs.setString(_kThemeMode, mode.name);
    state = state.copyWith(themeMode: mode);
  }

  void setCyrillic(bool value) {
    _prefs.setBool(_kCyrillic, value);
    state = state.copyWith(cyrillic: value);
  }

  static ThemeMode _themeFromString(String? s) => switch (s) {
    'light' => ThemeMode.light,
    'dark' => ThemeMode.dark,
    _ => ThemeMode.system,
  };
}

final appConfigProvider = StateNotifierProvider<AppConfigNotifier, AppConfig>((
  ref,
) {
  return AppConfigNotifier(ref.watch(sharedPrefsProvider));
});

/// Rebuilt whenever the server URL changes.
final apiClientProvider = Provider<ApiClient>((ref) {
  final baseUrl = ref.watch(appConfigProvider.select((c) => c.baseUrl));
  return ApiClient(baseUrl);
});

final providersProvider = FutureProvider<List<String>>((ref) async {
  return ref.watch(apiClientProvider).providers();
});

/// Full receipt list (filtering happens client-side in the UI, matching web).
final receiptsProvider = FutureProvider<List<Receipt>>((ref) async {
  return ref.watch(apiClientProvider).receipts();
});

/// Last refresh run state, for the "Poslednje preuzimanje" header label.
final refreshStatusProvider = FutureProvider<RefreshState>((ref) async {
  return ref.watch(apiClientProvider).refreshStatus();
});

/// Currently selected stats year.
final selectedYearProvider = StateProvider<int>((ref) => DateTime.now().year);

/// Provider slugs the stats screen is filtered to (empty = all).
final statsProviderFilterProvider = StateProvider<Set<String>>(
  (ref) => <String>{},
);

final statsProvider = FutureProvider<Stats>((ref) async {
  final year = ref.watch(selectedYearProvider);
  final provs = ref.watch(statsProviderFilterProvider);
  final provider = provs.isEmpty ? null : provs.join(',');
  return ref.watch(apiClientProvider).stats(year: year, provider: provider);
});

/// Years that actually have receipts, newest first — for the stats year picker.
final availableYearsProvider = Provider<List<int>>((ref) {
  final receipts = ref.watch(receiptsProvider).asData?.value ?? const [];
  final years = <int>{};
  for (final r in receipts) {
    final parts = r.period.replaceAll('/', '-').split('-');
    if (parts.length == 2) {
      final y = int.tryParse(parts[1]);
      if (y != null) years.add(y);
    }
  }
  final now = DateTime.now().year;
  if (years.isEmpty) years.add(now);
  final sorted = years.toList()..sort((a, b) => b.compareTo(a));
  return sorted;
});

final settingsProvider = FutureProvider<Settings>((ref) async {
  return ref.watch(apiClientProvider).settings();
});
