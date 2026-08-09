import 'package:flutter/material.dart';

import '../theme/app_colors.dart';
import '../util/i18n.dart';
import 'ui.dart';

class _Step {
  const _Step(this.title, this.body, this.tip);
  final String title;
  final String body;
  final String tip;
}

class _Guide {
  const _Guide(this.title, this.subtitle, this.steps);
  final String title;
  final String subtitle;
  final List<_Step> steps;
}

/// Setup guides, ported verbatim from `assets/js/settings.js`.
const Map<String, _Guide> _guides = {
  'telegram': _Guide('Podešavanje Telegrama', 'Korak po korak do bot tokena i chat ID-a.', [
    _Step(
      'Napravi bota',
      'Otvori Telegram i potraži @BotFather. Pošalji /newbot, izaberi ime i username. BotFather će ti vratiti bot token — sačuvaj ga.',
      'Token izgleda ovako: 123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw',
    ),
    _Step(
      'Pokreni chat sa botom',
      'Otvori chat sa svojim novim botom i pošalji /start (ili bilo koju poruku). Bez ovoga getUpdates neće imati tvoj chat.',
      'Ako želiš grupu: dodaj bota u grupu i pošalji poruku u grupi.',
    ),
    _Step(
      'Uzmi chat ID',
      'U browseru otvori: https://api.telegram.org/bot<TOKEN>/getUpdates — zameni <TOKEN> svojim bot tokenom. U JSON odgovoru nađi chat.id.',
      'Lični chat ima pozitivan ID. Grupe često imaju negativan ID (npr. -100…).',
    ),
    _Step(
      'Upiši u Podešavanja',
      'Postavi drajver na telegram, režim na per_receipt ili all_done, popuni bot token i chat ID, pa klikni „Primeni promene“.',
      'Posle primene klikni „Pošalji test“ — ne treba restart servera.',
    ),
    _Step(
      'Test',
      'Klikni „Pošalji test“. Ako stigne „preuzmi.me: test“, podešeno je kako treba.',
      'Ako test ne uspe, proveri da li si pisao botu /start i da li su token i chat_id tačni.',
    ),
  ]),
  'smtp': _Guide(
    'Podešavanje SMTP emaila',
    'Korak po korak do slanja obaveštenja preko emaila.',
    [
      _Step(
        'Izaberi SMTP provajdera',
        'Koristi email servis koji nudi SMTP: Gmail, Outlook, Mailgun, SES, privatni mail server… Trebaće ti host, port, korisničko ime i lozinka.',
        'Gmail: smtp.gmail.com, port 587. Za Gmail obično treba App Password, ne obična lozinka.',
      ),
      _Step(
        'Pripremi nalog',
        'Uključi SMTP pristup / 2FA + app password ako provajder to traži. Adresa „Od“ (from) mora biti dozvoljena na tom nalogu.',
        'Kod Gmail-a: Google Account → Security → App passwords.',
      ),
      _Step(
        'Upiši u Podešavanja',
        'Postavi drajver na smtp, izaberi režim, popuni SMTP polja i klikni „Primeni promene“.',
        'Lozinku ostavi praznu ako je već sačuvana — neće se obrisati.',
      ),
      _Step(
        'Port i TLS',
        'Port 587 koristi STARTTLS (podrazumevano). Port 465 koristi implicitni TLS (SMTPS). Većina modernih provajdera radi na 587.',
        'Ako slanje padne na „certificate“ ili timeout, proveri firewall i da li host/port odgovaraju dokumentaciji.',
      ),
      _Step(
        'Test',
        'Klikni „Pošalji test“. Proveri inbox (i spam) za poruku „preuzmi.me: test“.',
        'Test radi i kad je režim off — dovoljno je da SMTP kredencijali budu popunjeni.',
      ),
    ],
  ),
};

/// The step-by-step "Uputstvo" modal, mirroring the web guide dialog.
class GuideDialog extends StatefulWidget {
  const GuideDialog({super.key, required this.guideKey});
  final String guideKey;

  static Future<void> show(BuildContext context, String guideKey) {
    return showDialog<void>(
      context: context,
      barrierColor: Colors.black.withValues(alpha: 0.7),
      builder: (_) => GuideDialog(guideKey: guideKey),
    );
  }

  @override
  State<GuideDialog> createState() => _GuideDialogState();
}

class _GuideDialogState extends State<GuideDialog> {
  int _step = 0;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    final guide = _guides[widget.guideKey] ?? _guides['smtp']!;
    final steps = guide.steps;
    final step = steps[_step];
    final isLast = _step >= steps.length - 1;

    return Dialog(
      backgroundColor: c.card,
      insetPadding: const EdgeInsets.all(16),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppColors.radiusXl),
        side: BorderSide(color: c.border),
      ),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520, maxHeight: 640),
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
                          guide.title.t,
                          style: TextStyle(
                            fontSize: 17,
                            fontWeight: FontWeight.w600,
                            color: c.foreground,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          guide.subtitle.t,
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
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ),
            Divider(height: 1, color: c.border),
            // Progress segments.
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 4),
              child: Row(
                children: [
                  for (var i = 0; i < steps.length; i++) ...[
                    Expanded(
                      child: AnimatedContainer(
                        duration: const Duration(milliseconds: 200),
                        height: 5,
                        decoration: BoxDecoration(
                          color: i <= _step ? c.primary : c.muted,
                          borderRadius: BorderRadius.circular(3),
                        ),
                      ),
                    ),
                    if (i < steps.length - 1) const SizedBox(width: 6),
                  ],
                ],
              ),
            ),
            // Body.
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '${'KORAK'.t} ${_step + 1} / ${steps.length}',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                        letterSpacing: 0.5,
                        color: c.mutedForeground,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      step.title.t,
                      style: TextStyle(
                        fontSize: 20,
                        fontWeight: FontWeight.w600,
                        letterSpacing: -0.3,
                        color: c.foreground,
                      ),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      step.body.t,
                      style: TextStyle(
                        fontSize: 15,
                        height: 1.45,
                        color: c.mutedForeground,
                      ),
                    ),
                    if (step.tip.isNotEmpty) ...[
                      const SizedBox(height: 16),
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.all(14),
                        decoration: BoxDecoration(
                          color: c.muted.withValues(alpha: 0.5),
                          borderRadius: BorderRadius.circular(
                            AppColors.radiusMd,
                          ),
                          border: Border.all(color: c.border),
                        ),
                        child: RichText(
                          text: TextSpan(
                            style: TextStyle(
                              fontSize: 13,
                              height: 1.4,
                              color: c.mutedForeground,
                            ),
                            children: [
                              TextSpan(
                                text: '${'Savet:'.t} ',
                                style: TextStyle(
                                  fontWeight: FontWeight.w600,
                                  color: c.foreground,
                                ),
                              ),
                              TextSpan(text: step.tip.t),
                            ],
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            Divider(height: 1, color: c.border),
            // Footer.
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  AppButton(
                    label: 'Nazad'.t,
                    variant: ButtonVariant.outline,
                    onPressed: _step == 0
                        ? null
                        : () => setState(() => _step--),
                  ),
                  const Spacer(),
                  AppButton(
                    label: isLast ? 'Završi'.t : 'Dalje'.t,
                    onPressed: isLast
                        ? () => Navigator.of(context).pop()
                        : () => setState(() => _step++),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
