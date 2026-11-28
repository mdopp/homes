// Package teilen is „Kontakt austauschen“ (owner 2026-11-28): two phones
// touch (or one shows a QR code), the other phone gets a link at once, and
// the sharer's app then asks „Was teilen?“. The page behind the link
// (pages.Teilen, on the public kontakt listener) shows the sharer's first
// name from the start and follows what the sharer releases, live: chosen
// fields of their own card, the guest Wi-Fi, and for an admin the form of
// „Kontaktdaten eintragen lassen“ (kontaktantwort.Einladen).
//
// A session lives Dauer from its start, or until the sharer ends it. It is
// held in memory only: a restart of Homes ends every session, and the
// Wi-Fi password never touches a disk. What was released is wiped the
// moment a session ends or runs out; its husk (the first name, how it
// ended) stays Nachlauf longer, so the page can say so in words. Nothing
// here logs; the callers log ids and which parts, never a value.
package teilen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/caldav"
)

const (
	// Dauer is how long a session lives from its start.
	Dauer = 30 * time.Minute
	// Nachlauf is how long an ended or expired session's husk is kept for its page.
	Nachlauf = 2 * time.Hour
	// OffenJePerson is how many sessions one person may have open at once.
	OffenJePerson = 5
	// NeueJeStunde is how many sessions one person may start within an hour.
	NeueJeStunde = 20
	tokenBytes   = 32 // the link's randomness: 256 bits
)

var (
	ErrWeg     = errors.New("teilen: no such session")      // unknown, another person's, or long gone
	ErrVorbei  = errors.New("teilen: the session is over")  // ended or expired
	ErrZuViele = errors.New("teilen: too many open")        // OffenJePerson
	ErrZuOft   = errors.New("teilen: too many new ones")    // NeueJeStunde
	ErrFeld    = errors.New("teilen: a field has no shape") // a birthday that is no day, a value too long
)

// Karte is what the sharer releases of their own card: only what is set is shown.
type Karte struct {
	Name       string `json:"name,omitempty"`
	Mobil      string `json:"mobil,omitempty"`
	Telefon    string `json:"telefon,omitempty"`
	Mail       string `json:"mail,omitempty"`
	Adresse    string `json:"adresse,omitempty"`
	Geburtstag string `json:"geburtstag,omitempty"` // YYYY-MM-DD or --MM-DD
	Firma      string `json:"firma,omitempty"`
}

// WLAN is the guest Wi-Fi: its name, and its password unless it is open.
type WLAN struct {
	SSID     string `json:"ssid"`
	Kennwort string `json:"kennwort,omitempty"`
	Offen    bool   `json:"offen,omitempty"`
}

// Freigabe is what the page shows; nil parts are not released.
type Freigabe struct {
	Karte     *Karte
	WLAN      *WLAN
	Eintragen bool // the form of Kontaktdaten eintragen lassen (an admin's)
}

// Einladung is the kontaktantwort link the page's form answers to.
type Einladung struct{ Token, ID string }

// Ansicht is a session as its page and its sharer see it.
type Ansicht struct {
	ID, Vorname string
	Bis         time.Time
	Beendet     bool // the sharer ended it
	Abgelaufen  bool // Dauer is over
	Geoeffnet   bool // the page was opened
	Freigabe
	Einladung Einladung
}

// Vorbei says whether the session ended or ran out.
func (a Ansicht) Vorbei() bool { return a.Beendet || a.Abgelaufen }

// Wartet says whether nothing is released yet.
func (a Ansicht) Wartet() bool { return a.Karte == nil && a.WLAN == nil && !a.Eintragen }

type sitzung struct {
	Ansicht
	wer, hash string
	wechsel   chan struct{} // closed on every change, then replaced
}

// Store holds the sessions; the zero value is ready.
type Store struct {
	Now  func() time.Time // the tests'; default time.Now
	Rand io.Reader        // the tests'; default crypto/rand
	mu   sync.Mutex
	alle map[string]*sitzung    // by id
	neu  map[string][]time.Time // each person's starts within the last hour
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) random() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(cmpReader(s.Rand), b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func cmpReader(r io.Reader) io.Reader {
	if r != nil {
		return r
	}
	return rand.Reader
}

func hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// fegen wipes what an expired session released and drops husks past Nachlauf; under mu.
func (s *Store) fegen(now time.Time) {
	for id, x := range s.alle {
		switch {
		case now.After(x.Bis.Add(Nachlauf)):
			delete(s.alle, id)
		case !x.Abgelaufen && !x.Beendet && !now.Before(x.Bis):
			x.Abgelaufen = true
			x.aendern(Freigabe{})
		}
	}
}

// aendern sets what the session shows and wakes its page; under mu.
func (x *sitzung) aendern(f Freigabe) {
	x.Freigabe = f
	close(x.wechsel)
	x.wechsel = make(chan struct{})
}

// Neu starts a session for wer, its page naming vorname: its id, the link's token, its end.
func (s *Store) Neu(wer, vorname string) (string, string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.fegen(now)
	if s.alle == nil {
		s.alle, s.neu = map[string]*sitzung{}, map[string][]time.Time{}
	}
	s.neu[wer] = slices.DeleteFunc(s.neu[wer], func(t time.Time) bool { return now.Sub(t) >= time.Hour })
	offen := 0
	for _, x := range s.alle {
		if x.wer == wer && !x.Vorbei() {
			offen++
		}
	}
	switch {
	case offen >= OffenJePerson:
		return "", "", time.Time{}, ErrZuViele
	case len(s.neu[wer]) >= NeueJeStunde:
		return "", "", time.Time{}, ErrZuOft
	}
	token, err := s.random()
	if err != nil {
		return "", "", time.Time{}, err
	}
	id, err := s.random()
	if err != nil {
		return "", "", time.Time{}, err
	}
	id = id[:16]
	x := &sitzung{Ansicht: Ansicht{ID: id, Vorname: vorname, Bis: now.Add(Dauer)}, wer: wer, hash: hash(token), wechsel: make(chan struct{})}
	s.alle[id], s.neu[wer] = x, append(s.neu[wer], now)
	return id, token, x.Bis, nil
}

// eine is wer's session id; under mu.
func (s *Store) eine(id, wer string) (*sitzung, error) {
	s.fegen(s.now())
	x := s.alle[id]
	if x == nil || x.wer != wer {
		return nil, ErrWeg
	}
	return x, nil
}

// Setzen sets what wer's session id shows (vorname "" keeps it; an Einladung once set stays).
func (s *Store) Setzen(id, wer, vorname string, f Freigabe, e Einladung) error {
	if err := f.pruefen(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.eine(id, wer)
	switch {
	case err != nil:
		return err
	case x.Vorbei():
		return ErrVorbei
	}
	if vorname != "" {
		x.Vorname = vorname
	}
	if e.Token != "" {
		x.Einladung = e
	}
	x.aendern(f)
	return nil
}

// Stand is wer's session id as the sharer sees it; one over answers it with ErrVorbei.
func (s *Store) Stand(id, wer string) (Ansicht, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.eine(id, wer)
	if err != nil {
		return Ansicht{}, err
	}
	if x.Vorbei() {
		return x.Ansicht, ErrVorbei
	}
	return x.Ansicht, nil
}

// Beenden ends wer's session id at once and wipes what it released; one over stays so.
func (s *Store) Beenden(id, wer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.eine(id, wer)
	if err != nil || x.Vorbei() {
		return err
	}
	x.Beendet = true
	x.aendern(Freigabe{})
	return nil
}

// Zum is the session of a link's token as its page shows it, and a channel closed on its next
// change. geoeffnet marks it opened.
func (s *Store) Zum(token string, geoeffnet bool) (Ansicht, <-chan struct{}, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(tokenBytes) || strings.ContainsAny(token, "/.") {
		return Ansicht{}, nil, ErrWeg
	}
	h := hash(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fegen(s.now())
	for _, x := range s.alle {
		if x.hash == h {
			if geoeffnet && !x.Vorbei() && !x.Geoeffnet {
				x.Geoeffnet = true
			}
			return x.Ansicht, x.wechsel, nil
		}
	}
	return Ansicht{}, nil, ErrWeg
}

// pruefen holds the released fields to their limits.
func (f Freigabe) pruefen() error {
	var vs []string
	if k := f.Karte; k != nil {
		if k.Geburtstag != "" && !caldav.ValidBirthday(k.Geburtstag) {
			return ErrFeld
		}
		vs = append(vs, k.Name, k.Mobil, k.Telefon, k.Mail, k.Adresse, k.Firma)
	}
	if w := f.WLAN; w != nil {
		if w.SSID == "" || len(w.SSID) > 64 || len(w.Kennwort) > 128 {
			return ErrFeld
		}
	}
	for _, v := range vs {
		if len(v) > 300 {
			return ErrFeld
		}
	}
	return nil
}
