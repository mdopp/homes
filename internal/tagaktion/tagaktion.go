// Package tagaktion is what an NFC tag does (homes ADR 0004 §4, §8): it
// reports to Home Assistant (HA's tag_scanned, and the automations that
// trigger on it), or it does something of Homes' own at the scanner —
// ask for whom (open Homes' Scannen), or start a stack directly for
// whoever touched it or for the household, through the scanner's button
// in HA (§6) or, while HA has none, Laib's scan order. The app (napi) and
// the page /tag/<id> act through the same Dienst.
//
// The tag's name stays in HA's tag registry; only the action is Homes'
// (it is Homes' behaviour, not HA's): one JSON file in HOMES_DATA, tag id
// → action.
package tagaktion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/mdopp/homes/internal/jsonfile"
)

// The actions a tag can carry; "" is none: the tag reports to HA.
const (
	ScannerFragen   = "scanner-fragen"   // opens Homes' Scannen: für mich or für den Haushalt (the scanner's default, §8)
	ScannerIch      = "scanner-ich"      // a stack for the person who touched it, theirs alone
	ScannerHaushalt = "scanner-haushalt" // a stack for the household
)

// alt are the names §4 stored, read as §8's (ADR 0004 amendment).
var alt = map[string]string{"scannen-ich": ScannerIch, "scannen-haushalt": ScannerHaushalt}

// Aktuell is a, under its current name: an old one as §8's, any other as it is.
func Aktuell(a string) string {
	if n, ok := alt[a]; ok {
		return n
	}
	return a
}

// Titel is each action in the family's words, in the order offered.
var Titel = []struct{ ID, Titel string }{
	{"", "Home Assistant melden"},
	{ScannerFragen, "Scanner – fragen, für wen"},
	{ScannerIch, "Scanner – direkt für mich"},
	{ScannerHaushalt, "Scanner – direkt für den Haushalt"},
}

// Gibt says whether a is an action a tag can carry ("" included, an old
// name too).
func Gibt(a string) bool {
	for _, t := range Titel {
		if t.ID == Aktuell(a) {
			return true
		}
	}
	return false
}

// TitelVon is a's title, "" for an action there is not.
func TitelVon(a string) string {
	for _, t := range Titel {
		if t.ID == a {
			return t.Titel
		}
	}
	return ""
}

// Store is the file and what it holds.
type Store struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

// Open reads dir/tag-aktionen.json; a missing file is no action anywhere.
// An action stored under its old name is read under its new one (the
// file keeps the old until the next write).
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "tag-aktionen.json"), m: map[string]string{}}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.m); err != nil {
		return nil, err
	}
	for tag, a := range s.m {
		s.m[tag] = Aktuell(a)
	}
	return s, nil
}

// Von is tag's action, "" for none.
func (s *Store) Von(tag string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[tag]
}

// Setze gives tag the action a ("" takes it away) and writes the file.
func (s *Store) Setze(tag, a string) error {
	a = Aktuell(a)
	s.mu.Lock()
	defer s.mu.Unlock()
	alt, had := s.m[tag]
	if a == "" {
		delete(s.m, tag)
	} else {
		s.m[tag] = a
	}
	if err := jsonfile.Save(s.path, s.m); err != nil {
		if had {
			s.m[tag] = alt
		} else {
			delete(s.m, tag)
		}
		return err
	}
	return nil
}
