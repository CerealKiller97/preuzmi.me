package script_test

import (
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/script"
	"github.com/stretchr/testify/assert"
)

func TestToCyrillic(t *testing.T) {
	assert.Equal(t, "Рачун", script.ToCyrillic("Račun"))
	assert.Equal(t, "плаћено", script.ToCyrillic("plaćeno"))
	assert.Equal(t, "Љубав", script.ToCyrillic("Ljubav"))
	assert.Equal(t, "Његош", script.ToCyrillic("Njegoš"))
	assert.Equal(t, "џем", script.ToCyrillic("džem"))
	assert.Equal(t, "Preuzmi.me: рачун", script.ToCyrillic("Preuzmi.me: račun"))
	assert.Equal(t, "Све", script.ToCyrillic("Sve"))
}

func TestApply(t *testing.T) {
	assert.Equal(t, "Račun", script.Apply("latin", "Račun"))
	assert.Equal(t, "Рачун", script.Apply("cyrillic", "Račun"))
	assert.Equal(t, "Рачун", script.Apply("cyrilic", "Račun"))
	assert.Equal(t, "Račun", script.Apply("", "Račun"))
}

func TestApplyReplacing(t *testing.T) {
	// A pinned term is replaced verbatim; the rest transliterates.
	assert.Equal(t, "Рачун за ЈЕТЕЛ преузет",
		script.ApplyReplacing("cyrillic", "Račun za YETTEL preuzet", map[string]string{"YETTEL": "ЈЕТЕЛ"}))
	// Multiple pins: A1 kept Latin, YETTEL custom-spelled, MTS transliterates.
	assert.Equal(t, "МТС, ЈЕТЕЛ, A1",
		script.ApplyReplacing("cyrillic", "MTS, YETTEL, A1", map[string]string{"YETTEL": "ЈЕТЕЛ", "A1": "A1"}))
	// Latin lang is a passthrough regardless of pins.
	assert.Equal(t, "Račun za YETTEL",
		script.ApplyReplacing("latin", "Račun za YETTEL", map[string]string{"YETTEL": "ЈЕТЕЛ"}))
	// No pins behaves like Apply.
	assert.Equal(t, "Рачун", script.ApplyReplacing("cyrillic", "Račun", nil))
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, script.LangLatin, script.Normalize(""))
	assert.Equal(t, script.LangLatin, script.Normalize("latin"))
	assert.Equal(t, script.LangCyrillic, script.Normalize("cyrillic"))
	assert.Equal(t, script.LangCyrillic, script.Normalize("Cyrilic"))
}

func TestToCyrillicKeepsURLs(t *testing.T) {
	in := "Otvori https://api.telegram.org/botTOKEN/getUpdates za chat ID."
	out := script.ToCyrillic(in)
	assert.Contains(t, out, "https://api.telegram.org/botTOKEN/getUpdates")
	assert.Contains(t, out, "Отвори")
}
