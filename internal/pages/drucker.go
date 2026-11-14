package pages

import (
	"context"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/drucker"
	"github.com/mdopp/homes/internal/ha"
)

// printer is one of the household's printers (internal/drucker) as the
// pages draw it: its row on Geräte, its colours and check on its own page.
type printer struct{ drucker.Drucker }

// findPrinters are the house's printers (drucker.Find).
func findPrinters(states []ha.State, reg ha.Registry) []printer {
	var out []printer
	for _, p := range drucker.Find(states, reg) {
		out = append(out, printer{p})
	}
	return out
}

// row is the printer's one quiet row on its room's card: its state in
// words (bereit, druckt, angehalten; „wieder erreichbar“ after a check
// found it again) with what asks for a hand and the black ink (Detail), or
// not reachable (since when, or what the last check found, with „Prüfen
// und neu verbinden“, owner 2026-11-14). check is the printer's check running or answered;
// back the card the button lands on.
func (p printer) row(name string, now time.Time, loc *time.Location, check *pruefung, back string) cards.Row {
	r := cards.Row{Text: name + " · nicht erreichbar", Href: cards.ThingPage(p.ID())}
	answered := check != nil && check.answered()
	if !p.Reachable() {
		if p.Status.State == "unavailable" {
			r.Detail = changedWords(p.Status, now, loc)
		}
		p.checkRow(&r, check, answered, back)
		return r
	}
	r.Text = name + " · " + map[bool]string{true: "wieder erreichbar", false: p.Word()}[answered && check.wieder]
	r.Detail, r.Warn = p.Detail()
	return r
}

// checkRow is an unreachable printer's row with its check: „Prüfe Verbindung
// …“ while it runs, what it found once answered, and the button that
// checks (again), a form to DruckerPruefenPath that lands back on the card.
func (p printer) checkRow(r *cards.Row, check *pruefung, answered bool, back string) {
	switch {
	case check != nil && !answered:
		r.Detail = saysPruefe
		return
	case answered && check.netz != "":
		r.Detail = check.netz
	}
	if p.Status.EntityID != "" {
		r.Action = &cards.Action{Label: "Prüfen und neu verbinden", Entity: p.Status.EntityID, Do: "pruefen", Back: back,
			Path: DruckerPruefenPath, Laeuft: "Prüfe …"}
	}
}

// printerRows are the rows for Räume's room cards: „Drucker“, or each
// printer's name when the house has more than one. c is the Home
// Assistant the checks were asked of.
func printerRows(states []ha.State, reg ha.Registry, now time.Time, loc *time.Location, c *ha.Client) []roomExtra {
	ps := findPrinters(states, reg)
	out := make([]roomExtra, 0, len(ps))
	for _, p := range ps {
		name := "Drucker"
		if len(ps) > 1 && p.Status.EntityID != "" {
			name = friendly(p.Status)
		}
		title := trimRunes(p.Room, 60)
		if title == "" {
			title = noRoom
		}
		row := p.row(name, now, loc, laufend(c, p.Status.EntityID), "/raeume#"+cards.Card{Title: title}.ID())
		out = append(out, roomExtra{room: p.Room, name: name, entity: p.Status.EntityID, row: row})
	}
	return out
}

// thingDetail is a thing page's state in the family's words: a printer's
// in its own (bereit, druckt, angehalten), anything else as Räume says it.
func thingDetail(domain string, s ha.State, reg ha.Registry) string {
	if w := drucker.Words[s.State]; w != "" && reg.Platforms[s.EntityID] == "ipp" {
		return w
	}
	return roomDetail(domain, s, reg.TempUnit)
}

// thisPrinter is the IPP printer id is one of the sensors of, as HA's
// states say it now; !ok for anything else, or while HA does not answer.
func thisPrinter(ctx context.Context, cfg StartConfig, id string, reg ha.Registry) (printer, bool) {
	if reg.Platforms[id] != "ipp" {
		return printer{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, backendWait)
	defer cancel()
	states, err := cfg.HA.States(ctx)
	if err != nil {
		return printer{}, false
	}
	p, ok := drucker.Of(states, reg, id)
	return printer{p}, ok
}

// inksLine is all its colours on one line („Farben: Schwarz 12 % · Cyan
// 40 % · …“), "" while HA reaches none of its inks.
func (p printer) inksLine() string {
	var parts []string
	for _, s := range p.Inks {
		if v, ok := drucker.Level(s); ok {
			parts = append(parts, drucker.InkName(s, p.Status)+" "+drucker.Percent(v))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "<p class=k>Farben: " + html.EscapeString(strings.Join(parts, " · ")) + "</p>"
}

// printerPage is what a printer's own page adds under its state: its
// colours, and on its state sensor's page the connection's check.
func printerPage(r *http.Request, cfg StartConfig, id string, reg ha.Registry) string {
	p, ok := thisPrinter(r.Context(), cfg, id, reg)
	if !ok {
		return ""
	}
	if p.Status.EntityID == id {
		return p.inksLine() + printerCheck(r, cfg, id)
	}
	return p.inksLine()
}
