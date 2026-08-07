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
    const brands = [
      ['Preuzmi.me', '\uE000'],
      ['preuzmi.me', '\uE001'],
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

  // baseProvider maps an account key to its base provider type: the key itself
  // when it is a known provider ("a1"), otherwise the part before the first "-"
  // ("a1-mama" → "a1"). Mirrors config.BaseProvider on the server.
  const knownProviders = { mts: 1, a1: 1, yettel: 1, eps: 1, esanduce: 1, eupravnik: 1 };
  function baseProvider(key) {
    const k = String(key || '').toLowerCase();
    if (knownProviders[k]) return k;
    const dash = k.indexOf('-');
    return dash === -1 ? k : k.slice(0, dash);
  }
  window.baseProvider = baseProvider;

  // accountLabels maps an account key to its family-member label. Populated from
  // /api/providers via setProviderAccounts, so receipt/stats data (which carry a
  // bare account key) can still render the label. Refresh results carry their own
  // label and pass it to providerLabel directly.
  const accountLabels = {};
  window.setProviderAccounts = function setProviderAccounts(list) {
    if (!Array.isArray(list)) return;
    for (const a of list) {
      if (a && a.key) accountLabels[a.key] = a.label || '';
    }
  };

  // providerBrand returns the brand display name for an account key, resolved
  // from its base provider. In Cyrillic mode a pinned form wins; otherwise the
  // Latin name transliterates — so Serbian acronyms like MTS/EPS become МТС/ЕПС.
  function providerBrand(key) {
    const base = baseProvider(key);
    const latin = providerNames[base] || base.toUpperCase();
    if ((lang() === 'cyrillic' || lang() === 'cyrilic') && providerCyrillic[base]) {
      return providerCyrillic[base];
    }
    return window.t(latin);
  }

  // providerLabel returns the display name for an account. The brand comes from
  // the base provider; when the account has a family-member label it is appended
  // ("A1 — Mama"). label may be passed explicitly (refresh results); otherwise it
  // is looked up from the accounts fetched via setProviderAccounts.
  window.providerLabel = function providerLabel(key, label) {
    const brand = providerBrand(key);
    const l = (label === undefined || label === null ? accountLabels[key] : label) || '';
    return l ? `${brand} — ${l}` : brand;
  };
})();
