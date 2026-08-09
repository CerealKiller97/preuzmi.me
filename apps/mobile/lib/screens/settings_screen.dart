import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/models.dart';
import '../state/app_state.dart';
import '../theme/app_colors.dart';
import '../util/i18n.dart';
import '../widgets/app_toast.dart';
import '../widgets/guide_dialog.dart';
import '../widgets/ui.dart';

/// The Settings screen — mirrors `templates/settings.html`: status cards, the
/// editable config form (storage, email, notifications, providers) and the
/// apply action, plus a mobile-only server-URL + theme card.
class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = appColorsOf(context);
    I18n.cyrillic = ref.watch(appConfigProvider.select((x) => x.cyrillic));
    final settingsAsync = ref.watch(settingsProvider);

    return Scaffold(
      appBar: AppBar(title: Text('Podešavanja'.t)),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 40),
        children: [
          const _AppSettingsCard(),
          const SizedBox(height: 16),
          settingsAsync.when(
            loading: () => const Padding(
              padding: EdgeInsets.symmetric(vertical: 60),
              child: Center(child: CircularProgressIndicator()),
            ),
            error: (e, _) => _serverError(c, ref, e),
            // Keyed so a refetch after Apply rebuilds the editor with fresh
            // values (and blanked secrets) from the server.
            data: (s) => _SettingsEditor(
              key: ValueKey('${s.version}:${s.receiptCount}:${s.storage}'),
              settings: s,
            ),
          ),
        ],
      ),
    );
  }

  Widget _serverError(AppColors c, WidgetRef ref, Object e) {
    return AppCard(
      child: Column(
        children: [
          Icon(Icons.cloud_off_rounded, size: 36, color: c.mutedForeground),
          const SizedBox(height: 12),
          Text(
            'Ne mogu da učitam podešavanja'.t,
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: c.foreground,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            'Proverite adresu servera iznad.\n$e'.t,
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 13, color: c.mutedForeground),
          ),
          const SizedBox(height: 16),
          AppButton(
            label: 'Pokušaj ponovo'.t,
            variant: ButtonVariant.outline,
            onPressed: () => ref.invalidate(settingsProvider),
          ),
        ],
      ),
    );
  }
}

/// Mobile-only card: which server the app talks to, plus the theme mode.
class _AppSettingsCard extends ConsumerStatefulWidget {
  const _AppSettingsCard();
  @override
  ConsumerState<_AppSettingsCard> createState() => _AppSettingsCardState();
}

class _AppSettingsCardState extends ConsumerState<_AppSettingsCard> {
  late final TextEditingController _url;

  @override
  void initState() {
    super.initState();
    _url = TextEditingController(text: ref.read(appConfigProvider).baseUrl);
  }

  @override
  void dispose() {
    _url.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final cfg = ref.watch(appConfigProvider);
    return SectionCard(
      title: 'Aplikacija'.t,
      subtitle: 'Adresa servera sa kojim se aplikacija povezuje.'.t,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          FieldLabel('Adresa servera'.t),
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _url,
                  keyboardType: TextInputType.url,
                  autocorrect: false,
                  decoration: InputDecoration(
                    hintText: 'http://localhost:5500'.t,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              AppButton(
                label: 'Sačuvaj'.t,
                onPressed: () {
                  ref.read(appConfigProvider.notifier).setBaseUrl(_url.text);
                  ref.invalidate(receiptsProvider);
                  ref.invalidate(statsProvider);
                  ref.invalidate(providersProvider);
                  ref.invalidate(settingsProvider);
                  AppToast.success(
                    context,
                    'Uspešno',
                    message: 'Adresa servera je sačuvana.',
                  );
                },
              ),
            ],
          ),
          const SizedBox(height: 16),
          FieldLabel('Tema'.t),
          _ThemeSwitcher(
            current: cfg.themeMode,
            onChanged: (m) =>
                ref.read(appConfigProvider.notifier).setThemeMode(m),
          ),
          const SizedBox(height: 16),
          FieldLabel('Pismo'.t),
          _Segmented(
            selected: cfg.cyrillic ? 1 : 0,
            options: [
              (Icons.abc_rounded, 'Latinica'.t),
              (Icons.translate_rounded, 'Ćirilica'.t),
            ],
            onChanged: (i) =>
                ref.read(appConfigProvider.notifier).setCyrillic(i == 1),
          ),
        ],
      ),
    );
  }
}

/// A generic segmented control styled like the app's chart toggle: a muted
/// track with a raised, icon-labelled active segment.
class _Segmented extends StatelessWidget {
  const _Segmented({
    required this.selected,
    required this.options,
    required this.onChanged,
  });

  final int selected;
  final List<(IconData, String)> options;
  final ValueChanged<int> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    return Container(
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        color: c.muted,
        borderRadius: BorderRadius.circular(AppColors.radiusMd),
      ),
      child: Row(
        children: [
          for (var i = 0; i < options.length; i++)
            Expanded(
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => onChanged(i),
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 200),
                  curve: Curves.easeOutCubic,
                  padding: const EdgeInsets.symmetric(vertical: 9),
                  decoration: BoxDecoration(
                    color: selected == i ? c.background : Colors.transparent,
                    borderRadius: BorderRadius.circular(AppColors.radiusSm),
                    boxShadow: selected == i
                        ? [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.08),
                              blurRadius: 2,
                              offset: const Offset(0, 1),
                            ),
                          ]
                        : null,
                  ),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(
                        options[i].$1,
                        size: 16,
                        color: selected == i ? c.foreground : c.mutedForeground,
                      ),
                      const SizedBox(width: 6),
                      Text(
                        options[i].$2,
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: selected == i
                              ? c.foreground
                              : c.mutedForeground,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// A segmented theme selector styled like the app's chart toggle: a muted track
/// with a raised, icon-labelled active segment that slides between options.
class _ThemeSwitcher extends StatelessWidget {
  const _ThemeSwitcher({required this.current, required this.onChanged});

  final ThemeMode current;
  final ValueChanged<ThemeMode> onChanged;

  static const _options = [
    (ThemeMode.system, Icons.brightness_auto_rounded, 'Sistem'),
    (ThemeMode.light, Icons.light_mode_rounded, 'Svetla'),
    (ThemeMode.dark, Icons.dark_mode_rounded, 'Tamna'),
  ];

  @override
  Widget build(BuildContext context) {
    final c = appColorsOf(context);
    return Container(
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        color: c.muted,
        borderRadius: BorderRadius.circular(AppColors.radiusMd),
      ),
      child: Row(
        children: [
          for (final (mode, icon, label) in _options)
            Expanded(
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => onChanged(mode),
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 200),
                  curve: Curves.easeOutCubic,
                  padding: const EdgeInsets.symmetric(vertical: 9),
                  decoration: BoxDecoration(
                    color: current == mode ? c.background : Colors.transparent,
                    borderRadius: BorderRadius.circular(AppColors.radiusSm),
                    boxShadow: current == mode
                        ? [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.08),
                              blurRadius: 2,
                              offset: const Offset(0, 1),
                            ),
                          ]
                        : null,
                  ),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(
                        icon,
                        size: 16,
                        color: current == mode
                            ? c.foreground
                            : c.mutedForeground,
                      ),
                      const SizedBox(width: 6),
                      Text(
                        label,
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: current == mode
                              ? c.foreground
                              : c.mutedForeground,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// The server-config editor. Holds a mutable deep copy of `settings.form` and
/// syncs field controllers into it on Apply, then PUTs the whole map (so any
/// field the UI does not render round-trips untouched).
class _SettingsEditor extends ConsumerStatefulWidget {
  const _SettingsEditor({super.key, required this.settings});
  final Settings settings;

  @override
  ConsumerState<_SettingsEditor> createState() => _SettingsEditorState();
}

class _SettingsEditorState extends ConsumerState<_SettingsEditor> {
  late Map<String, dynamic> _form;
  final Map<String, TextEditingController> _ctl = {};
  bool _saving = false;

  // Non-text form state.
  late String _storage;
  late String _lang;
  late String _emailProvider;
  late String _notifyMode;
  late String _notifyDriver;
  late bool _paidConfirmation;
  late bool _dueReminders;

  Map<String, dynamic> _sub(String key) =>
      (_form[key] as Map?)?.cast<String, dynamic>() ?? {};

  @override
  void initState() {
    super.initState();
    _form = _deepCopy(widget.settings.form);
    final n = _sub('notifications');
    final smtp = (n['smtp'] as Map?)?.cast<String, dynamic>() ?? {};
    final tg = (n['telegram'] as Map?)?.cast<String, dynamic>() ?? {};
    final s3 = _sub('s3');
    final email = _sub('email');

    _storage = (_form['storage'] ?? 'local').toString();
    _lang = (_form['lang'] ?? 'latin').toString();
    _emailProvider = (email['provider'] ?? 'gmail').toString();
    _notifyMode = (n['mode'] ?? 'off').toString();
    _notifyDriver = (n['driver'] ?? 'telegram').toString();
    _paidConfirmation = n['paid_confirmation'] == true;
    _dueReminders = n['due_reminders'] == true;

    _mk('check_until', (_form['check_until'] ?? 20).toString());
    _mk('download_path', (_form['download_path'] ?? '').toString());
    _mk('s3.endpoint', (s3['endpoint'] ?? '').toString());
    _mk('s3.bucket', (s3['bucket'] ?? '').toString());
    _mk('s3.region', (s3['region'] ?? '').toString());
    _mk('s3.access_key', '');
    _mk('s3.secret_key', '');
    _mk('email.mailbox', (email['mailbox'] ?? '').toString());
    _mk('email.host', (email['host'] ?? '').toString());
    _mk('email.port', (email['port'] ?? 0).toString());
    _mk('n.due_days', (n['due_reminder_days'] ?? 7).toString());
    _mk('tg.bot_token', '');
    _mk('tg.chat_id', (tg['chat_id'] ?? '').toString());
    _mk('smtp.host', (smtp['host'] ?? '').toString());
    _mk('smtp.port', (smtp['port'] ?? 0).toString());
    _mk('smtp.username', (smtp['username'] ?? '').toString());
    _mk('smtp.password', '');
    _mk('smtp.from', (smtp['from'] ?? '').toString());
    _mk('smtp.to', (smtp['to'] ?? '').toString());

    for (final p in _providerNames()) {
      final creds = _providers()[p] as Map? ?? {};
      _mk('prov.$p.identifier', (creds['identifier'] ?? '').toString());
      _mk('prov.$p.password', '');
    }
  }

  void _mk(String key, String value) =>
      _ctl[key] = TextEditingController(text: value);

  @override
  void dispose() {
    for (final c in _ctl.values) {
      c.dispose();
    }
    super.dispose();
  }

  Map<String, dynamic> _providers() =>
      (_form['providers'] as Map?)?.cast<String, dynamic>() ?? {};

  List<String> _providerNames() {
    final names = _providers().keys.toList()..sort();
    return names;
  }

  int _int(String key, int fallback) =>
      int.tryParse(_ctl[key]!.text.trim()) ?? fallback;

  /// Rebuilds the form map from controllers/state, then PUTs it.
  Future<void> _apply() async {
    setState(() => _saving = true);
    final form = _deepCopy(_form);
    form['storage'] = _storage;
    form['lang'] = _lang;
    form['check_until'] = _int('check_until', 20);
    form['download_path'] = _ctl['download_path']!.text.trim();

    final s3 = <String, dynamic>{...?form['s3'] as Map?};
    s3['endpoint'] = _ctl['s3.endpoint']!.text.trim();
    s3['bucket'] = _ctl['s3.bucket']!.text.trim();
    s3['region'] = _ctl['s3.region']!.text.trim();
    s3['access_key'] = _ctl['s3.access_key']!.text; // blank keeps existing
    s3['secret_key'] = _ctl['s3.secret_key']!.text;
    form['s3'] = s3;

    final email = <String, dynamic>{...?form['email'] as Map?};
    email['provider'] = _emailProvider;
    email['mailbox'] = _ctl['email.mailbox']!.text.trim();
    email['host'] = _ctl['email.host']!.text.trim();
    email['port'] = _int('email.port', 0);
    form['email'] = email;

    final n = <String, dynamic>{...?form['notifications'] as Map?};
    n['mode'] = _notifyMode;
    n['driver'] = _notifyDriver;
    n['paid_confirmation'] = _paidConfirmation;
    n['due_reminders'] = _dueReminders;
    n['due_reminder_days'] = _int('n.due_days', 7);
    final tg = <String, dynamic>{...?n['telegram'] as Map?};
    tg['bot_token'] = _ctl['tg.bot_token']!.text;
    tg['chat_id'] = _ctl['tg.chat_id']!.text.trim();
    n['telegram'] = tg;
    final smtp = <String, dynamic>{...?n['smtp'] as Map?};
    smtp['host'] = _ctl['smtp.host']!.text.trim();
    smtp['port'] = _int('smtp.port', 0);
    smtp['username'] = _ctl['smtp.username']!.text.trim();
    smtp['password'] = _ctl['smtp.password']!.text;
    smtp['from'] = _ctl['smtp.from']!.text.trim();
    smtp['to'] = _ctl['smtp.to']!.text.trim();
    n['smtp'] = smtp;
    form['notifications'] = n;

    final provs = <String, dynamic>{...?form['providers'] as Map?};
    for (final p in _providerNames()) {
      final creds = <String, dynamic>{...?provs[p] as Map?};
      creds['identifier'] = _ctl['prov.$p.identifier']!.text.trim();
      creds['password'] = _ctl['prov.$p.password']!.text; // blank keeps
      provs[p] = creds;
    }
    form['providers'] = provs;

    try {
      final res = await ref.read(apiClientProvider).updateSettings(form);
      if (!mounted) return;
      if (res.ok) {
        _snack(res.message.isEmpty ? 'Izmene su sačuvane.' : res.message);
        ref.invalidate(settingsProvider);
        ref.invalidate(receiptsProvider);
        ref.invalidate(statsProvider);
        ref.invalidate(providersProvider);
      } else {
        _snack(
          res.error.isEmpty ? 'Čuvanje nije uspelo.' : res.error,
          error: true,
        );
      }
    } catch (e) {
      if (mounted) _snack('Greška: $e', error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _sendTest() async {
    final res = await ref.read(apiClientProvider).testNotification();
    if (!mounted) return;
    _snack(
      res.ok ? (res.message.isEmpty ? 'Poslato.' : res.message) : res.error,
      error: !res.ok,
    );
  }

  void _snack(String msg, {bool error = false}) {
    if (error) {
      AppToast.error(context, 'Greška', message: msg);
    } else {
      AppToast.success(context, 'Uspešno', message: msg);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.settings;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _statusCards(s),
        const SizedBox(height: 16),
        _storageSection(),
        const SizedBox(height: 16),
        _emailSection(),
        const SizedBox(height: 16),
        _notificationsSection(s),
        const SizedBox(height: 16),
        _providersSection(),
        const SizedBox(height: 16),
        AppButton(
          label: 'Primeni promene'.t,
          fullWidth: true,
          loading: _saving,
          onPressed: _apply,
        ),
      ],
    );
  }

  Widget _statusCards(Settings s) {
    final c = appColorsOf(context);
    Widget card(String label, Widget value) => Expanded(
      child: AppCard(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              label.t,
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w500,
                color: c.mutedForeground,
              ),
            ),
            const SizedBox(height: 8),
            value,
          ],
        ),
      ),
    );
    Widget big(String t) => Text(
      t,
      style: TextStyle(
        fontSize: 20,
        fontWeight: FontWeight.w600,
        color: c.foreground,
      ),
    );

    return Column(
      children: [
        Row(
          children: [
            card('Verzija', big(s.version.isEmpty ? '—' : s.version)),
            const SizedBox(width: 12),
            card('Broj računa', big('${s.receiptCount}')),
          ],
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            card(
              'Skladište računa',
              Align(
                alignment: Alignment.centerLeft,
                child: s.storage == 's3'
                    ? AppBadge('S3'.t, variant: BadgeVariant.success)
                    : AppBadge('Lokalno'.t),
              ),
            ),
            const SizedBox(width: 12),
            card(
              'Folder',
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  s.downloadPathExists
                      ? AppBadge('Postoji'.t, variant: BadgeVariant.success)
                      : AppBadge(
                          'Ne postoji'.t,
                          variant: BadgeVariant.destructive,
                        ),
                  s.downloadPathWritable
                      ? AppBadge('Upisiv'.t, variant: BadgeVariant.success)
                      : AppBadge(
                          'Nije upisiv'.t,
                          variant: BadgeVariant.warning,
                        ),
                ],
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _storageSection() {
    return SectionCard(
      title: 'Skladište'.t,
      subtitle: 'Gde se čuvaju računi i do kog dana u mesecu se osvežava.'.t,
      child: Column(
        children: [
          _dropdown('Tip skladišta'.t, _storage, const {
            'local': 'Lokalno',
            's3': 'S3',
          }, (v) => setState(() => _storage = v)),
          _numField('Osvežavanje do dana'.t, 'check_until'),
          _dropdown('Pismo'.t, _lang, const {
            'latin': 'Latinica',
            'cyrillic': 'Ćirilica',
          }, (v) => setState(() => _lang = v)),
          _textField('Folder za preuzimanje'.t, 'download_path', mono: true),
          if (_storage == 's3') ...[
            const SizedBox(height: 4),
            _textField('S3 Endpoint'.t, 's3.endpoint', mono: true),
            _textField('S3 Bucket'.t, 's3.bucket'),
            _textField('S3 Region'.t, 's3.region'),
            _secretField(
              'S3 Access key'.t,
              's3.access_key',
              widget.settings.hasS3Access,
            ),
            _secretField(
              'S3 Secret key'.t,
              's3.secret_key',
              widget.settings.hasS3Secret,
            ),
          ],
        ],
      ),
    );
  }

  Widget _emailSection() {
    return SectionCard(
      title: 'Email (IMAP)'.t,
      subtitle: 'Server za čitanje pošte za provajdere bez API-ja.'.t,
      child: Column(
        children: [
          _dropdown('Provajder'.t, _emailProvider, const {
            'gmail': 'gmail',
            'outlook': 'outlook',
            'yahoo': 'yahoo',
            'icloud': 'icloud',
            'custom': 'custom',
          }, (v) => setState(() => _emailProvider = v)),
          _textField('Folder'.t, 'email.mailbox'),
          if (_emailProvider == 'custom') ...[
            _textField('Host'.t, 'email.host', mono: true),
            _numField('Port'.t, 'email.port'),
          ],
        ],
      ),
    );
  }

  Widget _notificationsSection(Settings s) {
    return SectionCard(
      title: 'Obaveštenja'.t,
      subtitle: 'Kada i kako da se pošalje poruka posle preuzimanja računa.'.t,
      trailing: AppButton(
        label: 'Pošalji test'.t,
        variant: ButtonVariant.outline,
        onPressed: s.notifyCanTest ? _sendTest : null,
      ),
      child: Column(
        children: [
          _dropdown('Režim'.t, _notifyMode, const {
            'off': 'off',
            'per_receipt': 'per_receipt',
            'all_done': 'all_done',
          }, (v) => setState(() => _notifyMode = v)),
          _dropdown('Drajver'.t, _notifyDriver, const {
            'smtp': 'smtp',
            'telegram': 'telegram',
          }, (v) => setState(() => _notifyDriver = v)),
          Align(
            alignment: Alignment.centerLeft,
            child: AppButton(
              label: 'Uputstvo'.t,
              icon: Icons.help_outline_rounded,
              variant: ButtonVariant.outline,
              onPressed: () => GuideDialog.show(context, _notifyDriver),
            ),
          ),
          const SizedBox(height: 12),
          _checkbox(
            'Obavesti kada je račun potvrđen kao plaćen'.t,
            _paidConfirmation,
            (v) => setState(() => _paidConfirmation = v),
          ),
          _checkbox(
            'Obavesti o računima koji dospevaju'.t,
            _dueReminders,
            (v) => setState(() => _dueReminders = v),
          ),
          if (_dueReminders)
            _numField('Koliko dana unapred podsetiti'.t, 'n.due_days'),
          if (_notifyDriver == 'telegram') ...[
            const SizedBox(height: 4),
            _secretField('Telegram Bot token'.t, 'tg.bot_token', s.hasTgToken),
            _textField('Telegram Chat ID'.t, 'tg.chat_id', mono: true),
          ] else ...[
            const SizedBox(height: 4),
            _textField('SMTP Host'.t, 'smtp.host'),
            _numField('SMTP Port'.t, 'smtp.port'),
            _textField('Korisničko ime'.t, 'smtp.username'),
            _secretField('Lozinka'.t, 'smtp.password', s.hasSmtpPass),
            _textField('Od'.t, 'smtp.from'),
            _textField('Za'.t, 'smtp.to'),
          ],
        ],
      ),
    );
  }

  Widget _providersSection() {
    final c = appColorsOf(context);
    return SectionCard(
      title: 'Provajderi'.t,
      subtitle:
          'Preuzimaju se samo provajderi sa popunjena oba polja i implementacijom.',
      child: Column(
        children: [
          for (final p in _providerNames())
            Container(
              margin: const EdgeInsets.only(bottom: 12),
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                border: Border.all(color: c.border),
                borderRadius: BorderRadius.circular(AppColors.radiusMd),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    p.toUpperCase(),
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                      color: c.foreground,
                    ),
                  ),
                  const SizedBox(height: 10),
                  _textField('Nalog'.t, 'prov.$p.identifier', mono: true),
                  _secretField(
                    'Lozinka'.t,
                    'prov.$p.password',
                    _providerHasPassword(p),
                  ),
                ],
              ),
            ),
        ],
      ),
    );
  }

  bool _providerHasPassword(String name) {
    for (final ps in widget.settings.providers) {
      if (ps.name == name) return ps.hasPassword;
    }
    return false;
  }

  // --- field builders -------------------------------------------------------

  Widget _textField(String label, String key, {bool mono = false}) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          FieldLabel(label),
          TextField(
            controller: _ctl[key],
            autocorrect: false,
            enableSuggestions: false,
            style: mono
                ? const TextStyle(fontFamily: 'monospace', fontSize: 13)
                : null,
          ),
        ],
      ),
    );
  }

  Widget _numField(String label, String key) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          FieldLabel(label),
          TextField(controller: _ctl[key], keyboardType: TextInputType.number),
        ],
      ),
    );
  }

  Widget _secretField(String label, String key, bool hasValue) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          FieldLabel(label),
          TextField(
            controller: _ctl[key],
            obscureText: true,
            autocorrect: false,
            enableSuggestions: false,
            style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
            decoration: InputDecoration(
              hintText: hasValue ? '•••••••• (sačuvano)' : '—',
            ),
          ),
        ],
      ),
    );
  }

  Widget _dropdown(
    String label,
    String value,
    Map<String, String> options,
    ValueChanged<String> onChanged,
  ) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          FieldLabel(label),
          DropdownButtonFormField<String>(
            initialValue: options.containsKey(value)
                ? value
                : options.keys.first,
            isExpanded: true,
            items: [
              for (final e in options.entries)
                DropdownMenuItem(value: e.key, child: Text(e.value)),
            ],
            onChanged: (v) {
              if (v != null) onChanged(v);
            },
          ),
        ],
      ),
    );
  }

  Widget _checkbox(String label, bool value, ValueChanged<bool> onChanged) {
    final c = appColorsOf(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: InkWell(
        onTap: () => onChanged(!value),
        borderRadius: BorderRadius.circular(AppColors.radiusSm),
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: Row(
            children: [
              SizedBox(
                width: 22,
                height: 22,
                child: Checkbox(
                  value: value,
                  onChanged: (v) => onChanged(v ?? false),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  label,
                  style: TextStyle(fontSize: 14, color: c.foreground),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Deep-copies a JSON-shaped map so edits never mutate the provider's data.
Map<String, dynamic> _deepCopy(Map<String, dynamic> src) {
  final out = <String, dynamic>{};
  src.forEach((k, v) {
    out[k] = _copyValue(v);
  });
  return out;
}

dynamic _copyValue(dynamic v) {
  if (v is Map) {
    return v.map((k, val) => MapEntry(k.toString(), _copyValue(val)));
  }
  if (v is List) {
    return v.map(_copyValue).toList();
  }
  return v;
}
