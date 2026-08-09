import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'screens/home_shell.dart';
import 'state/app_state.dart';
import 'theme/app_theme.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final prefs = await SharedPreferences.getInstance();
  runApp(
    ProviderScope(
      overrides: [sharedPrefsProvider.overrideWithValue(prefs)],
      child: const PreuzmiApp(),
    ),
  );
}

/// The mobile client for preuzmi.me — a Flutter port of the web UI that talks to
/// the Go server's JSON API.
class PreuzmiApp extends ConsumerWidget {
  const PreuzmiApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final mode = ref.watch(appConfigProvider.select((c) => c.themeMode));
    return MaterialApp(
      title: 'Preuzmi.me',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light(),
      darkTheme: AppTheme.dark(),
      themeMode: mode,
      home: const HomeShell(),
    );
  }
}
