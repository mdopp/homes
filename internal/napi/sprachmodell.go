package napi

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/mdopp/homes/internal/modellease"
	"github.com/mdopp/homes/internal/sprachmodell"
	"github.com/mdopp/homes/internal/texte"
)

// modell is the language model's card for the app (owner 2026-11-17): the
// same words as the page's (sprachmodell.Ansicht), switched as the paired
// device's person — the right a lamp's switch has, nothing more.
type modell struct {
	s      *sprachmodell.Schalter
	whoami func(bearer string) (string, bool)
	loc    *time.Location
	now    func() time.Time
}

func (m *modell) ansicht(r *http.Request) map[string]any {
	now := time.Now()
	if m.now != nil {
		now = m.now()
	}
	a := m.s.Lesen(r.Context()).Sehen("de", now, cmp.Or(m.loc, time.Local))
	return map[string]any{"ok": true, "karte": a}
}

// GET /napi/sprachmodell → {ok, karte: Ansicht}
func (m *modell) get(w http.ResponseWriter, r *http.Request) { answer(w, http.StatusOK, m.ansicht(r)) }

// POST /napi/sprachmodell {preset, stunden} or {zurueck: true} → {ok, karte},
// or {ok: false, error} with the sentence the person reads.
func (m *modell) post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Preset  string `json:"preset"`
		Stunden int    `json:"stunden"`
		Zurueck bool   `json:"zurueck"`
	}
	person, ok := "", false
	if bearer, has := bearerOf(r); has && m.whoami != nil {
		person, ok = m.whoami(bearer)
	}
	loc := cmp.Or(m.loc, time.Local)
	if !ok || person == "" {
		answer(w, http.StatusForbidden, map[string]any{"ok": false, "error": texte.In("de", texte.SprachmodellFremd)})
		return
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		answer(w, http.StatusBadRequest, map[string]any{"ok": false, "error": sprachmodell.Sagen("de", sprachmodell.ErrNicht, loc)})
		return
	}
	var err error
	if body.Zurueck {
		err = m.s.Zurueck(r.Context(), person)
	} else {
		err = m.s.Umschalten(r.Context(), person, body.Preset, time.Duration(body.Stunden)*time.Hour)
	}
	if err == nil {
		answer(w, http.StatusOK, m.ansicht(r))
		return
	}
	code := http.StatusBadGateway
	var held *modellease.Held
	switch {
	case errors.As(err, &held):
		code = http.StatusConflict
	case errors.Is(err, sprachmodell.ErrNicht):
		code = http.StatusBadRequest
	}
	answer(w, code, map[string]any{"ok": false, "error": sprachmodell.Sagen("de", err, loc)})
}
