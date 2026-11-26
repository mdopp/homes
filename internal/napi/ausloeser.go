package napi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/app"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/tagaktion"
)

// The phone as a trigger (ADR 0004 §1): the app reports an NFC tag it
// touched and when it was laid on or taken off its charger; Homes fires
// that as an event on Home Assistant's bus and decides nothing. An NFC tag
// is HA's own tag_scanned (HA's tag trigger), the charger homes_laden. The
// tags' names live in HA's tag registry. A tag can instead carry an action
// of Homes' own (§4, package tagaktion): at the scanner, a stack for whoever
// touched it or for the household; that tag reports nothing to HA.
//
//	POST /napi/ausloeser      {"art":"nfc","tag":id} → {"ok":true,"name":"Bürotür"|"","aktion"?,"text"?}
//	                          {"art":"laden","zustand":"an"|"aus","ladeart":…} → {"ok":true}
//	POST /napi/ausloeser/tag  {"name":"Bürotür","tag"?,"aktion"?} → {"ok":true,"tag":id,"uri":"cloud.dopp.homes://tag/<id>","aktion"}
//	DELETE /napi/ausloeser/tag/{tag} → {"ok":true}
//	GET  /napi/ausloeser/tags → {"ok":true,"tags":[{"tag","name","aktion"}],"vorschlaege":[…],"aktionen":[{"id","titel"}]}
//
// A refusal is {"ok":false,"error": a German sentence}.

// LadenEvent is the event Homes fires for the charger.
const LadenEvent = "homes_laden"

var (
	// a tag id as HA's companion app and Homes write them: letters,
	// digits, dash and underscore
	tagID   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	ladeart = set("kabellos", "kabel", "usb", "")
)

type ausloeser struct {
	ha     *ha.Client
	uid    func(*http.Request) (string, bool)
	laib   *laib.Client     // the scan orders (§4); nil: no action but HA's
	aktion *tagaktion.Store // which tag does what of Homes' own; nil: none
}

// scanLogin is a person the scan service takes (box.yml people, lower case),
// as the Scannen buttons send it.
var scanLogin = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// aktionen are the actions this box offers a tag: HA's always, the scan's
// with Laib and the store.
func (a *ausloeser) aktionen() []map[string]string {
	out := []map[string]string{}
	for _, t := range tagaktion.Titel {
		if t.ID == "" || a.laib != nil && a.aktion != nil {
			out = append(out, map[string]string{"id": t.ID, "titel": t.Titel})
		}
	}
	return out
}

func (a *ausloeser) von(tag string) string {
	if a.aktion == nil || a.laib == nil {
		return ""
	}
	return a.aktion.Von(tag)
}

func (a *ausloeser) name(r *http.Request, tag string) string {
	tags, err := a.ha.Tags(r.Context())
	if err != nil { // the name is a courtesy: what the tag does has been done
		return ""
	}
	for _, t := range tags {
		if t.ID == tag {
			return t.Name
		}
	}
	return ""
}

// nfc is a touched tag: its action of Homes' own, else HA's tag_scanned.
func (a *ausloeser) nfc(w http.ResponseWriter, r *http.Request, tag, person string) {
	if akt := a.von(tag); akt != "" {
		a.scannen(w, r, tag, akt, person)
		return
	}
	if err := a.ha.Fire(r.Context(), "tag_scanned", map[string]any{"tag_id": tag, "person": person}); err != nil {
		haSagt(w, "nfc", err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "name": a.name(r, tag)})
}

// scannen starts a stack as the Scannen buttons do: for the person who
// touched the tag, theirs alone, or for the household. „Für mich“ is
// always the toucher's, never the tag's maker's.
func (a *ausloeser) scannen(w http.ResponseWriter, r *http.Request, tag, akt, person string) {
	if !scanLogin.MatchString(person) {
		person = ""
	}
	fuer, text := "", "Der Scanner startet – für den Haushalt."
	if akt == tagaktion.ScannenIch {
		if person == "" {
			sagNein(w, http.StatusForbidden, "Für dich allein scannt nur ein Telefon, das mit deinem Login gekoppelt ist.")
			return
		}
		fuer, text = person, "Der Scanner startet – nur für dich."
	}
	if _, err := a.laib.ScanOrder(r.Context(), "start", person, fuer); err != nil {
		log.Printf("homes: ausloeser scannen: %v", err)
		if errors.Is(err, laib.ErrNoScan) {
			sagNein(w, http.StatusServiceUnavailable, "Auf dieser Box ist kein Scanner eingerichtet.")
			return
		}
		sagNein(w, http.StatusBadGateway, "Der Scanner antwortet gerade nicht. Versuch es gleich noch einmal.")
		return
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "name": a.name(r, tag), "aktion": akt, "text": text})
}

func (a *ausloeser) routes(gs gruppen) {
	gs.add("geraete", "POST /napi/ausloeser", a.melden)
	gs.add("geraete", "POST /napi/ausloeser/tag", a.neuerTag)
	gs.add("geraete", "DELETE /napi/ausloeser/tag/{tag}", a.tagWeg)
	gs.add("geraete", "GET /napi/ausloeser/tags", a.tags)
}

func sagNein(w http.ResponseWriter, code int, satz string) {
	answer(w, code, map[string]any{"ok": false, "error": satz})
}

// haSagt is a failed HA call in the person's words; HA's detail goes to the log.
func haSagt(w http.ResponseWriter, was string, err error) {
	log.Printf("homes: ausloeser %s: %v", was, err)
	var he *ha.Error
	if errors.Is(err, ha.ErrNotAdmin) || errors.As(err, &he) && he.Status == http.StatusUnauthorized {
		sagNein(w, http.StatusBadGateway, "Home Assistant lässt Homes das nicht tun. Bitte sag dem Admin Bescheid.")
		return
	}
	sagNein(w, http.StatusBadGateway, "Home Assistant antwortet gerade nicht. Versuch es gleich noch einmal.")
}

func (a *ausloeser) melden(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Art     string `json:"art"`
		Tag     string `json:"tag"`
		Zustand string `json:"zustand"`
		Ladeart string `json:"ladeart"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		sagNein(w, http.StatusBadRequest, "Das kam nicht lesbar an.")
		return
	}
	person, _ := a.uid(r)
	switch in.Art {
	case "nfc":
		if !tagID.MatchString(in.Tag) {
			sagNein(w, http.StatusBadRequest, "Diesen Tag kennt Homes nicht. Beschreibe ihn in der App neu.")
			return
		}
		a.nfc(w, r, in.Tag, person)
	case "laden":
		if in.Zustand != "an" && in.Zustand != "aus" || !ladeart[in.Ladeart] {
			sagNein(w, http.StatusBadRequest, "Das kam nicht lesbar an.")
			return
		}
		if err := a.ha.Fire(r.Context(), LadenEvent, map[string]any{"person": person, "zustand": in.Zustand, "ladeart": in.Ladeart}); err != nil {
			haSagt(w, "laden", err)
			return
		}
		answer(w, http.StatusOK, map[string]any{"ok": true})
	default:
		sagNein(w, http.StatusBadRequest, "Das kennt Homes nicht. Bitte aktualisiere die App.")
	}
}

// neuerTag names a tag: a new id without "tag", else that id, named or
// renamed in place (the tag is not written again, ADR 0004 §2). A name is
// unique among the tags, case aside: a name that is said finds one tag.
func (a *ausloeser) neuerTag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   string  `json:"name"`
		Tag    string  `json:"tag"`
		Aktion *string `json:"aktion"` // absent: the tag keeps its action
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil || in.Tag != "" && !tagID.MatchString(in.Tag) {
		sagNein(w, http.StatusBadRequest, "Das kam nicht lesbar an.")
		return
	}
	if in.Aktion != nil && *in.Aktion != "" && (!tagaktion.Gibt(*in.Aktion) || a.laib == nil || a.aktion == nil) {
		sagNein(w, http.StatusBadRequest, "Das kann ein Tag auf dieser Box nicht tun.")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 60 {
		sagNein(w, http.StatusBadRequest, "Gib dem Tag einen kurzen Namen, zum Beispiel „Bürotür“.")
		return
	}
	tags, err := a.ha.Tags(r.Context())
	if err != nil {
		haSagt(w, "tag", err)
		return
	}
	known := false
	for _, t := range tags {
		known = known || t.ID == in.Tag
		if t.ID != in.Tag && strings.EqualFold(strings.TrimSpace(t.Name), name) {
			sagNein(w, http.StatusConflict, "Es gibt schon einen Tag „"+t.Name+"“ – nimm einen anderen Namen.")
			return
		}
	}
	id := in.Tag
	switch {
	case known:
		err = a.ha.UpdateTag(r.Context(), id, name)
	case id != "": // an id HA has not seen: one HA's app or another phone wrote
		err = a.ha.CreateTag(r.Context(), id, name)
	default:
		var b [6]byte
		rand.Read(b[:])
		id = "homes-" + hex.EncodeToString(b[:])
		err = a.ha.CreateTag(r.Context(), id, name)
	}
	if err != nil {
		haSagt(w, "tag", err)
		return
	}
	if in.Aktion != nil && a.aktion != nil {
		if err := a.aktion.Setze(id, *in.Aktion); err != nil {
			log.Printf("homes: ausloeser aktion: %v", err)
			sagNein(w, http.StatusInternalServerError, "Der Name ist gespeichert, was der Tag tun soll, aber nicht. Versuch es noch einmal.")
			return
		}
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "tag": id, "uri": app.Scheme + "://tag/" + id, "aktion": a.von(id)})
}

func (a *ausloeser) tagWeg(w http.ResponseWriter, r *http.Request) {
	err := a.ha.DeleteTag(r.Context(), r.PathValue("tag"))
	switch {
	case errors.Is(err, ha.ErrNotFound):
		sagNein(w, http.StatusNotFound, "Diesen Tag gibt es nicht mehr.")
	case err != nil:
		haSagt(w, "tag weg", err)
	default:
		if a.aktion != nil {
			if err := a.aktion.Setze(r.PathValue("tag"), ""); err != nil {
				log.Printf("homes: ausloeser aktion weg: %v", err) // the tag is gone from HA; a stale action is never reached
			}
		}
		answer(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// tagOrte are the places offered after the house's rooms for a tag's name.
var tagOrte = []string{"Tür", "Bett", "Schreibtisch", "Ladestation"}

func (a *ausloeser) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := a.ha.Tags(r.Context())
	if err != nil {
		haSagt(w, "tags", err)
		return
	}
	out := []map[string]string{}
	for _, t := range tags {
		out = append(out, map[string]string{"tag": t.ID, "name": t.Name, "aktion": a.von(t.ID)})
	}
	reg, _ := a.ha.CachedRegistry(r.Context()) // the rooms are a suggestion: without them, the places
	vor := slices.Sorted(maps.Keys(reg.AreaIDs))
	answer(w, http.StatusOK, map[string]any{"ok": true, "tags": out, "vorschlaege": append(vor, tagOrte...), "aktionen": a.aktionen()})
}
