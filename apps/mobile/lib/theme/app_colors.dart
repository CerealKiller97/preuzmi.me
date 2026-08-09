import 'package:flutter/material.dart';

/// The full shadcn/ui **zinc** palette used by the web app, ported from
/// `apps/server/assets/css/app.css`. The web defines these as oklch tokens;
/// these are their sRGB equivalents (zinc scale), plus the canvas-safe hex
/// brand/series colours the CSS already ships verbatim.
///
/// One [AppColors] exists per brightness. Look them up through
/// `Theme.of(context).extension<AppColors>()!` — see [appColorsOf].
@immutable
class AppColors extends ThemeExtension<AppColors> {
  const AppColors({
    required this.background,
    required this.foreground,
    required this.card,
    required this.cardForeground,
    required this.popover,
    required this.popoverForeground,
    required this.primary,
    required this.primaryForeground,
    required this.secondary,
    required this.secondaryForeground,
    required this.muted,
    required this.mutedForeground,
    required this.accent,
    required this.accentForeground,
    required this.destructive,
    required this.success,
    required this.warning,
    required this.border,
    required this.input,
    required this.ring,
    required this.brand,
    required this.chartSeries,
  });

  final Color background;
  final Color foreground;
  final Color card;
  final Color cardForeground;
  final Color popover;
  final Color popoverForeground;
  final Color primary;
  final Color primaryForeground;
  final Color secondary;
  final Color secondaryForeground;
  final Color muted;
  final Color mutedForeground;
  final Color accent;
  final Color accentForeground;
  final Color destructive;
  final Color success;
  final Color warning;
  final Color border;
  final Color input;
  final Color ring;

  /// Provider brand colours, keyed by provider slug (a1, eps, mts, ...).
  final Map<String, Color> brand;

  /// Generic categorical series palette (canvas-safe `--chart-series-1..5`),
  /// used as a fallback for providers without a brand colour.
  final List<Color> chartSeries;

  /// The border-radius scale from the CSS (`--radius: 0.625rem`).
  static const double radius = 10; // 0.625rem
  static const double radiusSm = 6; // radius - 4
  static const double radiusMd = 8; // radius - 2
  static const double radiusLg = 10; // radius
  static const double radiusXl = 14; // radius + 4 (used by .card)

  /// The colour a provider's chart series / badge uses. Falls back to the
  /// muted foreground for providers without a brand colour.
  Color brandFor(String provider) =>
      brand[provider.toLowerCase()] ?? mutedForeground;

  /// The colour for one provider's chart series — its brand colour when we have
  /// one, otherwise a stable slot from the generic palette. Mirrors the web's
  /// `providerColor(provider, i)`, so bars, lines and the donut agree.
  Color providerColor(String provider, int i) =>
      brand[provider.toLowerCase()] ?? chartSeries[i % chartSeries.length];

  static const AppColors light = AppColors(
    background: Color(0xFFFFFFFF),
    foreground: Color(0xFF09090B),
    card: Color(0xFFFFFFFF),
    cardForeground: Color(0xFF09090B),
    popover: Color(0xFFFFFFFF),
    popoverForeground: Color(0xFF09090B),
    primary: Color(0xFF18181B),
    primaryForeground: Color(0xFFFAFAFA),
    secondary: Color(0xFFF4F4F5),
    secondaryForeground: Color(0xFF18181B),
    muted: Color(0xFFF4F4F5),
    mutedForeground: Color(0xFF71717A),
    accent: Color(0xFFF4F4F5),
    accentForeground: Color(0xFF18181B),
    destructive: Color(0xFFDC2626),
    success: Color(0xFF059669),
    warning: Color(0xFFD97706),
    border: Color(0xFFE4E4E7),
    input: Color(0xFFE4E4E7),
    ring: Color(0xFFA1A1AA),
    brand: {
      'eps': Color(0xFF24418C),
      'a1': Color(0xFFE8321E),
      'mts': Color(0xFFE64553),
      'yettel': Color(0xFFB8E438),
      'esanduce': Color(0xFF4B8EF7),
      'eupravnik': Color(0xFF2A9D90),
    },
    chartSeries: [
      Color(0xFFE76E50),
      Color(0xFF2A9D90),
      Color(0xFF274754),
      Color(0xFFE8C468),
      Color(0xFFF4A462),
    ],
  );

  static const AppColors dark = AppColors(
    background: Color(0xFF09090B),
    foreground: Color(0xFFFAFAFA),
    card: Color(0xFF18181B),
    cardForeground: Color(0xFFFAFAFA),
    popover: Color(0xFF18181B),
    popoverForeground: Color(0xFFFAFAFA),
    primary: Color(0xFFE4E4E7),
    primaryForeground: Color(0xFF18181B),
    secondary: Color(0xFF27272A),
    secondaryForeground: Color(0xFFFAFAFA),
    muted: Color(0xFF27272A),
    mutedForeground: Color(0xFFA1A1AA),
    accent: Color(0xFF27272A),
    accentForeground: Color(0xFFFAFAFA),
    destructive: Color(0xFFF87171),
    success: Color(0xFF10B981),
    warning: Color(0xFFF59E0B),
    border: Color(0x1AFFFFFF), // white / 10%
    input: Color(0x26FFFFFF), // white / 15%
    ring: Color(0xFF52525B),
    brand: {
      'eps': Color(0xFF4A6FD4),
      'a1': Color(0xFFE8321E),
      'mts': Color(0xFFE64553),
      'yettel': Color(0xFFCCFA4E),
      'esanduce': Color(0xFF4B8EF7),
      'eupravnik': Color(0xFF2EB88A),
    },
    chartSeries: [
      Color(0xFF2662D9),
      Color(0xFF2EB88A),
      Color(0xFFE88C30),
      Color(0xFFAF57DB),
      Color(0xFFE23670),
    ],
  );

  @override
  AppColors copyWith() => this;

  @override
  AppColors lerp(ThemeExtension<AppColors>? other, double t) => this;
}

/// Shorthand for `Theme.of(context).extension<AppColors>()!`.
AppColors appColorsOf(BuildContext context) =>
    Theme.of(context).extension<AppColors>()!;
