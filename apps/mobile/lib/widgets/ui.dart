import 'package:flutter/material.dart';

import '../theme/app_colors.dart';

/// Shadcn-equivalent widgets, matching the `@layer components` classes in
/// `apps/server/assets/css/app.css` so the mobile UI reuses the same visual
/// vocabulary as the web (`.card`, `.badge-*`, `.pill`, `.btn-*`).

/// `.card` — rounded-xl border, card surface, subtle shadow.
class AppCard extends StatelessWidget {
  const AppCard({super.key, required this.child, this.padding, this.onTap});

  final Widget child;
  final EdgeInsetsGeometry? padding;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    final card = Container(
      decoration: BoxDecoration(
        color: c.card,
        borderRadius: BorderRadius.circular(AppColors.radiusXl),
        border: Border.all(color: c.border),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.05),
            blurRadius: 2,
            offset: const Offset(0, 1),
          ),
        ],
      ),
      padding: padding ?? const EdgeInsets.all(20),
      child: child,
    );
    if (onTap == null) return card;
    return Material(
      color: Colors.transparent,
      child: InkWell(
        borderRadius: BorderRadius.circular(AppColors.radiusXl),
        onTap: onTap,
        child: card,
      ),
    );
  }
}

enum BadgeVariant { secondary, success, warning, destructive, outline }

/// `.badge` + variant. Tinted (bg /10, border /25) for status variants.
class AppBadge extends StatelessWidget {
  const AppBadge(
    this.text, {
    super.key,
    this.variant = BadgeVariant.secondary,
    this.icon,
  });

  final String text;
  final BadgeVariant variant;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    late final Color bg;
    late final Color fg;
    late final Color border;
    switch (variant) {
      case BadgeVariant.secondary:
        bg = c.secondary;
        fg = c.secondaryForeground;
        border = Colors.transparent;
      case BadgeVariant.success:
        bg = c.success.withValues(alpha: 0.10);
        fg = c.success;
        border = c.success.withValues(alpha: 0.25);
      case BadgeVariant.warning:
        bg = c.warning.withValues(alpha: 0.10);
        fg = c.warning;
        border = c.warning.withValues(alpha: 0.25);
      case BadgeVariant.destructive:
        bg = c.destructive.withValues(alpha: 0.10);
        fg = c.destructive;
        border = c.destructive.withValues(alpha: 0.25);
      case BadgeVariant.outline:
        bg = Colors.transparent;
        fg = c.foreground;
        border = c.border;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(AppColors.radiusMd),
        border: Border.all(color: border),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (icon != null) ...[
            Icon(icon, size: 12, color: fg),
            const SizedBox(width: 4),
          ],
          Text(
            text,
            style: TextStyle(
              color: fg,
              fontSize: 12,
              fontWeight: FontWeight.w500,
              height: 1.2,
            ),
          ),
        ],
      ),
    );
  }
}

/// `.pill` filter chip (rounded-full).
class Pill extends StatelessWidget {
  const Pill({
    super.key,
    required this.label,
    required this.active,
    required this.onTap,
    this.leading,
  });

  final String label;
  final bool active;
  final VoidCallback onTap;
  final Widget? leading;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    return Material(
      color: active ? c.primary : c.background,
      shape: StadiumBorder(
        side: BorderSide(color: active ? Colors.transparent : c.border),
      ),
      child: InkWell(
        customBorder: const StadiumBorder(),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (leading != null) ...[leading!, const SizedBox(width: 6)],
              Text(
                label,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                  color: active ? c.primaryForeground : c.foreground,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

enum ButtonVariant { primary, outline, ghost }

/// `.btn` + variant. Height 36 (h-9). Supports a leading icon, full-width, and
/// a loading spinner (disables interaction).
class AppButton extends StatelessWidget {
  const AppButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.variant = ButtonVariant.primary,
    this.icon,
    this.fullWidth = false,
    this.loading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final ButtonVariant variant;
  final IconData? icon;
  final bool fullWidth;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    late final Color bg;
    late final Color fg;
    Color border = Colors.transparent;
    switch (variant) {
      case ButtonVariant.primary:
        bg = c.primary;
        fg = c.primaryForeground;
      case ButtonVariant.outline:
        bg = c.background;
        fg = c.foreground;
        border = c.border;
      case ButtonVariant.ghost:
        bg = Colors.transparent;
        fg = c.foreground;
    }
    final enabled = onPressed != null && !loading;
    final child = Row(
      mainAxisSize: fullWidth ? MainAxisSize.max : MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        if (loading)
          SizedBox(
            width: 15,
            height: 15,
            child: CircularProgressIndicator(strokeWidth: 2, color: fg),
          )
        else if (icon != null)
          Icon(icon, size: 16, color: fg),
        if (loading || icon != null) const SizedBox(width: 8),
        Text(
          label,
          style: TextStyle(
            color: fg,
            fontSize: 14,
            fontWeight: FontWeight.w500,
          ),
        ),
      ],
    );
    return Opacity(
      opacity: enabled ? 1 : 0.5,
      child: Material(
        color: bg,
        borderRadius: BorderRadius.circular(AppColors.radiusMd),
        child: InkWell(
          borderRadius: BorderRadius.circular(AppColors.radiusMd),
          onTap: enabled ? onPressed : null,
          child: Container(
            height: 36,
            padding: const EdgeInsets.symmetric(horizontal: 16),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(AppColors.radiusMd),
              border: Border.all(color: border),
            ),
            alignment: Alignment.center,
            child: child,
          ),
        ),
      ),
    );
  }
}

/// A `.card` with a title/subtitle header — the repeated settings/stats section.
class SectionCard extends StatelessWidget {
  const SectionCard({
    super.key,
    required this.title,
    this.subtitle,
    this.trailing,
    required this.child,
  });

  final String title;
  final String? subtitle;
  final Widget? trailing;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        letterSpacing: -0.2,
                        color: c.foreground,
                      ),
                    ),
                    if (subtitle != null) ...[
                      const SizedBox(height: 2),
                      Text(
                        subtitle!,
                        style: TextStyle(
                          fontSize: 14,
                          color: c.mutedForeground,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              ?trailing,
            ],
          ),
          const SizedBox(height: 16),
          child,
        ],
      ),
    );
  }
}

/// Muted label for a form field.
class FieldLabel extends StatelessWidget {
  const FieldLabel(this.text, {super.key});
  final String text;
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Text(
        text,
        style: TextStyle(
          fontSize: 13,
          fontWeight: FontWeight.w500,
          color: appColorsOf(context).foreground,
        ),
      ),
    );
  }
}
