package cli

import (
	"os"
	"sync"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// resolveLang picks the stdout language: the ETL_LANG environment variable
// overrides the config value, and anything but "ko" falls back to English.
// A display preference must never stop a run, so unknown values are not errors.
func resolveLang(configLang string) language.Tag {
	lang := os.Getenv("ETL_LANG")
	if lang == "" {
		lang = configLang
	}
	if lang == "ko" {
		return language.Korean
	}
	return language.English
}

var registerCatalogOnce sync.Once

// NewPrinter returns the printer for user-facing stdout text. English source
// strings are the single source of truth; Korean comes from the catalog.
// Errors and log files stay English regardless (D18). configLang may be empty
// when no config is loaded yet (then only ETL_LANG applies).
func NewPrinter(configLang string) *message.Printer {
	registerCatalogOnce.Do(registerKoCatalog)
	return message.NewPrinter(resolveLang(configLang))
}
