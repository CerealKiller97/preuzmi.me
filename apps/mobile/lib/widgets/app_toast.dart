import 'dart:async';

import 'package:flutter/material.dart';

import '../theme/app_colors.dart';

/// A floating toast that mirrors the web app's `.toast` component: a card with
/// a tinted icon circle, a title, an optional message, and a close button —
/// success (green) / error (red) variants, sliding in from the top.
///
/// Only one toast shows at a time (a new one replaces the current), matching
/// the web's single "flash". Auto-dismisses after 4.5s (success) / 7s (error).
class AppToast {
  AppToast._();

  static _ActiveToast? _active;

  static void success(BuildContext context, String title, {String? message}) =>
      _show(context, success: true, title: title, message: message);

  static void error(BuildContext context, String title, {String? message}) =>
      _show(context, success: false, title: title, message: message);

  static void _show(
    BuildContext context, {
    required bool success,
    required String title,
    String? message,
  }) {
    final overlay = Overlay.maybeOf(context, rootOverlay: true);
    if (overlay == null) {
      return;
    }

    _active?.dismiss();

    final key = GlobalKey<_ToastCardState>();
    late OverlayEntry entry;
    Timer? timer;

    Future<void> remove() async {
      timer?.cancel();
      await key.currentState?.hide();
      if (entry.mounted) {
        entry.remove();
      }

      if (identical(_active?.entry, entry)) {
        _active = null;
      }
    }

    entry = OverlayEntry(
      builder: (_) => _ToastCard(
        key: key,
        success: success,
        title: title,
        message: message,
        onClose: remove,
      ),
    );

    _active = _ActiveToast(entry, remove);
    overlay.insert(entry);

    timer = Timer(Duration(milliseconds: success ? 4500 : 7000), () {
      if (identical(_active?.entry, entry)) {
        remove();
      }
    });
  }
}

class _ActiveToast {
  _ActiveToast(this.entry, this.dismiss);
  final OverlayEntry entry;
  final Future<void> Function() dismiss;
}

class _ToastCard extends StatefulWidget {
  const _ToastCard({
    super.key,
    required this.success,
    required this.title,
    required this.message,
    required this.onClose,
  });

  final bool success;
  final String title;
  final String? message;
  final Future<void> Function() onClose;

  @override
  State<_ToastCard> createState() => _ToastCardState();
}

class _ToastCardState extends State<_ToastCard>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 300),
    reverseDuration: const Duration(milliseconds: 200),
  );

  late final Animation<double> _fade = CurvedAnimation(
    parent: _c,
    curve: Curves.easeOutCubic,
  );
  late final Animation<Offset> _slide = Tween<Offset>(
    begin: const Offset(0, -0.25),
    end: Offset.zero,
  ).animate(CurvedAnimation(parent: _c, curve: Curves.easeOutCubic));

  @override
  void initState() {
    super.initState();
    _c.forward();
  }

  Future<void> hide() async {
    if (!mounted) {
      return;
    }

    await _c.reverse();
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    final accent = widget.success ? c.success : c.destructive;
    final icon = widget.success
        ? Icons.check_rounded
        : Icons.priority_high_rounded;
    final topInset = MediaQuery.of(context).padding.top;

    // Positioned with top+left+right (no height) sizes the child to its
    // intrinsic height — content wraps, nothing forces unbounded expansion.
    return Positioned(
      top: topInset + 8,
      left: 12,
      right: 12,
      child: FadeTransition(
        opacity: _fade,
        child: SlideTransition(
          position: _slide,
          child: Material(
            color: Colors.transparent,
            child: GestureDetector(
              onTap: () => widget.onClose(),
              child: _card(c, accent, icon),
            ),
          ),
        ),
      ),
    );
  }

  Widget _card(AppColors c, Color accent, IconData icon) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 14, 12, 14),
      decoration: BoxDecoration(
        color: c.card,
        borderRadius: BorderRadius.circular(AppColors.radiusXl),
        border: Border.all(color: accent.withValues(alpha: 0.30)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.18),
            blurRadius: 24,
            offset: const Offset(0, 10),
          ),
          // Coloured glow, like the web's success/error toast shadow.
          BoxShadow(
            color: accent.withValues(alpha: 0.28),
            blurRadius: 40,
            spreadRadius: -12,
            offset: const Offset(0, 12),
          ),
        ],
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 36,
            height: 36,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.15),
              shape: BoxShape.circle,
            ),
            alignment: Alignment.center,
            child: Icon(icon, size: 20, color: accent),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Padding(
                  padding: const EdgeInsets.only(top: 2),
                  child: Text(
                    widget.title,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      letterSpacing: -0.2,
                      color: c.foreground,
                    ),
                  ),
                ),
                if (widget.message != null && widget.message!.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  Text(
                    widget.message!,
                    style: TextStyle(
                      fontSize: 13,
                      height: 1.3,
                      color: c.mutedForeground,
                    ),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(width: 8),
          InkWell(
            onTap: () => widget.onClose(),
            borderRadius: BorderRadius.circular(AppColors.radiusMd),
            child: Padding(
              padding: const EdgeInsets.all(6),
              child: Icon(
                Icons.close_rounded,
                size: 18,
                color: c.mutedForeground,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
