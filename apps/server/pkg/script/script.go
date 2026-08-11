// Package script transliterates Serbian Latin text to Cyrillic.
//
// Source strings in the app stay in Latin; call Apply when lang is cyrillic.
// Apply is used for notifications and for variables passed into HTML views —
// never for rewriting rendered markup.
package script

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Allowed config.json "lang" values.
const (
	LangLatin    = "latin"
	LangCyrillic = "cyrillic"
)

// Normalize returns a canonical lang value. Empty defaults to latin.
// "cyrilic" (common misspelling) is accepted as cyrillic.
func Normalize(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "", LangLatin:
		return LangLatin
	case LangCyrillic, "cyrilic":
		return LangCyrillic
	default:
		return strings.ToLower(strings.TrimSpace(lang))
	}
}

// IsCyrillic reports whether lang selects the Cyrillic script.
func IsCyrillic(lang string) bool {
	return Normalize(lang) == LangCyrillic
}

// HTMLLang is the BCP 47 language tag for <html lang="...">.
func HTMLLang(lang string) string {
	if IsCyrillic(lang) {
		return "sr-Cyrl"
	}
	return "sr-Latn"
}

// Locale is the Intl locale (sr-Latn-RS / sr-Cyrl-RS).
func Locale(lang string) string {
	if IsCyrillic(lang) {
		return "sr-Cyrl-RS"
	}
	return "sr-Latn-RS"
}

// Apply returns s unchanged for latin, or the Cyrillic form for cyrillic.
func Apply(lang, s string) string {
	if !IsCyrillic(lang) || s == "" {
		return s
	}
	return ToCyrillic(s)
}

// ApplyReplacing is Apply, but each key found in s is swapped for its mapped
// value verbatim instead of being transliterated. Used for provider names whose
// Cyrillic form is fixed — a foreign brand kept Latin (A1 → A1) or a custom
// spelling (YETTEL → ЈЕТЕЛ). In Latin mode it is a passthrough, since the keys
// are the Latin forms already present in s. Each key is swapped for a Private
// Use Area placeholder that ToCyrillic passes through untouched, then restored
// to its replacement value.
func ApplyReplacing(lang, s string, repl map[string]string) string {
	if !IsCyrillic(lang) || s == "" || len(repl) == 0 {
		return Apply(lang, s)
	}

	placeholders := make([]string, 0, len(repl))
	values := make([]string, 0, len(repl))
	i := 0
	for k, v := range repl {
		if k == "" || !strings.Contains(s, k) {
			continue
		}
		token := string(rune(0xE100 + i))
		s = strings.ReplaceAll(s, k, token)
		placeholders = append(placeholders, token)
		values = append(values, v)
		i++
	}

	s = ToCyrillic(s)

	for j := range placeholders {
		s = strings.ReplaceAll(s, placeholders[j], values[j])
	}
	return s
}

// digraphs must be matched before single letters (order matters).
var digraphs = []struct {
	from, to string
}{
	{"Dž", "Џ"}, {"DŽ", "Џ"}, {"dž", "џ"},
	{"Lj", "Љ"}, {"LJ", "Љ"}, {"lj", "љ"},
	{"Nj", "Њ"}, {"NJ", "Њ"}, {"nj", "њ"},
}

var single = map[rune]rune{
	'A': 'А', 'a': 'а',
	'B': 'Б', 'b': 'б',
	'V': 'В', 'v': 'в',
	'G': 'Г', 'g': 'г',
	'D': 'Д', 'd': 'д',
	'Đ': 'Ђ', 'đ': 'ђ',
	'E': 'Е', 'e': 'е',
	'Ž': 'Ж', 'ž': 'ж',
	'Z': 'З', 'z': 'з',
	'I': 'И', 'i': 'и',
	'J': 'Ј', 'j': 'ј',
	'K': 'К', 'k': 'к',
	'L': 'Л', 'l': 'л',
	'M': 'М', 'm': 'м',
	'N': 'Н', 'n': 'н',
	'O': 'О', 'o': 'о',
	'P': 'П', 'p': 'п',
	'R': 'Р', 'r': 'р',
	'S': 'С', 's': 'с',
	'T': 'Т', 't': 'т',
	'Ć': 'Ћ', 'ć': 'ћ',
	'U': 'У', 'u': 'у',
	'F': 'Ф', 'f': 'ф',
	'H': 'Х', 'h': 'х',
	'C': 'Ц', 'c': 'ц',
	'Č': 'Ч', 'č': 'ч',
	'Š': 'Ш', 'š': 'ш',
}

// brandPlaceholders keep product names, domains, and filenames in Latin.
// Tokens use the Unicode Private Use Area so ToCyrillic never rewrites them.
var brandPlaceholders = []struct{ from, token string }{
	{"Preuzmi.me", "\uE000"},
	{"preuzmi.me", "\uE001"},
	{"config.json", "\uE002"},
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// literalPattern matches technical literals — commands, handles, config keys and
// acronyms — that are typed or searched verbatim in the real tool, so a Cyrillic
// transliteration would be unusable. Word boundaries keep standalone terms Latin
// ("bot token", "chat") while inflected Serbian forms ("botom", "tokenom") still
// transliterate. Ordered longest-first so specific variants win over "chat".
var literalPattern = regexp.MustCompile(`@BotFather|\bBotFather\b|\bgetUpdates\b|/newbot|/start|\bchat ID\b|\bchatID\b|\bchat_id\b|\bchat\.id\b|\bbot_token\b|<TOKEN>|\bper_receipt\b|\ball_done\b|\busername\b|\bSTARTTLS\b|\bSMTPS\b|\bSMTP\b|\bTLS\b|\bJSON\b|\btelegram\b|\bsmtp\b|\bchat\b`)

// ToCyrillic transliterates Serbian Latin orthography to Cyrillic.
// Digraphs (lj, nj, dž) are handled before single letters; "dj" is not mapped.
// The product name Preuzmi.me, config.json, and http(s) URLs are left in Latin.
func ToCyrillic(s string) string {
	if s == "" {
		return s
	}

	type ph struct{ token, value string }
	var urls []ph
	s = urlPattern.ReplaceAllStringFunc(s, func(u string) string {
		token := string(rune(0xE010 + len(urls)))
		urls = append(urls, ph{token, u})
		return token
	})

	var literals []ph
	s = literalPattern.ReplaceAllStringFunc(s, func(m string) string {
		token := string(rune(0xE030 + len(literals)))
		literals = append(literals, ph{token, m})
		return token
	})

	for _, p := range brandPlaceholders {
		s = strings.ReplaceAll(s, p.from, p.token)
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		matched := false
		for _, d := range digraphs {
			if strings.HasPrefix(s[i:], d.from) {
				b.WriteString(d.to)
				i += len(d.from)
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteByte(s[i])
			i++
			continue
		}
		if c, ok := single[r]; ok {
			b.WriteRune(c)
		} else {
			b.WriteRune(r)
		}
		i += size
	}

	out := b.String()
	for _, p := range brandPlaceholders {
		out = strings.ReplaceAll(out, p.token, p.from)
	}
	for _, u := range urls {
		out = strings.ReplaceAll(out, u.token, u.value)
	}
	for _, l := range literals {
		out = strings.ReplaceAll(out, l.token, l.value)
	}
	return out
}
