package pages

import (
	"context"
	"encoding/json"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/drucker"
	"github.com/mdopp/homes/internal/erreichbar"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/jsonfile"
)

// Opening a printer's own page connects it again (owner 2026-11-14: „Beim
// Klicken des Gerätes versucht er ihn neu zu verbinden und zu prüfen, ob er
// erreichbar ist“): the page answers at once with „Prüfe Verbindung …“ in
// a place its script fills by POST to DruckerPruefenPath. That asks Home
// Assistant to reload the printer's config entry (homeassistant.
// reload_config_entry, an admin's service: HOMES_HA_TOKEN is the user
// homes's, an administrator), waits a few seconds for the fresh state and,
// while HA still cannot reach it, asks the printer's own address (its
// IPP config entry's host, else its device's configuration_url in HA,
// never one a person sent) whether it answers on the network („Sollte er nicht erreichbar sein, wird
// angezeigt, ob ein Ping auf die Adresse funktioniert“). One check per
// printer at a time; its answer is kept a while, so the page fetched
// again in place (a push, the fallback) shows it instead of checking
// again, and a GET only ever reads it.
const DruckerPruefenPath = "/drucker/pruefen"

var (
	pruefenWarte = 4 * time.Second        // how long the fresh state is waited for
	pruefenTakt  = 400 * time.Millisecond // how often it is read meanwhile
	pruefenHalt  = 10 * time.Minute       // how long an answer is shown on the page fetched again
)

// pruefenMS is how long the page's script waits for the check: the
// reload (3 s), the wait and the network check (erreichbar.Wait), and a margin.
const pruefenMS = "12e3"

const (
	saysPruefe           = "Prüfe Verbindung …"
	saysPruefenLange     = "Die Prüfung hat zu lange gedauert. Öffne die Seite noch einmal, um es neu zu versuchen."
	saysNoReload         = "Home Assistant hat den Drucker nicht neu verbunden."
	saysPingJa           = "Der Drucker antwortet im Netz (Ping), aber Home Assistant erreicht ihn nicht."
	saysPortJa           = "Der Drucker antwortet im Netz, aber Home Assistant erreicht ihn nicht."
	saysNetzNein         = "Der Drucker antwortet nicht im Netz. Ist er an und im WLAN?"
	saysPruefenFremd     = "Prüfen geht nur von Homes' eigener Seite."
	saysAdresseUnbekannt = "Die Adresse des Druckers ist nicht bekannt."
	saysPruefenUnknown   = "Diesen Drucker gibt es nicht."
)

// pruefung is one printer's check: done is closed when it has its answer,
// html on the printer's page, netz the row's sentence while it stays
// unreachable, wieder when it was unreachable and is reachable again.
type pruefung struct {
	done   chan struct{}
	html   string
	netz   string
	wieder bool
	at     time.Time
}

// answered says whether the check has its answer.
func (p *pruefung) answered() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// pruefungen are the checks, by Home Assistant and entity.
var pruefungen = struct {
	sync.Mutex
	m map[pruefKey]*pruefung
}{m: map[pruefKey]*pruefung{}}

type pruefKey struct {
	ha *ha.Client
	id string
}

// laufend is the printer's check still running or answered within pruefenHalt; nil: none.
func laufend(c *ha.Client, id string) *pruefung {
	pruefungen.Lock()
	defer pruefungen.Unlock()
	p := pruefungen.m[pruefKey{c, id}]
	if p == nil {
		return nil
	}
	select {
	case <-p.done:
		if time.Since(p.at) > pruefenHalt {
			return nil
		}
	default:
	}
	return p
}

// starten is a new check of the printer, or the one still running.
func starten(c *ha.Client, id string, check func(*pruefung)) *pruefung {
	pruefungen.Lock()
	if p := pruefungen.m[pruefKey{c, id}]; p != nil {
		select {
		case <-p.done:
		default:
			pruefungen.Unlock()
			return p
		}
	}
	p := &pruefung{done: make(chan struct{})}
	pruefungen.m[pruefKey{c, id}] = p
	pruefungen.Unlock()
	go func() {
		check(p)
		p.at = time.Now()
		close(p.done)
	}()
	return p
}

// DruckerPruefen is the check's answer for the place on the printer's page:
// POST starts it (or joins the one running), GET only reads it; either
// waits for it as long as the page's script does. The row's button on
// Geräte (zurueck: Räume, a room's card) lands back there with 303 once
// the check has answered, the row then saying it.
func DruckerPruefen(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
		id := r.FormValue("geraet")
		var reg ha.Registry
		ok := cfg.HA != nil && ha.EntityRE.MatchString(id)
		if ok {
			reg, ok = cfg.HA.CachedRegistry(r.Context())
		}
		if !ok || reg.Platforms[id] != "ipp" || !strings.HasPrefix(id, "sensor.") {
			http.Error(w, saysPruefenUnknown, http.StatusNotFound)
			return
		}
		p := laufend(cfg.HA, id)
		switch {
		case r.Method == http.MethodPost && !fromHomes(r):
			http.Error(w, saysPruefenFremd, http.StatusForbidden)
			return
		case r.Method == http.MethodPost:
			p = starten(cfg.HA, id, func(p *pruefung) { pruefen(context.WithoutCancel(r.Context()), cfg, id, reg, p) })
		case r.Method != http.MethodGet:
			http.Error(w, "Nur GET und POST.", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if p == nil {
			return // nothing checked: the place goes
		}
		back := r.PostFormValue("zurueck")
		select {
		case <-p.done:
			if inRaeume.MatchString(back) {
				http.Redirect(w, r, back, http.StatusSeeOther)
				return
			}
			w.Write([]byte(p.html))
		case <-time.After(12 * time.Second):
			if inRaeume.MatchString(back) {
				http.Redirect(w, r, back, http.StatusSeeOther) // the row says „Prüfe …“ while it runs
				return
			}
			w.Write([]byte("<p class=k>" + saysPruefenLange + "</p>"))
		case <-r.Context().Done():
		}
	}
}

// pruefen reloads the printer's config entry, waits for its fresh state,
// and says it: reachable with its state and colours, or not with what the
// network says.
func pruefen(ctx context.Context, cfg StartConfig, id string, reg ha.Registry, into *pruefung) {
	before, _ := cfg.HA.State(ctx, id)
	call, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, err := cfg.HA.Call(call, "homeassistant", "reload_config_entry", map[string]any{"entity_id": id})
	cancel()
	var out strings.Builder
	s := before
	if err != nil {
		log.Printf("homes: drucker %q: reload_config_entry: %v", id, err) // a 401 or 403: the token's user is no administrator
		out.WriteString("<p class=k>" + saysNoReload + "</p>")
	} else {
		s = frisch(ctx, cfg, id, before)
	}
	if (printer{drucker.Drucker{Status: s}}).Reachable() {
		word := drucker.Words[s.State]
		if word == "" {
			word = roomDetail("sensor", s, "")
		}
		p, _ := thisPrinter(ctx, cfg, id, reg)
		entryHost(ctx, cfg, reg, id) // kept while HA's diagnostics answer, for when they do not
		into.wieder = !(printer{drucker.Drucker{Status: before}}).Reachable()
		into.html = "<p class=zustand><span class=punkt></span>Drucker erreichbar · " + html.EscapeString(word) + "</p>" + out.String() + p.inksLine()
		return
	}
	out.WriteString("<p class=zustand><span class=\"punkt gelb\"></span>Drucker nicht erreichbar</p>")
	probe := cfg.Probe
	if probe == nil {
		probe = erreichbar.Pruefe
	}
	answer := erreichbar.Unbekannt
	for _, host := range printerHosts(ctx, cfg, reg, id) {
		if answer = probe(ctx, host); answer != erreichbar.Unbekannt {
			break
		}
	}
	into.netz = saysNetz[answer]
	if err != nil {
		into.netz = saysNoReload + " " + into.netz
	}
	out.WriteString("<p class=k>" + saysNetz[answer] + "</p>")
	into.html = out.String()
}

// frisch waits up to pruefenWarte for the printer's state after the reload:
// reachable and new, or reachable and it was not before.
func frisch(ctx context.Context, cfg StartConfig, id string, before ha.State) ha.State {
	s, end := before, time.Now().Add(pruefenWarte)
	for time.Now().Before(end) {
		select {
		case <-time.After(pruefenTakt):
		case <-ctx.Done():
			return s
		}
		if n, err := cfg.HA.State(ctx, id); err == nil {
			s = n
		}
		if (printer{drucker.Drucker{Status: s}}).Reachable() && (s.LastUpdated != before.LastUpdated || !(printer{drucker.Drucker{Status: before}}).Reachable()) {
			return s
		}
	}
	return s
}

// saysNetz is what the printer's address answered, in the family's words.
var saysNetz = map[erreichbar.Antwort]string{erreichbar.Ping: saysPingJa, erreichbar.Port: saysPortJa,
	erreichbar.Nicht: saysNetzNein, erreichbar.Unbekannt: saysAdresseUnbekannt}

// printerHosts are the printer's addresses, the best first: the host its
// IPP config entry was set up with (HA's diagnostics of that entry: an IP
// address on the box), then its device's configuration_url's host (often
// a .local name, which resolves only with mDNS). Never one from a request.
func printerHosts(ctx context.Context, cfg StartConfig, reg ha.Registry, id string) []string {
	var hosts []string
	if host := entryHost(ctx, cfg, reg, id); host != "" {
		hosts = append(hosts, host)
	}
	if u, err := url.Parse(reg.ConfigURLs[reg.Devices[id]]); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" {
		hosts = append(hosts, u.Hostname())
	}
	return hosts
}

// entryHost is the host of the printer's IPP config entry: from HA's
// diagnostics, kept in drucker.json by device; while they fail (HA's IPP
// diagnostics read the coordinator's data, which an unreachable printer's
// entry lacks: a 500 just when it is needed) the one kept. "" when HA never said one.
func entryHost(ctx context.Context, cfg StartConfig, reg ha.Registry, id string) string {
	entry, key := reg.Entries[id], reg.Devices[id]
	if key == "" {
		key = id
	}
	if entry == "" {
		return druckerGemerkt(cfg.Daten)[key]
	}
	ctx, cancel := context.WithTimeout(ctx, backendWait)
	host, err := cfg.HA.EntryHost(ctx, entry)
	cancel()
	if err != nil || host == "" {
		log.Printf("homes: drucker %q: its config entry's address: %v; the one kept", id, err)
		return druckerGemerkt(cfg.Daten)[key]
	}
	druckerMerken(cfg.Daten, key, host)
	return host
}

// druckerDatei is drucker.json's lock: device → host.
var druckerDatei sync.Mutex

// druckerGemerkt are the printers' hosts kept in dir; empty without dir or file.
func druckerGemerkt(dir string) map[string]string {
	m := map[string]string{}
	if dir == "" {
		return m
	}
	druckerDatei.Lock()
	defer druckerDatei.Unlock()
	if b, err := os.ReadFile(filepath.Join(dir, "drucker.json")); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

// druckerMerken keeps host as the printer's (device key) in dir, written only when it changed.
func druckerMerken(dir, key, host string) {
	m := druckerGemerkt(dir)
	if dir == "" || m[key] == host {
		return
	}
	m[key] = host
	druckerDatei.Lock()
	defer druckerDatei.Unlock()
	if err := jsonfile.Save(filepath.Join(dir, "drucker.json"), m); err != nil {
		log.Printf("homes: drucker.json: %v", err)
	}
}

// printerCheck is the check's place on the printer's own page: on opening
// it, a POST that checks; on the page fetched again in place, the answer
// kept, or a GET that waits for the check still running; nothing else.
func printerCheck(r *http.Request, cfg StartConfig, id string) string {
	path := DruckerPruefenPath + "?geraet=" + url.QueryEscape(id)
	meanwhile := "<p class=zustand aria-live=polite><span class=\"punkt grau\"></span>" + saysPruefe + "</p>"
	failed := "<p class=k>" + saysPruefenLange + "</p>"
	p := laufend(cfg.HA, id)
	switch {
	case p != nil && r.Header.Get("X-Homes-Skript") != "":
		select {
		case <-p.done:
			return p.html
		default:
			return nachladenWie(path, false, pruefenMS, meanwhile, failed)
		}
	case r.Header.Get("X-Homes-Skript") != "":
		return ""
	}
	return nachladenWie(path, true, pruefenMS, meanwhile, failed)
}
