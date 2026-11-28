package napi

import (
	"cmp"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/teilen"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakt austauschen (owner 2026-11-28): two phones touch or one shows a
// QR code; the app starts a session, hands its link to the other phone,
// asks „Was teilen?“ and sets what the public page (pages.Teilen) shows,
// which follows live. Any paired person; a session is its person's alone
// (another's answers 404, as one that never was). Held in memory
// (internal/teilen): a restart ends every session.
//
//	POST   /napi/teilen {"vorname"?} → 201 {"ok":true,"id","link","gueltig_bis"}
//	PUT    /napi/teilen/{id} {"vorname","karte":{…}|null,"wlan":{…}|null,"eintragen"} → {"ok":true}
//	GET    /napi/teilen/{id} → {"ok":true,"geoeffnet","eingetragen","gueltig_bis"}
//	DELETE /napi/teilen/{id} → {"ok":true}
//
// docs/REFERENCE.md has the fields and every refusal.

type teilenAPI struct {
	store     *teilen.Store
	antworten *kontaktantwort.Store // the form of Kontaktdaten eintragen lassen; nil: eintragen 503
	basis     string                // the public page's address (HOMES_KONTAKT_URL); "": 503
	whoami    func(string) (string, bool)
	admin     func(person string) bool
}

func (t *teilenAPI) routes(gs gruppen) {
	if t.store == nil {
		return
	}
	gs.add("kontakte", "POST /napi/teilen", t.neu)
	gs.add("kontakte", "PUT /napi/teilen/{id}", t.setzen)
	gs.add("kontakte", "GET /napi/teilen/{id}", t.stand)
	gs.add("kontakte", "DELETE /napi/teilen/{id}", t.beenden)
}

// person is the device's person; false: the refusal is written.
func (t *teilenAPI) person(w http.ResponseWriter, r *http.Request) (string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	person := ""
	if bearer, ok := bearerOf(r); ok && t.whoami != nil {
		person, _ = t.whoami(bearer)
	}
	switch {
	case !validPerson(person):
		sagNein(w, http.StatusForbidden, saysNoPerson)
	case t.basis == "":
		sagNein(w, http.StatusServiceUnavailable, texte.Von(r).T(texte.TeilenAus))
	default:
		return person, true
	}
	return "", false
}

// teilenNein answers a store's refusal in words.
func teilenNein(w http.ResponseWriter, r *http.Request, err error) {
	sp := texte.Von(r)
	switch {
	case errors.Is(err, teilen.ErrWeg):
		sagNein(w, http.StatusNotFound, sp.T(texte.TeilenWeg))
	case errors.Is(err, teilen.ErrVorbei):
		sagNein(w, http.StatusGone, sp.T(texte.TeilenVorbei))
	case errors.Is(err, teilen.ErrFeld):
		sagNein(w, http.StatusBadRequest, sp.T(texte.TeilenFeld))
	case errors.Is(err, teilen.ErrZuViele):
		sagNein(w, http.StatusTooManyRequests, sp.T(texte.TeilenZuViele))
	case errors.Is(err, teilen.ErrZuOft):
		w.Header().Set("Retry-After", "600")
		sagNein(w, http.StatusTooManyRequests, sp.T(texte.TeilenZuOft))
	default:
		log.Printf("homes: teilen: %v", err)
		sagNein(w, http.StatusServiceUnavailable, sp.T(texte.TeilenAus))
	}
}

// vornameVon is the first name a session starts with: the app's, else the login's first part, capitalized.
func vornameVon(an, person string) string {
	if v := strings.TrimSpace(an); v != "" && utf8.RuneCountInString(v) <= 60 {
		return v
	}
	p, _, _ := strings.Cut(person, "@")
	p, _, _ = strings.Cut(p, ".")
	r, n := utf8.DecodeRuneInString(p)
	return string(unicode.ToUpper(r)) + p[n:]
}

func (t *teilenAPI) neu(w http.ResponseWriter, r *http.Request) {
	person, ok := t.person(w, r)
	if !ok {
		return
	}
	var in struct {
		Vorname string `json:"vorname"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) // no body: the login's name
	id, token, bis, err := t.store.Neu(person, vornameVon(in.Vorname, person))
	if err != nil {
		teilenNein(w, r, err)
		return
	}
	log.Printf("homes: teilen %s: started", id)
	answer(w, http.StatusCreated, map[string]any{"ok": true, "id": id, "link": strings.TrimRight(t.basis, "/") + "/t/" + token, "gueltig_bis": bis.UTC().Format(time.RFC3339)})
}

// teilenAn is what the app sends: exactly the released fields.
type teilenAn struct {
	Vorname   string        `json:"vorname"`
	Karte     *teilen.Karte `json:"karte"`
	WLAN      *teilen.WLAN  `json:"wlan"`
	Eintragen bool          `json:"eintragen"`
}

func (t *teilenAPI) setzen(w http.ResponseWriter, r *http.Request) {
	person, ok := t.person(w, r)
	if !ok {
		return
	}
	sp, id := texte.Von(r), r.PathValue("id")
	var in teilenAn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in) != nil { // never logged: it holds the password
		sagNein(w, http.StatusBadRequest, saysUnreadable)
		return
	}
	if in.Karte != nil && *in.Karte == (teilen.Karte{}) {
		in.Karte = nil
	}
	st, err := t.store.Stand(id, person)
	if err != nil {
		teilenNein(w, r, err)
		return
	}
	var e teilen.Einladung
	if in.Eintragen {
		switch {
		case t.admin == nil || !t.admin(person):
			sagNein(w, http.StatusForbidden, sp.T(texte.PruefenNurAdmins))
			return
		case t.antworten == nil:
			sagNein(w, http.StatusServiceUnavailable, sp.T(texte.FehltAus))
			return
		case st.Einladung.Token == "":
			link, f, err := t.antworten.Einladen(person, cmp.Or(strings.TrimSpace(in.Vorname), st.Vorname), t.basis)
			if err != nil {
				log.Printf("homes: teilen %s: the form's link not made: %v", id, err)
				sagNein(w, http.StatusServiceUnavailable, sp.T(texte.PruefenNicht))
				return
			}
			e = teilen.Einladung{Token: link[strings.LastIndex(link, "/")+1:], ID: f.ID}
		}
	}
	if utf8.RuneCountInString(in.Vorname) > 60 { // the name kept
		in.Vorname = ""
	}
	if err := t.store.Setzen(id, person, strings.TrimSpace(in.Vorname), teilen.Freigabe{Karte: in.Karte, WLAN: in.WLAN, Eintragen: in.Eintragen}, e); err != nil {
		teilenNein(w, r, err)
		return
	}
	log.Printf("homes: teilen %s: released karte=%v wlan=%v eintragen=%v", id, in.Karte != nil, in.WLAN != nil, in.Eintragen)
	answer(w, http.StatusOK, map[string]any{"ok": true})
}

func (t *teilenAPI) stand(w http.ResponseWriter, r *http.Request) {
	person, ok := t.person(w, r)
	if !ok {
		return
	}
	st, err := t.store.Stand(r.PathValue("id"), person)
	if err != nil {
		teilenNein(w, r, err)
		return
	}
	eingetragen := false
	if st.Einladung.ID != "" && t.antworten != nil {
		f, err := t.antworten.Eine(st.Einladung.ID, person)
		eingetragen = err == nil && f.Beantwortet != nil
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "geoeffnet": st.Geoeffnet, "eingetragen": eingetragen, "gueltig_bis": st.Bis.UTC().Format(time.RFC3339)})
}

func (t *teilenAPI) beenden(w http.ResponseWriter, r *http.Request) {
	person, ok := t.person(w, r)
	if !ok {
		return
	}
	if err := t.store.Beenden(r.PathValue("id"), person); err != nil {
		teilenNein(w, r, err)
		return
	}
	log.Printf("homes: teilen %s: ended", r.PathValue("id"))
	answer(w, http.StatusOK, map[string]any{"ok": true})
}
