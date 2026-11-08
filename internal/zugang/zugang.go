// Package zugang keeps the access requests from the public teaser page
// (owner 2026-11-08: the domain itself shows visitors without an account
// what the box is; „Zugang anfragen“ is a request a person reads, never a
// registration). One JSON file in HOMES_DATA: each request with its time,
// name, contact, message and a salted hash of the address it came from —
// never the address itself — and when an admin ticked it off. The file is
// the throttle's memory too: one accepted request per address an hour,
// twenty a day for everyone, so a restart forgets nothing.
package zugang

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/jsonfile"
)

// Datei is the file's name in HOMES_DATA.
const Datei = "zugang.json"

// The limits (owner 2026-11-08): what the form says, and what everyone
// together may send in a day.
const (
	ProAdresse = time.Hour
	ProTag     = 20
	MaxName    = 80
	MaxKontakt = 120
	MaxText    = 300
)

// ErrZuViele is a request over the limit: the page says to try later.
var ErrZuViele = errors.New("zu viele Zugangsanfragen")

// Anfrage is one request.
type Anfrage struct {
	ID        string     `json:"id"`
	Zeit      time.Time  `json:"zeit"`
	Name      string     `json:"name"`
	Kontakt   string     `json:"kontakt"`
	Nachricht string     `json:"nachricht,omitempty"`
	Adresse   string     `json:"adresse"` // sha256(salt + address): the same address again, never the address
	Erledigt  *time.Time `json:"erledigt,omitempty"`
}

type datei struct {
	Salz     string    `json:"salz"`
	Anfragen []Anfrage `json:"anfragen"`
}

// Store is the file; Now for the tests.
type Store struct {
	Path string
	Now  func() time.Time
	mu   sync.Mutex
}

// Fehler is what is wrong with a form: "" nothing, else which field.
type Fehler string

const (
	OhneName    Fehler = "name"
	OhneKontakt Fehler = "kontakt"
	ZuLang      Fehler = "nachricht"
)

// Pruefen is the form's three fields trimmed to one line each (the message
// keeps its lines), and what is wrong with them.
func Pruefen(name, kontakt, nachricht string) (Anfrage, Fehler) {
	a := Anfrage{Name: zeile(name), Kontakt: zeile(kontakt), Nachricht: strings.TrimSpace(nachricht)}
	switch {
	case a.Name == "" || utf8.RuneCountInString(a.Name) > MaxName:
		return a, OhneName
	case a.Kontakt == "" || utf8.RuneCountInString(a.Kontakt) > MaxKontakt:
		return a, OhneKontakt
	case utf8.RuneCountInString(a.Nachricht) > MaxText:
		return a, ZuLang
	}
	return a, ""
}

func zeile(s string) string { return strings.Join(strings.Fields(s), " ") }

// Annehmen keeps a (checked by Pruefen) from address, or answers ErrZuViele
// when that address sent one within ProAdresse or everyone sent ProTag today.
func (s *Store) Annehmen(a Anfrage, adresse string) (Anfrage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lesen()
	if err != nil {
		return a, err
	}
	now := s.now()
	a.Adresse = hash(d.Salz, adresse)
	heute := 0
	for _, b := range d.Anfragen {
		if b.Adresse == a.Adresse && now.Sub(b.Zeit) < ProAdresse {
			return a, ErrZuViele
		}
		if now.Sub(b.Zeit) < 24*time.Hour {
			heute++
		}
	}
	if heute >= ProTag {
		return a, ErrZuViele
	}
	a.ID, a.Zeit = zufall(8), now
	d.Anfragen = append(d.Anfragen, a)
	return a, jsonfile.Save(s.Path, d)
}

// Offen are the requests nobody ticked off yet, oldest first.
func (s *Store) Offen() ([]Anfrage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lesen()
	if err != nil {
		return nil, err
	}
	var out []Anfrage
	for _, a := range d.Anfragen {
		if a.Erledigt == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// Erledigen ticks one off; an unknown id is nothing to do.
func (s *Store) Erledigen(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lesen()
	if err != nil {
		return err
	}
	for i := range d.Anfragen {
		if d.Anfragen[i].ID == id && d.Anfragen[i].Erledigt == nil {
			now := s.now()
			d.Anfragen[i].Erledigt = &now
			return jsonfile.Save(s.Path, d)
		}
	}
	return nil
}

// lesen is the file, or a new one with its salt; a file that cannot be
// read is an error, never an empty list (a request must not be lost).
func (s *Store) lesen() (datei, error) {
	var d datei
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return datei{Salz: zufall(16)}, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &d)
	}
	return d, err
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func hash(salz, adresse string) string {
	sum := sha256.Sum256([]byte(salz + "\n" + adresse))
	return hex.EncodeToString(sum[:])
}

func zufall(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
