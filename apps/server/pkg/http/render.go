package http

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/CerealKiller97/preuzmi.me/pkg/script"
	"github.com/rs/zerolog/log"
)

// parseTemplates loads the given HTML files with a `t` helper that transliterates
// Serbian Latin → Cyrillic when lang is cyrillic. Only strings passed through `t`
// (or Apply on view variables) are converted — markup itself is never rewritten.
func parseTemplates(lang string, files ...string) (*template.Template, error) {
	if len(files) == 0 {
		return nil, os.ErrInvalid
	}
	name := filepath.Base(files[0])
	return template.New(name).Funcs(template.FuncMap{
		"t": func(s string) string {
			return script.Apply(lang, s)
		},
	}).ParseFiles(files...)
}

// renderTemplate executes tmpl and writes the result as-is.
func renderTemplate(w http.ResponseWriter, tmpl *template.Template, data any) {
	if err := tmpl.Execute(w, data); err != nil {
		log.Err(err).Msg("Error executing template")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// withPageScript sets HTML lang / data-lang on PageData and transliterates
// user-facing strings that the templates interpolate (title, description).
func withPageScript(data *PageData, lang string) {
	data.Lang = script.Normalize(lang)
	data.HTMLLang = script.HTMLLang(lang)
	data.Locale = script.Locale(lang)
	data.Title = script.Apply(lang, data.Title)
	data.Description = script.Apply(lang, data.Description)
}
