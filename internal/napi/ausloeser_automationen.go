package napi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/mdopp/homes/internal/tagaktion"
)

// A tag's automations (ADR 0004 §8): HA's automations to link, a tag
// linked to one or loosed from it, a new one by a sentence. Every change
// comes first without "bestaetigt" and answers what it would do; with
// "bestaetigt":true an admin's is written, anyone else's waits for an
// admin (a household's approval of its HA automation, not a verdict on
// the box: Homes decides nothing in Laib's inbox, haus#1).
//
//	GET  /napi/automationen?q= → {"ok":true,"automationen":[{"id","name","an"}]}
//	POST /napi/ausloeser/tag/{tag}/verbinden {"automation":id,"bestaetigt"?} → {"ok":true,"satz",
//	     "bestaetigen"?,"freigabe_noetig"?,"wartet"?,"freigabe"?}
//	POST /napi/ausloeser/tag/{tag}/loesen    {"automation":id,"bestaetigt"?} → the same
//	POST /napi/ausloeser/tag/{tag}/neu       {"satz":…,"bestaetigt"?} → the same, and "bote" (the Bote's reply) for an admin
//	GET  /napi/ausloeser/freigabe/{id} → {"ok":true,"stand":"wartet"|"freigegeben"|"abgelehnt"|"verfallen","satz","wunsch"}

func (a *ausloeser) automationRoutes(gs gruppen) {
	gs.add("geraete", "GET /napi/automationen", a.automationen)
	for _, art := range []string{tagaktion.Verbinden, tagaktion.Loesen, tagaktion.NeueAutomation} {
		gs.add("geraete", "POST /napi/ausloeser/tag/{tag}/"+art, a.aendern(art))
	}
	gs.add("geraete", "GET /napi/ausloeser/freigabe/{id}", a.freigabe)
}

func (a *ausloeser) automationen(w http.ResponseWriter, r *http.Request) {
	all, err := a.tag.Automationen(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		haSagt(w, "automationen", err)
		return
	}
	out := []map[string]any{}
	for _, x := range all {
		out = append(out, map[string]any{"id": x.ID, "name": x.Name, "an": x.An})
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "automationen": out})
}

func (a *ausloeser) wer(r *http.Request) tagaktion.Wer {
	person, _ := a.uid(r)
	return tagaktion.Wer{Login: person, Admin: a.admin != nil && a.admin(person)}
}

func (a *ausloeser) aendern(art string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Automation string `json:"automation"`
			Satz       string `json:"satz"`
			Bestaetigt bool   `json:"bestaetigt"`
		}
		tag := r.PathValue("tag")
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil || !tagID.MatchString(tag) {
			sagNein(w, http.StatusBadRequest, "Das kam nicht lesbar an.")
			return
		}
		got, err := a.tag.Aendern(r.Context(), tagaktion.Wunsch{Art: art, Tag: tag, Automation: in.Automation, Satz: in.Satz}, a.wer(r), in.Bestaetigt)
		if err != nil {
			n := tagaktion.Als(err)
			sagNein(w, n.Status, n.Satz)
			return
		}
		out := map[string]any{"ok": true, "satz": got.Satz}
		for k, v := range map[string]bool{"bestaetigen": got.Bestaetigen, "freigabe_noetig": got.FreigabeNoetig, "wartet": got.Wartet} {
			if v {
				out[k] = true
			}
		}
		if got.Freigabe != "" {
			out["freigabe"] = got.Freigabe
		}
		if got.Bote != "" {
			out["bote"] = got.Bote
		}
		answer(w, http.StatusOK, out)
	}
}

// freigabe is a wish's state, for the person who asked and for the admins.
func (a *ausloeser) freigabe(w http.ResponseWriter, r *http.Request) {
	f, ok := a.tag.Freigabe(r.PathValue("id"))
	if wer := a.wer(r); !ok || f.Von != wer.Login && !wer.Admin {
		sagNein(w, http.StatusNotFound, "Diesen Wunsch gibt es nicht (mehr).")
		return
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "stand": f.Stand, "satz": f.Satz(), "wunsch": f.Text})
}
