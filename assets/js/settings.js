/**
 * Settings page: editable config form with apply/reload, notification test,
 * and step-by-step setup guides for the selected driver.
 */
document.addEventListener('alpine:init', () => {
  Alpine.data('settingsPage', () => ({
    form: {},
    dirty: false,
    saving: false,
    flash: { show: false, ok: false, title: '', message: '', warnings: [] },
    flashTimer: null,

    canTest: false,
    sending: false,
    testOk: false,
    testMessage: '',

    guideOpen: false,
    guideKey: 'smtp',
    step: 0,

    secretHints: {
      appKey: false,
      s3Access: false,
      s3Secret: false,
      smtpPass: false,
      tgToken: false,
      providers: {},
    },

    guides: {
      telegram: {
        title: 'Podešavanje Telegrama',
        subtitle: 'Korak po korak do bot tokena i chat ID-a.',
        steps: [
          {
            title: 'Napravi bota',
            body: 'Otvori Telegram i potraži @BotFather. Pošalji /newbot, izaberi ime i username. BotFather će ti vratiti bot token — sačuvaj ga.',
            tip: 'Token izgleda ovako: 123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw',
          },
          {
            title: 'Pokreni chat sa botom',
            body: 'Otvori chat sa svojim novim botom i pošalji /start (ili bilo koju poruku). Bez ovoga getUpdates neće imati tvoj chat.',
            tip: 'Ako želiš grupu: dodaj bota u grupu i pošalji poruku u grupi.',
          },
          {
            title: 'Uzmi chat ID',
            body: 'U browseru otvori: https://api.telegram.org/bot<TOKEN>/getUpdates — zameni <TOKEN> svojim bot tokenom. U JSON odgovoru nađi chat.id.',
            tip: 'Lični chat ima pozitivan ID. Grupe često imaju negativan ID (npr. -100…).',
          },
          {
            title: 'Upisi u Podešavanja',
            body: 'Postavi drajver na telegram, režim na per_receipt ili all_done, popuni bot token i chat ID, pa klikni „Primeni promene“.',
            tip: 'Posle primene klikni „Pošalji test“ — ne treba restart servera.',
          },
          {
            title: 'Test',
            body: 'Klikni „Pošalji test“. Ako stigne „preuzmi.me: test“, podešeno je kako treba.',
            tip: 'Ako test ne uspe, proveri da li si pisao botu /start i da li su token i chat_id tačni.',
          },
        ],
      },
      smtp: {
        title: 'Podešavanje SMTP emaila',
        subtitle: 'Korak po korak do slanja obaveštenja preko emaila.',
        steps: [
          {
            title: 'Izaberi SMTP provajdera',
            body: 'Koristi email servis koji nudi SMTP: Gmail, Outlook, Mailgun, SES, privatni mail server… Trebaće ti host, port, korisničko ime i lozinka.',
            tip: 'Gmail: smtp.gmail.com, port 587. Za Gmail obično treba App Password, ne obična lozinka.',
          },
          {
            title: 'Pripremi nalog',
            body: 'Uključi SMTP pristup / 2FA + app password ako provajder to traži. Adresa „Od“ (from) mora biti dozvoljena na tom nalogu.',
            tip: 'Kod Gmail-a: Google Account → Security → App passwords.',
          },
          {
            title: 'Upisi u Podešavanja',
            body: 'Postavi drajver na smtp, izaberi režim, popuni SMTP polja i klikni „Primeni promene“.',
            tip: 'Lozinku ostavi praznu ako je već sačuvana — neće se obrisati.',
          },
          {
            title: 'Port i TLS',
            body: 'Port 587 koristi STARTTLS (podrazumevano). Port 465 koristi implicitni TLS (SMTPS). Većina modernih provajdera radi na 587.',
            tip: 'Ako slanje padne na „certificate“ ili timeout, proveri firewall i da li host/port odgovaraju dokumentaciji.',
          },
          {
            title: 'Test',
            body: 'Klikni „Pošalji test“. Proveri inbox (i spam) za poruku „preuzmi.me: test“.',
            tip: 'Test radi i kad je režim off — dovoljno je da SMTP kredencijali budu popunjeni.',
          },
        ],
      },
    },

    init() {
      let initial = {};
      try {
        const raw = document.getElementById('settings-form-data');
        initial = JSON.parse(raw?.textContent || '{}');
      } catch (e) {
        console.error('Failed to parse settings form data', e);
        this.showFlash(false, t('Greška'), t('Ne mogu da učitam podešavanja sa stranice.'));
      }

      this.form = this.normalizeForm(initial);
      this.guides = this.localizeTree(this.guides);

      this.canTest = this.$el.dataset.canTest === 'true';
      const driver = (this.form.notifications?.driver || 'smtp').toLowerCase();
      this.guideKey = this.guides[driver] ? driver : 'smtp';

      this.secretHints.appKey = this.$el.dataset.hasAppKey === 'true';
      this.secretHints.s3Access = this.$el.dataset.hasS3Access === 'true';
      this.secretHints.s3Secret = this.$el.dataset.hasS3Secret === 'true';
      this.secretHints.smtpPass = this.$el.dataset.hasSmtpPass === 'true';
      this.secretHints.tgToken = this.$el.dataset.hasTgToken === 'true';

      try {
        const el = document.getElementById('settings-provider-secrets');
        this.secretHints.providers = el ? JSON.parse(el.textContent || '{}') : {};
      } catch {
        this.secretHints.providers = {};
      }

      this.restoreFlash();

      this.$nextTick(() => {
        this.$watch(
          'form',
          () => {
            this.dirty = true;
          },
          { deep: true },
        );
        this.dirty = false;
      });
    },

    restoreFlash() {
      try {
        const raw = sessionStorage.getItem('settings-flash');
        if (!raw) {
          return;
        }
        sessionStorage.removeItem('settings-flash');
        const saved = JSON.parse(raw);
        this.showFlash(
          !!saved.ok,
          saved.title || (saved.ok ? t('Uspešno') : t('Greška')),
          saved.message || '',
          saved.warnings || [],
        );
      } catch {
        sessionStorage.removeItem('settings-flash');
      }
    },

    localizeTree(value) {
      if (typeof value === 'string') {
        return t(value);
      }
      if (Array.isArray(value)) {
        return value.map((v) => this.localizeTree(v));
      }
      if (value && typeof value === 'object') {
        const out = {};
        for (const [k, v] of Object.entries(value)) {
          out[k] = this.localizeTree(v);
        }
        return out;
      }
      return value;
    },

    persistFlash(ok, title, message, warnings = []) {
      try {
        sessionStorage.setItem(
          'settings-flash',
          JSON.stringify({ ok, title, message, warnings }),
        );
      } catch {
        // sessionStorage may be unavailable; toast still shows before reload.
      }
    },

    showFlash(ok, title, message = '', warnings = []) {
      if (this.flashTimer) {
        window.clearTimeout(this.flashTimer);
        this.flashTimer = null;
      }

      this.flash = {
        show: true,
        ok,
        title,
        message,
        warnings: warnings || [],
      };

      this.flashTimer = window.setTimeout(() => {
        if (this.flash.show) {
          this.flash.show = false;
        }
      }, ok ? 4500 : 7000);
    },

    normalizeForm(input) {
      // Alpine wraps form in a Proxy; structuredClone cannot clone proxies.
      let form = {};
      try {
        form = input && typeof input === 'object'
          ? JSON.parse(JSON.stringify(input))
          : {};
      } catch {
        form = {};
      }

      if (!form.providers) {
        form.providers = {};
      }
      if (!form.notifications) {
        form.notifications = {};
      }
      if (!form.notifications.smtp) {
        form.notifications.smtp = {};
      }
      if (!form.notifications.telegram) {
        form.notifications.telegram = {};
      }
      if (!form.notifications.mode) {
        form.notifications.mode = 'off';
      }
      if (!form.notifications.driver) {
        form.notifications.driver = 'smtp';
      }
      if (!form.s3) {
        form.s3 = {};
      }
      if (!form.application) {
        form.application = {};
      }
      if (!form.application.certs) {
        form.application.certs = {};
      }
      if (form.storage !== 's3') {
        form.storage = 'local';
      }
      if (!form.log_level) {
        form.log_level = 'info';
      }
      if (form.lang !== 'cyrillic' && form.lang !== 'cyrilic') {
        form.lang = 'latin';
      } else {
        form.lang = 'cyrillic';
      }

      return form;
    },

    providerNames() {
      return Object.keys(this.form.providers || {}).sort();
    },

    secretPlaceholder(has) {
      return has ? t('•••••••• (neizmenjeno)') : '';
    },

    markDirty() {
      this.dirty = true;
    },

    async apply() {
      if (this.saving) {
        return;
      }

      this.saving = true;
      this.flash.show = false;

      try {
        const payload = this.normalizeForm(this.form);
        payload.application.port = Number(payload.application.port) || 0;
        payload.check_until = Number(payload.check_until) || 0;
        payload.notifications.smtp.port = Number(payload.notifications.smtp.port) || 0;
        payload.pretty_print = !!payload.pretty_print;

        const res = await fetch('/api/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        const data = await res.json().catch(() => ({}));

        if (!res.ok || !data.ok) {
          throw new Error(data.error || `HTTP ${res.status}`);
        }

        this.dirty = false;

        const title = t('Konfiguracija je uspešno promenjena');
        const message = data.message || t('Izmene su sačuvane i primenjene.');
        const warnings = data.warnings || [];

        this.persistFlash(true, title, message, warnings);
        this.showFlash(true, title, message, warnings);

        // Reload so badges, summary cards, and secret hints match disk.
        window.setTimeout(() => {
          window.location.reload();
        }, 650);
      } catch (e) {
        this.showFlash(false, t('Čuvanje nije uspelo'), e.message || t('Pokušaj ponovo.'));
      } finally {
        this.saving = false;
      }
    },

    get currentGuide() {
      return this.guides[this.guideKey] || this.guides.smtp;
    },

    get currentStep() {
      return this.currentGuide.steps[this.step] || this.currentGuide.steps[0];
    },

    get isFirst() {
      return this.step <= 0;
    },

    get isLast() {
      return this.step >= this.currentGuide.steps.length - 1;
    },

    openGuide(key) {
      const next = (key || this.form.notifications.driver || 'smtp').toLowerCase();
      this.guideKey = this.guides[next] ? next : 'smtp';
      this.step = 0;
      this.guideOpen = true;
      document.documentElement.classList.add('overflow-hidden');
    },

    closeGuide() {
      this.guideOpen = false;
      document.documentElement.classList.remove('overflow-hidden');
    },

    next() {
      if (!this.isLast) {
        this.step += 1;
      }
    },

    prev() {
      if (!this.isFirst) {
        this.step -= 1;
      }
    },

    goTo(index) {
      if (index >= 0 && index < this.currentGuide.steps.length) {
        this.step = index;
      }
    },

    async send() {
      if (!this.canTest || this.sending) {
        return;
      }

      this.sending = true;
      this.testMessage = '';

      try {
        const res = await fetch('/api/notifications/test', { method: 'POST' });
        const text = await res.text();
        if (!res.ok) {
          throw new Error(text.trim() || `HTTP ${res.status}`);
        }
        this.testOk = true;
        this.testMessage = t('Test poruka je poslata.');
      } catch (e) {
        this.testOk = false;
        this.testMessage = e.message || t('Slanje nije uspelo.');
      } finally {
        this.sending = false;
      }
    },
  }));
});
