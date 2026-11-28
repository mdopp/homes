package napi

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakte fragen (owner 2026-11-28): a contact shared to the app from the
// phone's contacts (Android's share sheet, a vCard) is asked for its own
// data like „Anna selbst fragen“ on its page: the app sends what the vCard
// says, Homes finds the household's card (a number first, then an e-mail,
// then the name), makes the link and answers the message to send with it,
// what the card lacks, and the mobile number for WhatsApp or SMS. Only an
// admin (as on the page), the device's person as einrichten.Admins last
// saw them.
//
//	POST /napi/kontakte/fragen {"name","telefon":[…],"mail":[…],"anlegen"?:bool,"uid"?}
//	  → {"ok":true,"kontakt","vorname","link","text","fehlt":[…],"telefon","neu"?:true}
//	  404 {"ok":false,"error","anlegen":true} · 409 {"ok":false,"error","kandidaten":[{"kontakt","uid"}]}
//
// Kontaktdaten eintragen lassen (owner 2026-11-28): a link for a person
// the household has no card of, the same public form empty, 7 days, once;
// the answer is reviewed on the page (kontaktantwort.Einladen).
//
//	POST /napi/kontakte/einladen (no body) → {"ok":true,"link","text","gueltig_bis"}

type fragen struct {
	dav    *caldav.Client // the household's address book (Haushalt.Contacts); nil: no route
	store  *kontaktantwort.Store
	basis  string // the public page's address (HOMES_KONTAKT_URL); "": 503
	land   string // the house's calling code
	whoami func(string) (string, bool)
	admin  func(person string) bool
}

func (f *fragen) routes(gs gruppen) {
	if f.dav == nil { // kontakte off: no released app reads it
		return
	}
	gs.add("kontakte", "POST /napi/kontakte/fragen", f.post)
	gs.add("kontakte", "POST /napi/kontakte/einladen", f.einladen)
}

// fragenAn is what the app sends: the shared vCard's name, numbers and e-mail addresses.
type fragenAn struct {
	Name    string   `json:"name"`
	Telefon []string `json:"telefon"`
	Mail    []string `json:"mail"`
	Anlegen bool     `json:"anlegen"`
	UID     string   `json:"uid"`
}

// darf is the device's person when they may ask (an admin, the public page there); false: the refusal is written.
func (f *fragen) darf(w http.ResponseWriter, r *http.Request, sp texte.Sprache) (string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	person := personOf(r, f.whoami)
	switch {
	case !validPerson(person):
		refuse(w, http.StatusForbidden, saysNoPerson)
	case f.admin == nil || !f.admin(person):
		refuse(w, http.StatusForbidden, sp.T(texte.PruefenNurAdmins))
	case f.store == nil || f.basis == "":
		refuse(w, http.StatusServiceUnavailable, sp.T(texte.FehltAus))
	default:
		return person, true
	}
	return "", false
}

// einladen makes a new person's link and answers it with the message the share sheet gets.
func (f *fragen) einladen(w http.ResponseWriter, r *http.Request) {
	sp := texte.Von(r)
	person, ok := f.darf(w, r, sp)
	if !ok {
		return
	}
	link, frage, err := f.store.Einladen(person, "", f.basis)
	if err != nil {
		log.Printf("homes: kontakt einladen: the link not made: %v", err)
		refuse(w, http.StatusServiceUnavailable, sp.T(texte.PruefenNicht))
		return
	}
	log.Printf("homes: kontakt einladen: link %s made from the app", frage.ID)
	answer(w, http.StatusOK, map[string]any{"ok": true, "link": link, "text": fmt.Sprintf(sp.T(texte.EinladenText), link), "gueltig_bis": frage.Bis.UTC().Format(time.RFC3339)})
}

func (f *fragen) post(w http.ResponseWriter, r *http.Request) {
	sp := texte.Von(r)
	person, ok := f.darf(w, r, sp)
	if !ok {
		return
	}
	var in fragenAn
	switch {
	case json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil:
		refuse(w, http.StatusBadRequest, saysUnreadable)
	case in.Name == "" && len(in.Telefon) == 0 && len(in.Mail) == 0 && in.UID == "":
		refuse(w, http.StatusBadRequest, sp.T(texte.FehltOhneAngaben))
	default:
		in.Telefon, in.Mail = in.Telefon[:min(len(in.Telefon), kontaktantwort.GrenzeJeArt)], in.Mail[:min(len(in.Mail), kontaktantwort.GrenzeJeArt)]
		if ct, neu, ok := f.kontakt(w, r, sp, in); ok {
			f.link(w, sp, person, ct, neu)
		}
	}
}

// kontakt is the household's card the app means (in.UID, else kontaktantwort.Finden), or the one
// made from it (neu); false: the refusal is written.
func (f *fragen) kontakt(w http.ResponseWriter, r *http.Request, sp texte.Sprache, in fragenAn) (caldav.Contact, bool, bool) {
	all, _, err := f.dav.Contacts(r.Context())
	if err != nil {
		log.Printf("homes: kontakte fragen: %v", err)
		refuse(w, http.StatusBadGateway, sp.T(texte.FehltNichtLesbar))
		return caldav.Contact{}, false, false
	}
	hits, name := kontaktantwort.Finden(all, in.Name, in.Telefon, in.Mail, f.land), cmp.Or(in.Name, at(in.Telefon), at(in.Mail))
	if in.UID != "" {
		hits = nil
		if i := slices.IndexFunc(all, func(ct caldav.Contact) bool { return ct.UID == in.UID }); i >= 0 {
			hits = all[i : i+1]
		}
	}
	switch {
	case len(hits) == 1:
		return hits[0], false, true
	case in.UID != "":
		refuse(w, http.StatusNotFound, sp.T(texte.FehltUIDWeg))
	case len(hits) > 1:
		ks := []map[string]string{}
		for _, ct := range hits {
			ks = append(ks, map[string]string{"kontakt": cmp.Or(ct.Name, ct.Org), "uid": ct.UID})
		}
		answer(w, http.StatusConflict, map[string]any{"ok": false, "error": fmt.Sprintf(sp.T(texte.FehltMehrdeutig), name), "kandidaten": ks})
	case !in.Anlegen || in.Name == "":
		answer(w, http.StatusNotFound, map[string]any{"ok": false, "error": fmt.Sprintf(sp.T(texte.FehltNichtImBuch), name), "anlegen": in.Name != ""})
	default:
		ct, err := kontaktantwort.Anlegen(r.Context(), f.dav, in.Name, in.Telefon, in.Mail, f.land)
		if err == nil {
			return ct, true, true
		}
		log.Printf("homes: kontakte fragen: the new card not written: %v", err)
		refuse(w, http.StatusBadGateway, sp.T(texte.FehltNichtAngelegt))
	}
	return caldav.Contact{}, false, false
}

// link makes the link for ct and answers it with the message the share sheet gets on the page.
func (f *fragen) link(w http.ResponseWriter, sp texte.Sprache, person string, ct caldav.Contact, neu bool) {
	link, frage, err := f.store.Link(person, "", kontaktantwort.Haushalt, ct, f.basis)
	if err != nil {
		log.Printf("homes: kontakte fragen: the link not made: %v", err)
		refuse(w, http.StatusServiceUnavailable, sp.T(texte.PruefenNicht))
		return
	}
	log.Printf("homes: kontakte fragen: link %s made from the app (neu %v)", frage.ID, neu)
	vorname := cmp.Or(frage.Vorher.Vorname, frage.Name(), ct.Name)
	out := map[string]any{"ok": true, "kontakt": cmp.Or(ct.Name, ct.Org), "vorname": vorname, "link": link, "text": fmt.Sprintf(sp.T(texte.PruefenText), vorname, link),
		"fehlt": kontaktantwort.FehltWorte(sp, kontaktantwort.Fehlt(ct, f.land)), "telefon": kontaktantwort.Mobil(ct, f.land)}
	if neu {
		out["neu"] = true
	}
	answer(w, http.StatusOK, out)
}

// at is vs' first, "" none.
func at(vs []string) string {
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}
