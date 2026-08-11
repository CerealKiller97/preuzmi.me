/**
 * Serbian Latin → Cyrillic for UI strings built in JS.
 * Reads <html data-lang="latin|cyrillic">. Use t('Račun') for user-facing text.
 */
(function () {
  const digraphs = [
    ['Dž', 'Џ'], ['DŽ', 'Џ'], ['dž', 'џ'],
    ['Lj', 'Љ'], ['LJ', 'Љ'], ['lj', 'љ'],
    ['Nj', 'Њ'], ['NJ', 'Њ'], ['nj', 'њ'],
  ];

  const single = {
    A: 'А', a: 'а', B: 'Б', b: 'б', V: 'В', v: 'в', G: 'Г', g: 'г',
    D: 'Д', d: 'д', Đ: 'Ђ', đ: 'ђ', E: 'Е', e: 'е', Ž: 'Ж', ž: 'ж',
    Z: 'З', z: 'з', I: 'И', i: 'и', J: 'Ј', j: 'ј', K: 'К', k: 'к',
    L: 'Л', l: 'л', M: 'М', m: 'м', N: 'Н', n: 'н', O: 'О', o: 'о',
    P: 'П', p: 'п', R: 'Р', r: 'р', S: 'С', s: 'с', T: 'Т', t: 'т',
    Ć: 'Ћ', ć: 'ћ', U: 'У', u: 'у', F: 'Ф', f: 'ф', H: 'Х', h: 'х',
    C: 'Ц', c: 'ц', Č: 'Ч', č: 'ч', Š: 'Ш', š: 'ш',
  };

  function toCyrillic(s) {
    if (!s) return s;
    const urls = [];
    s = String(s).replace(/https?:\/\/[^\s<>"']+/g, (u) => {
      const token = String.fromCharCode(0xE010 + urls.length);
      urls.push({ token, value: u });
      return token;
    });
    // Technical literals \u2014 commands, handles, config keys and acronyms \u2014 that
    // are typed/searched verbatim in the real tool, so a Cyrillic transliteration
    // would be unusable. Word boundaries keep standalone terms Latin ("bot token",
    // "chat") while inflected Serbian forms ("botom", "tokenom") still convert.
    const literals = [];
    s = s.replace(/@BotFather|\bBotFather\b|\bgetUpdates\b|\/newbot|\/start|\bchat ID\b|\bchatID\b|\bchat_id\b|\bchat\.id\b|\bbot_token\b|<TOKEN>|\bper_receipt\b|\ball_done\b|\busername\b|\bSTARTTLS\b|\bSMTPS\b|\bSMTP\b|\bTLS\b|\bJSON\b|\btelegram\b|\bsmtp\b|\bchat\b/g, (m) => {
      const token = String.fromCharCode(0xE030 + literals.length);
      literals.push({ token, value: m });
      return token;
    });
    const brands = [
      ['Preuzmi.me', '\uE000'],
      ['preuzmi.me', '\uE001'],
      ['config.json', '\uE002'],
    ];
    for (const [from, token] of brands) {
      s = s.split(from).join(token);
    }
    let out = '';
    let i = 0;
    while (i < s.length) {
      let matched = false;
      for (const [from, to] of digraphs) {
        if (s.startsWith(from, i)) {
          out += to;
          i += from.length;
          matched = true;
          break;
        }
      }
      if (matched) continue;
      const ch = s[i];
      out += Object.prototype.hasOwnProperty.call(single, ch) ? single[ch] : ch;
      i += 1;
    }
    for (const [from, token] of brands) {
      out = out.split(token).join(from);
    }
    for (const u of urls) {
      out = out.split(u.token).join(u.value);
    }
    for (const l of literals) {
      out = out.split(l.token).join(l.value);
    }
    return out;
  }

  function lang() {
    return (document.documentElement.dataset.lang || 'latin').toLowerCase();
  }

  window.toCyrillic = toCyrillic;
  window.t = function t(s) {
    return lang() === 'cyrillic' || lang() === 'cyrilic' ? toCyrillic(s) : s;
  };
  window.srLocale = function srLocale() {
    return lang() === 'cyrillic' || lang() === 'cyrilic' ? 'sr-Cyrl-RS' : 'sr-Latn-RS';
  };

  // providerNames overrides the Latin display name for provider keys whose
  // uppercased form would read wrong (the Serbian-word providers). providerCyrillic
  // pins the Cyrillic form for names that do not simply transliterate — a foreign
  // brand kept Latin (A1) or a custom spelling (Yettel → ЈЕТЕЛ). Kept in sync with
  // the notify package so the UI names providers the same way as notifications.
  const providerNames = {
    esanduce: 'E-SANDUČE',
    eupravnik: 'E-UPRAVNIK',
  };
  const providerCyrillic = {
    a1: 'A1',
    yettel: 'ЈЕТЕЛ',
  };

  // providerLabel returns the display name for a provider key. In Cyrillic mode a
  // pinned form wins; otherwise the Latin name (override or uppercased key)
  // transliterates — so Serbian acronyms like MTS/EPS become МТС/ЕПС.
  window.providerLabel = function providerLabel(key) {
    const k = String(key || '').toLowerCase();
    const latin = providerNames[k] || String(key || '').toUpperCase();
    if ((lang() === 'cyrillic' || lang() === 'cyrilic') && providerCyrillic[k]) {
      return providerCyrillic[k];
    }
    return window.t(latin);
  };
})();
