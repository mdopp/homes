// Package texte holds Homes' family-visible words by a stable id, one
// table per language (owner 2026-11-04: Homes becomes multilingual — de,
// en, es, fr — after the module work). German holds every word; the other
// languages hold what has been translated, and a word they lack is German
// (uebersetzt). Which language a request gets is the client's (owner
// 2026-11-18): the browser's or the phone's Accept-Language (Von), no
// picker and no setting per person. Older strings move here in that later
// project. The words are in katalog.go (not counted by scripts/loc.sh); the
// logic here is. Standard library only (shared).
package texte

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Sprache is the language a request is answered in: "de", "en", "es" or "fr".
type Sprache string

// sprachenFolge are the languages Homes speaks, German first: the fallback.
var sprachenFolge = []Sprache{"de", "en", "es", "fr"}

// Von is the language of r: its Accept-Language as Waehle reads it.
func Von(r *http.Request) Sprache { return Waehle(r.Header.Get("Accept-Language")) }

// Waehle is the language Homes speaks that an Accept-Language value
// (RFC 9110 §12.5.4) weighs highest, the first of equal weights; a region
// counts as its language (en-US is en). German when it names none of them,
// or only with q=0.
func Waehle(accept string) Sprache {
	best, bestQ := sprachenFolge[0], 0.0
	for _, teil := range strings.Split(accept, ",") {
		tag, param, _ := strings.Cut(teil, ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(param), "q="); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				continue
			}
			q = f
		}
		lang, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
		if q > bestQ && slices.Contains(sprachenFolge, Sprache(lang)) {
			best, bestQ = Sprache(lang), q
		}
	}
	return best
}

// T is a word in s.
func (s Sprache) T(id ID) string { return In(string(s), id) }

// Titel is a module's title in s.
func (s Sprache) Titel(modul string) string { return s.T(ID("modul." + modul)) }

// ID is a word's stable key: the app and other languages refer to it.
type ID string

// sprachen are the tables by language; a missing word falls back to German.
var sprachen = map[string]map[ID]string{"de": de}

// T is a word in German.
func T(id ID) string { return In("de", id) }

// In is a word in a language; German when that language lacks it, and
// the id itself when German does too (a test holds every id to German).
func In(lang string, id ID) string {
	if s, ok := sprachen[lang][id]; ok {
		return s
	}
	if s, ok := uebersetzt[id][lang]; ok {
		return s
	}
	if s, ok := de[id]; ok {
		return s
	}
	return string(id)
}

// Titel is a module's title.
func Titel(modul string) string { return T(ID("modul." + modul)) }

// Anzahl is "1 Datei", "12 Dateien".
func Anzahl(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return Tausender(int64(n)) + " " + many
}

// Tausender is n with the German thousands point: 12.345.
func Tausender(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

// Groesse is a size as the family reads it: "3,2 GB", "450 MB", "12 KB".
func Groesse(n int64) string {
	units := []struct {
		size int64
		name string
	}{{1 << 40, "TB"}, {1 << 30, "GB"}, {1 << 20, "MB"}, {1 << 10, "KB"}}
	for _, u := range units {
		if n >= u.size {
			v := float64(n) / float64(u.size)
			if v >= 10 {
				return Tausender(int64(v+0.5)) + " " + u.name
			}
			return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".", ",", 1) + " " + u.name
		}
	}
	return Tausender(n) + " Bytes"
}
