/**
 * Settings page behaviours: notification test send and step-by-step setup
 * guides for each notification driver (Telegram, SMTP).
 */
document.addEventListener('alpine:init', () => {
  Alpine.data('notifySection', () => ({
    canTest: false,
    sending: false,
    ok: false,
    message: '',

    guideOpen: false,
    guideKey: 'smtp',
    step: 0,

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
            title: 'Upisi u config.json',
            body: 'U notifications sekciji postavi driver na telegram, mode na per_receipt ili all_done, i popuni bot_token i chat_id.',
            code: `"notifications": {\n  "mode": "per_receipt",\n  "driver": "telegram",\n  "telegram": {\n    "bot_token": "TVOJ_TOKEN",\n    "chat_id": "TVOJ_CHAT_ID"\n  }\n}`,
          },
          {
            title: 'Restart i test',
            body: 'Restartuj server da učita config. Vrati se na Podešavanja i klikni „Pošalji test“. Ako stigne „preuzmi.me: test“, podešeno je kako treba.',
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
            title: 'Upisi u config.json',
            body: 'Postavi driver na smtp, izaberi mode (per_receipt ili all_done), i popuni smtp blok.',
            code: `"notifications": {\n  "mode": "per_receipt",\n  "driver": "smtp",\n  "smtp": {\n    "host": "smtp.gmail.com",\n    "port": 587,\n    "username": "ti@gmail.com",\n    "password": "app-password",\n    "from": "ti@gmail.com",\n    "to": "ti@gmail.com"\n  }\n}`,
          },
          {
            title: 'Port i TLS',
            body: 'Port 587 koristi STARTTLS (podrazumevano). Port 465 koristi implicitni TLS (SMTPS). Većina modernih provajdera radi na 587.',
            tip: 'Ako slanje padne na „certificate“ ili timeout, proveri firewall i da li host/port odgovaraju dokumentaciji.',
          },
          {
            title: 'Restart i test',
            body: 'Restartuj server, otvori Podešavanja i klikni „Pošalji test“. Proveri inbox (i spam) za poruku „preuzmi.me: test“.',
            tip: 'Test radi i kad je mode: off — dovoljno je da SMTP kredencijali budu popunjeni.',
          },
        ],
      },
    },

    init() {
      // Injected from the template so the button state matches the server.
      this.canTest = this.$el.dataset.canTest === 'true';
      const driver = (this.$el.dataset.driver || 'smtp').toLowerCase();
      this.guideKey = this.guides[driver] ? driver : 'smtp';
    },

    /**
     * @returns {{title: string, subtitle: string, steps: object[]}}
     */
    get currentGuide() {
      return this.guides[this.guideKey] || this.guides.smtp;
    },

    /**
     * @returns {object}
     */
    get currentStep() {
      return this.currentGuide.steps[this.step] || this.currentGuide.steps[0];
    },

    /**
     * @returns {boolean}
     */
    get isFirst() {
      return this.step <= 0;
    },

    /**
     * @returns {boolean}
     */
    get isLast() {
      return this.step >= this.currentGuide.steps.length - 1;
    },

    /**
     * Opens the setup guide for the currently selected notification driver.
     * @param {string} [key]
     */
    openGuide(key) {
      const next = (key || this.guideKey || 'smtp').toLowerCase();
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

    /**
     * @param {number} index
     */
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
      this.message = '';

      try {
        const res = await fetch('/api/notifications/test', { method: 'POST' });
        const text = await res.text();
        if (!res.ok) {
          throw new Error(text.trim() || `HTTP ${res.status}`);
        }
        this.ok = true;
        this.message = 'Test poruka je poslata.';
      } catch (e) {
        this.ok = false;
        this.message = e.message || 'Slanje nije uspelo.';
      } finally {
        this.sending = false;
      }
    },
  }));
});
