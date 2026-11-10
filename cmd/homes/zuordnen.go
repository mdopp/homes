package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/jsonfile"
	"github.com/mdopp/homes/internal/pages"
)

// zuordnenPfad is where Dokumente (mdopp/dokumente ordnen/zuteilen.py) posts what
// it could not give anyone (laib docs/DATA.md, gap 6), on Homes' own port, never
// through Caddy, with the Haushalt bearer it holds: {"offen": [pages.Scan],
// "personen": [pages.Person]}. The answer is the admins' answers for scans still
// open, each with the Zeichen they saw: {"antworten": [{"id", "zeichen", "an"}]}.
// Dokumente acts on an answer only when that Zeichen is still the scan's, so a
// list someone else posted with the bearer cannot make an admin give away what
// they never saw. The list and the answers live in HOMES_DATA/zuordnen.json.
const zuordnenPfad = "/intern/zuordnen"

type antwort struct {
	ID      int    `json:"id"`
	Zeichen string `json:"zeichen"`
	An      string `json:"an"`
	Von     string `json:"von,omitempty"`
	Am      string `json:"am,omitempty"`
}

type zuordnen struct {
	path, token string
	mu          sync.Mutex
	stand       struct {
		Offen     []pages.Scan   `json:"offen"`
		Personen  []pages.Person `json:"personen"`
		Antworten []antwort      `json:"antworten"`
	}
}

// openZuordnen is the list in path; nil without the bearer (no Dokumente can post).
func openZuordnen(path, token string) *zuordnen {
	if token == "" {
		return nil
	}
	z := &zuordnen{path: path, token: token}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &z.stand)
	}
	return z
}

// Stand is the open scans, each with the answer given, and the people.
func (z *zuordnen) Stand() ([]pages.Scan, []pages.Person) {
	z.mu.Lock()
	defer z.mu.Unlock()
	offen := slices.Clone(z.stand.Offen)
	for i, s := range offen {
		if a := slices.IndexFunc(z.stand.Antworten, func(a antwort) bool { return a.ID == s.ID && a.Zeichen == s.Zeichen }); a >= 0 {
			offen[i].An = z.stand.Antworten[a].An
		}
	}
	return offen, slices.Clone(z.stand.Personen)
}

// Antworte keeps an admin's answer for a scan still open as they saw it.
func (z *zuordnen) Antworte(id int, zeichen, an, von string) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	known := an == "haushalt" || slices.ContainsFunc(z.stand.Personen, func(p pages.Person) bool { return p.Login == an })
	if !known || !slices.ContainsFunc(z.stand.Offen, func(s pages.Scan) bool { return s.ID == id && s.Zeichen == zeichen }) {
		return errors.New("zuordnen: no such scan or person")
	}
	z.stand.Antworten = append(slices.DeleteFunc(z.stand.Antworten, func(a antwort) bool { return a.ID == id }),
		antwort{id, zeichen, an, von, time.Now().UTC().Format(time.RFC3339)})
	return jsonfile.Save(z.path, z.stand)
}

func (z *zuordnen) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method != http.MethodPost:
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	case r.Header.Get("X-Homes-Proxy") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Remote-User") != "":
		http.Error(w, "not through the public route", http.StatusForbidden)
		return
	case z.token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+z.token)) != 1:
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		Offen    []pages.Scan   `json:"offen"`
		Personen []pages.Person `json:"personen"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		http.Error(w, "want {offen, personen}", http.StatusBadRequest)
		return
	}
	z.mu.Lock()
	z.stand.Offen, z.stand.Personen = in.Offen, in.Personen
	// an answer whose scan is gone was taken (or the scan given otherwise): dropped
	z.stand.Antworten = slices.DeleteFunc(z.stand.Antworten, func(a antwort) bool {
		return !slices.ContainsFunc(in.Offen, func(s pages.Scan) bool { return s.ID == a.ID && s.Zeichen == a.Zeichen })
	})
	out := map[string]any{"antworten": slices.Clone(z.stand.Antworten)}
	err := jsonfile.Save(z.path, z.stand)
	z.mu.Unlock()
	if err != nil {
		http.Error(w, "not saved", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
