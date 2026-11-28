// Package zugang keeps the access requests from the public teaser page
// (owner 2026-11-08: the domain itself shows visitors without an account
// what the box is; „Zugang anfragen“ is a request a person reads, never a
// registration). One JSON file in HOMES_DATA: each request with its time,
// name, contact, message and a salted hash of the address it came from —
// never the address itself — and when an admin ticked it off. The file is
// the throttle's memory too: one accepted request per address an hour,
// twenty a day for everyone, so a restart forgets nothing.
//
// A request an admin refuses (owner 2026-11-28, after spam: „So etwas muss
// ich ablehnen können.“) goes like a ticked-off one, and its sender's
// contact is remembered, normalised (Normal), with when and by whom. A new
// request from a refused sender is not kept: only its trace for the
// throttle (time and the address's hash), so the form answers it exactly
// as any other, over the limit too. Nothing is ever sent to the sender.
package zugang

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// ErrAbgelehnt is a request from a refused sender: dropped, and answered
// as if kept. Its text carries a hash prefix of the sender, never the sender.
var ErrAbgelehnt = errors.New("abgelehnter Absender")

// Anfrage is one request.
type Anfrage struct {
	ID        string     `json:"id"`
	Zeit      time.Time  `json:"zeit"`
	Name      string     `json:"name"`
	Kontakt   string     `json:"kontakt"`
	Nachricht string     `json:"nachricht,omitempty"`
	Adresse   string     `json:"adresse"`            // sha256(salt + address): the same address again, never the address
	Erledigt  *time.Time `json:"erledigt,omitempty"` // ticked off, refused, or a refused sender's trace (no name, no contact)
}

// Sperre is one refused sender: the contact as Normal writes it, when, and
// which admin.
type Sperre struct {
	Kontakt string    `json:"kontakt"`
	Zeit    time.Time `json:"zeit"`
	Von     string    `json:"von"`
}

type datei struct {
	Salz      string    `json:"salz"`
	Anfragen  []Anfrage `json:"anfragen"`
	Abgelehnt []Sperre  `json:"abgelehnt,omitempty"`
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
	if k := Normal(a.Kontakt); d.gesperrt(k) {
		d.Anfragen = append(d.Anfragen, Anfrage{ID: a.ID, Zeit: now, Adresse: a.Adresse, Erledigt: &now}) // the throttle's trace only
		if err := jsonfile.Save(s.Path, d); err != nil {
			return a, err
		}
		return a, fmt.Errorf("%w (%s)", ErrAbgelehnt, hash(d.Salz, k)[:8])
	}
	d.Anfragen = append(d.Anfragen, a)
	return a, jsonfile.Save(s.Path, d)
}

// Normal is a contact as a refusal remembers it: trimmed and lower case;
// an e-mail address without a +tag in its local part (most providers
// deliver a+b@x to a@x), and at gmail.com or googlemail.com (one mailbox)
// without its dots as well; a phone number its digits, a leading + kept.
// Anything else its words, lower case.
func Normal(kontakt string) string {
	k := strings.TrimPrefix(strings.ToLower(zeile(kontakt)), "mailto:")
	if i := strings.LastIndex(k, "@"); i > 0 && !strings.Contains(k, " ") {
		lokal, domain := k[:i], k[i+1:]
		lokal, _, _ = strings.Cut(lokal, "+")
		if domain == "googlemail.com" {
			domain = "gmail.com"
		}
		if domain == "gmail.com" {
			lokal = strings.ReplaceAll(lokal, ".", "")
		}
		return lokal + "@" + domain
	}
	if strings.Trim(k, "0123456789+-/(). ") == "" && strings.ContainsAny(k, "0123456789") {
		n := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, k)
		if strings.HasPrefix(k, "+") {
			n = "+" + n
		}
		return n
	}
	return k
}

func (d datei) gesperrt(kontakt string) bool {
	for _, g := range d.Abgelehnt {
		if g.Kontakt == kontakt {
			return true
		}
	}
	return false
}

// Ablehnen refuses one open request as admin von: it goes like Erledigen,
// with every other open one from the same sender, their words dropped from
// the file, and the sender is remembered. It answers the sender as Normal
// writes it; an unknown id is nothing to do ("").
func (s *Store) Ablehnen(id, von string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lesen()
	if err != nil {
		return "", err
	}
	k := ""
	for _, a := range d.Anfragen {
		if a.ID == id && a.Erledigt == nil {
			k = Normal(a.Kontakt)
		}
	}
	if k == "" {
		return "", nil
	}
	now := s.now()
	for i, a := range d.Anfragen {
		if a.Erledigt == nil && Normal(a.Kontakt) == k {
			d.Anfragen[i] = Anfrage{ID: a.ID, Zeit: a.Zeit, Adresse: a.Adresse, Erledigt: &now}
		}
	}
	if !d.gesperrt(k) {
		d.Abgelehnt = append(d.Abgelehnt, Sperre{Kontakt: k, Zeit: now, Von: von})
	}
	return k, jsonfile.Save(s.Path, d)
}

// Abgelehnte are the refused senders, the latest first.
func (s *Store) Abgelehnte() ([]Sperre, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lesen()
	out := make([]Sperre, 0, len(d.Abgelehnt))
	for i := len(d.Abgelehnt) - 1; i >= 0; i-- {
		out = append(out, d.Abgelehnt[i])
	}
	return out, err
}

// Zulassen forgets one refused sender (as Normal writes it): their next
// request is kept again. One not refused is nothing to do.
func (s *Store) Zulassen(kontakt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.lesen()
	if err != nil {
		return err
	}
	k, rest := Normal(kontakt), d.Abgelehnt[:0]
	for _, g := range d.Abgelehnt {
		if g.Kontakt != k {
			rest = append(rest, g)
		}
	}
	if len(rest) == len(d.Abgelehnt) {
		return nil
	}
	d.Abgelehnt = rest
	return jsonfile.Save(s.Path, d)
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
