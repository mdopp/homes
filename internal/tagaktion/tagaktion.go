// Package tagaktion keeps which NFC tag does something of Homes' own
// instead of reporting to Home Assistant (homes ADR 0004 §4): a tag at the
// scanner starts a stack for whoever touched it, or for the household.
// The tag's name stays in HA's tag registry; only the action is Homes'
// (it is Homes' behaviour, not HA's). One JSON file in HOMES_DATA,
// tag id → action.
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
	ScannenIch      = "scannen-ich"      // a stack for the person who touched it, theirs alone
	ScannenHaushalt = "scannen-haushalt" // a stack for the household
)

// Titel is each action in the family's words, in the order offered.
var Titel = []struct{ ID, Titel string }{
	{"", "Home Assistant melden"},
	{ScannenIch, "Scannen – für mich"},
	{ScannenHaushalt, "Scannen – für den Haushalt"},
}

// Gibt says whether a is an action a tag can carry ("" included).
func Gibt(a string) bool {
	for _, t := range Titel {
		if t.ID == a {
			return true
		}
	}
	return false
}

// Store is the file and what it holds.
type Store struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

// Open reads dir/tag-aktionen.json; a missing file is no action anywhere.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "tag-aktionen.json"), m: map[string]string{}}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, &s.m)
}

// Von is tag's action, "" for none.
func (s *Store) Von(tag string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[tag]
}

// Setze gives tag the action a ("" takes it away) and writes the file.
func (s *Store) Setze(tag, a string) error {
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
