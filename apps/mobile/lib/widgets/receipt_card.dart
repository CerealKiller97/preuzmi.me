import 'package:flutter/material.dart';

import '../api/models.dart';
import '../theme/app_colors.dart';
import '../util/i18n.dart';
import '../util/format.dart' as fmt;
import 'ui.dart';

/// A single receipt card, mirroring the `<article class="card">` in
/// `templates/index.html`: header badges, amount, file meta, paid/due lines,
/// and the QR + view actions.
class ReceiptCard extends StatelessWidget {
  const ReceiptCard({
    super.key,
    required this.receipt,
    required this.onOpenQr,
    required this.onOpenPdf,
  });

  final Receipt receipt;
  final VoidCallback onOpenQr;
  final VoidCallback onOpenPdf;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    final paid = fmt.isPaid(receipt);
    final due = fmt.dueLabel(receipt);
    final showDueBadge = !paid && due.isNotEmpty;
    final overdue = fmt.isOverdue(receipt);
    final verified = fmt.isProviderPaid(receipt);
    final paidLineDate = fmt.formatDate(
      receipt.paidAt > 0 ? receipt.paidAt : receipt.confirmedAt,
    );
    final showPaidLine =
        paid && (receipt.paidAt > 0 || receipt.confirmedAt > 0);

    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header: provider badge + due/paid badges.
          Row(
            children: [
              AppBadge(fmt.receiptLabel(receipt)),
              const Spacer(),
              if (showDueBadge) ...[
                AppBadge(
                  due,
                  variant: overdue
                      ? BadgeVariant.destructive
                      : BadgeVariant.warning,
                ),
                const SizedBox(width: 6),
              ],
              // Provider-confirmed payment sits alongside the paid status here,
              // grouping both status marks at the top. It's a compact icon-only
              // badge (long-press shows the "Verifikovano" tooltip) so it fits
              // next to the paid pill without crowding the provider label.
              if (verified) ...[
                Tooltip(
                  message: 'Verifikovano'.t,
                  child: const AppBadge(
                    '',
                    variant: BadgeVariant.success,
                    icon: Icons.verified_outlined,
                  ),
                ),
                const SizedBox(width: 6),
              ],
              AppBadge(
                paid ? 'Plaćeno' : 'Neplaćeno',
                variant: paid ? BadgeVariant.success : BadgeVariant.warning,
                icon: paid ? Icons.check_rounded : Icons.schedule_rounded,
              ),
            ],
          ),
          const SizedBox(height: 16),
          Text(
            receipt.amount != 0
                ? fmt.formatMoney(receipt.amount, receipt.currency)
                : '—',
            style: TextStyle(
              fontSize: 24,
              fontWeight: FontWeight.w600,
              letterSpacing: -0.5,
              color: c.foreground,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            receipt.period,
            style: TextStyle(fontSize: 14, color: c.mutedForeground),
          ),
          const SizedBox(height: 16),
          // File meta row. The filename group is Expanded so the download date
          // sits flush against the card's right edge — aligned with the status
          // badges above and the action button below — instead of being pushed
          // only halfway by a Spacer that shares flex with the filename.
          Row(
            children: [
              Expanded(
                child: Row(
                  children: [
                    Icon(
                      Icons.description_outlined,
                      size: 14,
                      color: c.mutedForeground,
                    ),
                    const SizedBox(width: 6),
                    Flexible(
                      child: Text(
                        receipt.filename,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          fontSize: 12,
                          color: c.mutedForeground,
                        ),
                      ),
                    ),
                    if (receipt.size > 0)
                      Text(
                        ' · ${fmt.formatSize(receipt.size)}'.t,
                        style: TextStyle(
                          fontSize: 12,
                          color: c.mutedForeground,
                        ),
                      ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              // Download date ("datum preuzimanja"): the small download glyph
              // marks it as when the PDF was fetched, matching the web tooltip.
              Icon(Icons.download_outlined, size: 13, color: c.mutedForeground),
              const SizedBox(width: 4),
              Text(
                fmt.formatDate(
                  receipt.downloadedAt > 0
                      ? receipt.downloadedAt
                      : receipt.modified,
                ),
                style: TextStyle(fontSize: 12, color: c.mutedForeground),
              ),
            ],
          ),
          if (showPaidLine) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                Icon(
                  Icons.event_available_outlined,
                  size: 14,
                  color: c.mutedForeground,
                ),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    'Plaćeno $paidLineDate'.t,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(fontSize: 12, color: c.mutedForeground),
                  ),
                ),
              ],
            ),
          ],
          if (receipt.dueAt > 0 && !paid) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                Icon(
                  Icons.calendar_today_outlined,
                  size: 14,
                  color: overdue ? c.destructive : c.mutedForeground,
                ),
                const SizedBox(width: 6),
                Text(
                  'Dospeće ${fmt.formatDate(receipt.dueAt)}'.t,
                  style: TextStyle(
                    fontSize: 12,
                    color: overdue ? c.destructive : c.mutedForeground,
                  ),
                ),
              ],
            ),
          ],
          const SizedBox(height: 16),
          if (!paid) ...[
            AppButton(
              label: 'Plati skeniranjem (QR)'.t,
              icon: Icons.qr_code_2_rounded,
              variant: ButtonVariant.outline,
              fullWidth: true,
              onPressed: onOpenQr,
            ),
            const SizedBox(height: 8),
          ],
          AppButton(
            label: 'Pogledaj račun'.t,
            variant: ButtonVariant.primary,
            fullWidth: true,
            onPressed: onOpenPdf,
          ),
        ],
      ),
    );
  }
}
