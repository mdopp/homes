package pages

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/drucker"
	"github.com/mdopp/homes/internal/erreichbar"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/ha/hafake"
)

// an Epson through HA's IPP integration in the Büro, as on the box
// (2026-11-14): its state sensor, four inks (black not first by entity id)
// and its uptime; a lamp in the same room.
func epson(t *testing.T, fake *hafake.HA, status string, black float64) {
	t.Helper()
	fake.SetRegistries(
		[]map[string]any{{"area_id": "buero", "name": "Büro"}},
		[]map[string]any{{"id": "dev-epson", "area_id": "buero"}},
		[]map[string]any{
			{"entity_id": "light.buero", "area_id": "buero"},
			{"entity_id": "sensor.epson_wf_3540_series", "device_id": "dev-epson", "platform": "ipp"},
			{"entity_id": "sensor.epson_wf_3540_series_cyan_ink", "device_id": "dev-epson", "platform": "ipp"},
			{"entity_id": "sensor.epson_wf_3540_series_black_ink", "device_id": "dev-epson", "platform": "ipp"},
			{"entity_id": "sensor.epson_wf_3540_series_magenta_ink", "device_id": "dev-epson", "platform": "ipp"},
			{"entity_id": "sensor.epson_wf_3540_series_yellow_ink", "device_id": "dev-epson", "platform": "ipp"},
			{"entity_id": "sensor.epson_wf_3540_series_uptime", "device_id": "dev-epson", "platform": "ipp"},
		})
	fake.Set("light.buero", "off", map[string]any{"friendly_name": "Schreibtisch"})
	fake.Set("sensor.epson_wf_3540_series", status, map[string]any{"friendly_name": "EPSON WF-3540 Series", "device_class": "enum",
		"options": []any{"idle", "printing", "stopped"}})
	ink := func(id, name, state string) {
		if status == "unavailable" {
			state = "unavailable"
		}
		fake.Set(id, state, map[string]any{"friendly_name": "EPSON WF-3540 Series " + name, "unit_of_measurement": "%",
			"marker_type": "ink-cartridge", "marker_low_level": 15, "marker_high_level": 100})
	}
	ink("sensor.epson_wf_3540_series_cyan_ink", "Cyan ink", "40")
	ink("sensor.epson_wf_3540_series_black_ink", "Black ink", decimalDot(black))
	ink("sensor.epson_wf_3540_series_magenta_ink", "Magenta ink", "35")
	ink("sensor.epson_wf_3540_series_yellow_ink", "Yellow ink", "50")
	fake.Set("sensor.epson_wf_3540_series_uptime", "2026-10-01T08:00:00+00:00", map[string]any{"friendly_name": "EPSON WF-3540 Series Uptime",
		"device_class": "timestamp"})
}

// the clock of these tests: the fake's states changed at 10:00 UTC that day
var druckerCfg = func(hc *ha.Client) StartConfig {
	return StartConfig{HA: hc, Loc: time.UTC, Now: func() time.Time { return time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC) }}
}

// The printer is found by its registry platform, not its entity id: one
// device's sensors together, the state sensor apart from the inks, the
// uptime left out, the room the device's; an ink-only lookalike of
// another platform is no printer.
func TestDruckerFound(t *testing.T) {
	states := []ha.State{
		{EntityID: "sensor.buero_drucker", State: "idle", Attributes: map[string]any{"friendly_name": "Bürodrucker", "device_class": "enum"}},
		{EntityID: "sensor.buero_drucker_tinte_schwarz", State: "12", Attributes: map[string]any{"friendly_name": "Bürodrucker Schwarz", "unit_of_measurement": "%"}},
		{EntityID: "sensor.buero_drucker_laufzeit", State: "2026-10-01T08:00:00+00:00", Attributes: map[string]any{"device_class": "timestamp"}},
		{EntityID: "sensor.akku_handy", State: "80", Attributes: map[string]any{"unit_of_measurement": "%"}},
	}
	reg := ha.Registry{Platforms: map[string]string{"sensor.buero_drucker": "ipp", "sensor.buero_drucker_tinte_schwarz": "ipp",
		"sensor.buero_drucker_laufzeit": "ipp", "sensor.akku_handy": "mobile_app"},
		Devices: map[string]string{"sensor.buero_drucker": "d", "sensor.buero_drucker_tinte_schwarz": "d", "sensor.buero_drucker_laufzeit": "d"},
		Rooms:   map[string]string{"sensor.buero_drucker": "Büro"}}
	ps := findPrinters(states, reg)
	if len(ps) != 1 {
		t.Fatalf("printers %d, want 1: %+v", len(ps), ps)
	}
	p := ps[0]
	if p.Status.EntityID != "sensor.buero_drucker" || len(p.Inks) != 1 || p.Room != "Büro" {
		t.Errorf("printer %+v", p)
	}
	if b, ok := p.Black(); !ok || b.EntityID != "sensor.buero_drucker_tinte_schwarz" {
		t.Errorf("black %v %v (Schwarz in the name)", b, ok)
	}
}

// Black is the marker named Black or Schwarz, wherever it stands among the
// markers; a printer without one has no black (no row detail, no guess).
func TestDruckerBlackAmongMarkers(t *testing.T) {
	fake, hc := house(t)
	epson(t, fake, "idle", 12)
	states, _ := hc.States(t.Context())
	reg, _ := hc.CachedRegistry(t.Context())
	ps := findPrinters(states, reg)
	if len(ps) != 1 {
		t.Fatalf("printers %d", len(ps))
	}
	b, ok := ps[0].Black()
	if !ok || b.EntityID != "sensor.epson_wf_3540_series_black_ink" {
		t.Errorf("black %v %v", b.EntityID, ok)
	}
	var names []string
	for _, s := range ps[0].Inks {
		names = append(names, drucker.InkName(s, ps[0].Status))
	}
	if got := strings.Join(names, ","); got != "Schwarz,Cyan,Magenta,Gelb" {
		t.Errorf("inks %s", got)
	}
	colour := printer{drucker.Drucker{Status: ha.State{EntityID: "sensor.p", State: "idle"}, Inks: []ha.State{{EntityID: "sensor.p_c", State: "40",
		Attributes: map[string]any{"friendly_name": "P Cyan"}}}}}
	if _, ok := colour.Black(); ok {
		t.Error("a printer with only cyan has black")
	}
	if r := colour.row("Drucker", time.Now(), time.UTC, nil, "/raeume"); r.Text != "Drucker · bereit" || r.Detail != "" {
		t.Errorf("row without black %+v", r)
	}
}

// Reachable: „Drucker · bereit“ (idle, the page's word) with the black ink, in the printer's
// room; its tap opens the state sensor's own page; the other colours are
// not on the row.
func TestDruckerReachable(t *testing.T) {
	fake, hc := house(t)
	epson(t, fake, "idle", 42)
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `<a href="/raeume/sensor.epson_wf_3540_series">Drucker · bereit</a> <span class=k>Schwarz 42 %</span>`)
	before(t, body, "karte-buero", "Drucker · bereit")
	before(t, body, "Schreibtisch", "Drucker · bereit") // the printer last in its room
	mustNotContain(t, body, "Cyan", "Gelb", "Uptime", "fast leer")
}

// Unreachable (HA says unavailable): „nicht erreichbar“ since when, no
// ink; unknown says it is not reachable too, without a since.
func TestDruckerUnreachable(t *testing.T) {
	fake, hc := house(t)
	epson(t, fake, "unavailable", 12)
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `<a href="/raeume/sensor.epson_wf_3540_series">Drucker · nicht erreichbar</a> <span class=k>seit heute 10:00</span>`)
	mustNotContain(t, body, "Schwarz", "Drucker · bereit")
	p := printer{drucker.Drucker{Status: ha.State{EntityID: "sensor.p", State: "unknown"}}}
	if r := p.row("Drucker", time.Now(), time.UTC, nil, "/raeume"); r.Text != "Drucker · nicht erreichbar" || r.Detail != "" {
		t.Errorf("unknown %+v", r)
	}
	gone := printer{drucker.Drucker{Inks: []ha.State{{EntityID: "sensor.p_black", State: "unavailable", Attributes: map[string]any{"friendly_name": "P Black"}}}}}
	if r := gone.row("Drucker", time.Now(), time.UTC, nil, "/raeume"); r.Text != "Drucker · nicht erreichbar" || r.Href != "/raeume/sensor.p_black" {
		t.Errorf("missing state sensor %+v", r)
	}
}

// Under 15 % the black is a warning, in words and in its colour; at 15 %
// it is not yet.
func TestDruckerInkWarning(t *testing.T) {
	for _, c := range []struct {
		level  float64
		detail string
		warn   bool
	}{{12, "Schwarz fast leer · 12 %", true}, {14.6, "Schwarz 15 %", false}, {15, "Schwarz 15 %", false}, {0, "Schwarz fast leer · 0 %", true}} {
		p := printer{drucker.Drucker{Status: ha.State{EntityID: "sensor.p", State: "printing"}, Inks: []ha.State{{EntityID: "sensor.p_k",
			State: decimalDot(c.level), Attributes: map[string]any{"friendly_name": "P Black ink"}}}}}
		if r := p.row("Drucker", time.Now(), time.UTC, nil, "/raeume"); r.Detail != c.detail || r.Warn != c.warn {
			t.Errorf("%v: %+v", c.level, r)
		}
	}
	fake, hc := house(t)
	epson(t, fake, "idle", 12)
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `Drucker · bereit</a> <span class="k warn">Schwarz fast leer · 12 %</span>`)
}

// No IPP printer, no row and nothing about printers.
func TestDruckerNone(t *testing.T) {
	fake, hc := house(t)
	rooms(t, fake)
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustNotContain(t, body, "Drucker", "Schwarz", "k warn")
	if ps := findPrinters([]ha.State{{EntityID: "sensor.x", State: "1"}}, ha.Registry{}); len(ps) != 0 {
		t.Errorf("printers %+v", ps)
	}
}

// The printer's own page says its state in words and has all its colours
// on one line; a page of another sensor has none.
func TestDruckerDingFarben(t *testing.T) {
	fake, hc := house(t)
	epson(t, fake, "printing", 12)
	_, body, _ := familyPage(t, Ding, druckerCfg(hc), "GET", "/raeume/sensor.epson_wf_3540_series", "")
	mustContain(t, body, "druckt", "<p class=k>Farben: Schwarz 12 % · Cyan 40 % · Magenta 35 % · Gelb 50 %</p>")
	fake.Set("sensor.temp", "21", map[string]any{"friendly_name": "Temperatur", "unit_of_measurement": "°C"})
	_, body, _ = familyPage(t, Ding, druckerCfg(hc), "GET", "/raeume/sensor.temp", "")
	mustNotContain(t, body, "Farben:")
}

// decimalDot is a level as HA sends a sensor's state: 12, 14.6.
func decimalDot(v float64) string { return strings.Replace(decimal(v), ",", ".", 1) }

// pruefe is the place's POST (or GET) as the page's script sends it.
func pruefe(t *testing.T, cfg StartConfig, method, id string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, DruckerPruefenPath+"?geraet="+id, nil)
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Homes-Skript", "1")
	rec := httptest.NewRecorder()
	DruckerPruefen(cfg).ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// fastChecks makes the check's waits short for a test.
func fastChecks(t *testing.T) {
	w, k := pruefenWarte, pruefenTakt
	pruefenWarte, pruefenTakt = 300*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { pruefenWarte, pruefenTakt = w, k })
}

// a printer with its addresses in HA as on the box (2026-11-14): its
// device's configuration_url a .local name, its IPP config entry's host
// (its diagnostics) an IP address
func epsonAt(t *testing.T, fake *hafake.HA, status string) {
	t.Helper()
	epson(t, fake, status, 12)
	fake.SetRegistries(
		[]map[string]any{{"area_id": "buero", "name": "Büro"}},
		[]map[string]any{{"id": "dev-epson", "area_id": "buero", "configuration_url": "http://EPSON0A36A8.local.:80/PRESENTATION/BONJOUR"}},
		[]map[string]any{
			{"entity_id": "sensor.epson_wf_3540_series", "device_id": "dev-epson", "platform": "ipp", "config_entry_id": "entry-ipp"},
			{"entity_id": "sensor.epson_wf_3540_series_black_ink", "device_id": "dev-epson", "platform": "ipp", "config_entry_id": "entry-ipp"},
		})
	fake.SetDiagnostics("entry-ipp", map[string]any{"entry": map[string]any{"data": map[string]any{"host": "192.168.178.87", "port": 631,
		"base_path": "/Epson_IPP_Printer", "ssl": false, "verify_ssl": false}, "unique_id": "cfe92100-67c4-11d4-a45f-9caed30a36a8"}})
}

// Opening the printer's page answers at once with „Prüfe Verbindung …“ in
// a place its script POSTs; nothing is reloaded by the GET itself; an ink's
// page has no check. The page fetched again in place starts none either.
func TestDruckerPageOpensTheCheck(t *testing.T) {
	fake, hc := house(t)
	epsonAt(t, fake, "idle")
	_, body, _ := familyPage(t, Ding, druckerCfg(hc), "GET", "/raeume/sensor.epson_wf_3540_series", "")
	mustContain(t, body, `<div data-nachladen="/drucker/pruefen?geraet=sensor.epson_wf_3540_series" data-post data-ms=12e3 aria-busy=true>`, "Prüfe Verbindung …",
		"Die Prüfung hat zu lange gedauert.")
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("opening the page called HA %d times", n)
	}
	_, body, _ = familyPage(t, Ding, druckerCfg(hc), "GET", "/raeume/sensor.epson_wf_3540_series_black_ink", "")
	mustNotContain(t, body, "Prüfe Verbindung")
	req := httptest.NewRequest("GET", "/raeume/sensor.epson_wf_3540_series", nil)
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("X-Homes-Skript", "1")
	rec := httptest.NewRecorder()
	Ding(druckerCfg(hc)).ServeHTTP(rec, req)
	mustNotContain(t, rec.Body.String(), `data-nachladen="/drucker`)
}

// The check reloads the printer's config entry in HA with the state
// sensor as its target, and says the fresh state: reachable, druckt,
// with its colours.
func TestDruckerCheckReloads(t *testing.T) {
	fastChecks(t)
	fake, hc := house(t)
	epsonAt(t, fake, "printing")
	cfg := druckerCfg(hc)
	cfg.Probe = func(context.Context, string) erreichbar.Antwort {
		t.Error("a reachable printer was probed")
		return erreichbar.Nicht
	}
	code, body := pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series")
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	calls := fake.Calls()
	if len(calls) != 1 || calls[0].Domain != "homeassistant" || calls[0].Service != "reload_config_entry" ||
		calls[0].Data["entity_id"] != "sensor.epson_wf_3540_series" {
		t.Fatalf("calls %+v", calls)
	}
	mustContain(t, body, "Drucker erreichbar · druckt", "Farben: Schwarz 12 %")
	// the page fetched again in place shows the answer kept, and checks no more
	req := httptest.NewRequest("GET", "/raeume/sensor.epson_wf_3540_series", nil)
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("X-Homes-Skript", "1")
	rec := httptest.NewRecorder()
	Ding(cfg).ServeHTTP(rec, req)
	mustContain(t, rec.Body.String(), "Drucker erreichbar · druckt")
	if _, again := pruefe(t, cfg, "GET", "sensor.epson_wf_3540_series"); !strings.Contains(again, "erreichbar") || len(fake.Calls()) != 1 {
		t.Errorf("GET %q, calls %d: a GET reads, never reloads", again, len(fake.Calls()))
	}
}

// Still unreachable after the reload: the printer's own address (from HA,
// never the request) is asked; each answer in its words.
func TestDruckerCheckUnreachableProbes(t *testing.T) {
	fastChecks(t)
	for _, c := range []struct {
		a    erreichbar.Antwort
		says string
	}{{erreichbar.Ping, "Der Drucker antwortet im Netz (Ping), aber Home Assistant erreicht ihn nicht."},
		{erreichbar.Port, "Der Drucker antwortet im Netz, aber Home Assistant erreicht ihn nicht."},
		{erreichbar.Nicht, "Der Drucker antwortet nicht im Netz. Ist er an und im WLAN?"}} {
		fake, hc := house(t)
		epsonAt(t, fake, "unavailable")
		cfg := druckerCfg(hc)
		var asked []string
		cfg.Probe = func(_ context.Context, host string) erreichbar.Antwort { asked = append(asked, host); return c.a }
		_, body := pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series&host=10.0.0.1") // a host sent along is never asked
		mustContain(t, body, "Drucker nicht erreichbar", c.says)
		if strings.Join(asked, ",") != "192.168.178.87" { // the config entry's host first
			t.Errorf("%v: probed %v", c.a, asked)
		}
	}
}

// HA refusing the reload (a token that is no administrator's) is said; the
// state then is the one HA has, and without an address nothing is probed.
func TestDruckerCheckReloadRefused(t *testing.T) {
	fastChecks(t)
	fake, hc := house(t)
	epson(t, fake, "unavailable", 12)
	fake.FailCalls(401)
	cfg := druckerCfg(hc)
	cfg.Probe = func(context.Context, string) erreichbar.Antwort {
		t.Error("probed without an address")
		return erreichbar.Nicht
	}
	_, body := pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series")
	mustContain(t, body, "Home Assistant hat den Drucker nicht neu verbunden.", "Drucker nicht erreichbar",
		"Die Adresse des Druckers ist nicht bekannt.")
	mustNotContain(t, body, "im Netz")
}

// The box's case (2026-11-14): HA's diagnostics give no host, the
// device's configuration_url is a .local name Homes' container cannot
// resolve. That is said as not known, never as „antwortet nicht“; with
// the entry's host unresolved too, the .local name is still tried.
func TestDruckerCheckLocalNameUnresolved(t *testing.T) {
	fastChecks(t)
	for _, c := range []struct {
		name      string
		diagnoses bool
		want      string
	}{{"no diagnostics", false, "EPSON0A36A8.local."}, {"neither resolves", true, "192.168.178.87,EPSON0A36A8.local."}} {
		fake, hc := house(t)
		epsonAt(t, fake, "unavailable")
		if !c.diagnoses {
			fake.SetDiagnostics("entry-ipp", map[string]any{"entry": map[string]any{"data": map[string]any{}}})
		}
		cfg := druckerCfg(hc)
		var asked []string
		cfg.Probe = func(_ context.Context, host string) erreichbar.Antwort {
			asked = append(asked, host)
			return erreichbar.Unbekannt
		}
		_, body := pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series")
		mustContain(t, body, "Drucker nicht erreichbar", "Die Adresse des Druckers ist nicht bekannt.")
		mustNotContain(t, body, "antwortet")
		if strings.Join(asked, ",") != c.want {
			t.Errorf("%s: asked %v, want %s", c.name, asked, c.want)
		}
	}
	// the .local name that does resolve (an mDNS resolver) is asked and answers
	fake, hc := house(t)
	epsonAt(t, fake, "unavailable")
	fake.SetDiagnostics("entry-ipp", map[string]any{"entry": map[string]any{"data": map[string]any{}}})
	cfg := druckerCfg(hc)
	cfg.Probe = func(context.Context, string) erreichbar.Antwort { return erreichbar.Port }
	_, body := pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series")
	mustContain(t, body, "Der Drucker antwortet im Netz, aber Home Assistant erreicht ihn nicht.")
}

// Only a printer's state sensor is checked, only from Homes' own page.
func TestDruckerCheckOnlyAPrinter(t *testing.T) {
	fake, hc := house(t)
	epsonAt(t, fake, "idle")
	fake.Set("sensor.temp", "21", nil)
	if code, _ := pruefe(t, druckerCfg(hc), "POST", "sensor.temp"); code != 404 {
		t.Errorf("another sensor: %d", code)
	}
	req := httptest.NewRequest("POST", DruckerPruefenPath+"?geraet=sensor.epson_wf_3540_series", nil)
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	DruckerPruefen(druckerCfg(hc)).ServeHTTP(rec, req)
	if rec.Code != 403 || len(fake.Calls()) != 0 {
		t.Errorf("from elsewhere: %d, calls %d", rec.Code, len(fake.Calls()))
	}
}

// pruefeForm is the row's button as a form sends it (with script, as Homes'
// script sends a form; without, the browser's own POST).
func pruefeForm(t *testing.T, cfg StartConfig, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", DruckerPruefenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	DruckerPruefen(cfg).ServeHTTP(rec, req)
	return rec
}

// The owner (2026-11-14): „einen Button auf der Karte … ‚Prüfen und neu
// verbinden‘“. Only an unreachable printer's row has it: a form to the
// check that says „Prüfe …“ on the button at once; a reachable one stays
// quiet.
func TestDruckerRowButtonOnlyWhenUnreachable(t *testing.T) {
	fake, hc := house(t)
	epsonAt(t, fake, "unavailable")
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `<form method=post action=/drucker/pruefen><input type=hidden name=geraet value="sensor.epson_wf_3540_series">`,
		`<input type=hidden name=zurueck value="/raeume#karte-buero"><button data-laeuft="Prüfe …">Prüfen und neu verbinden</button></form>`)
	fake, hc = house(t)
	epsonAt(t, fake, "idle")
	_, body, _ = familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustNotContain(t, body, "Prüfen und neu verbinden", "action=/drucker/pruefen")
}

// The row's button checks as the printer's page does and lands back on the
// card (303), whose row then says what the check found: the network's
// sentence, the button again; or, reachable again, „wieder erreichbar“
// with the black ink. A zurueck that is not Räume gets no redirect.
func TestDruckerRowButtonAnswersInTheRow(t *testing.T) {
	fastChecks(t)
	fake, hc := house(t)
	epsonAt(t, fake, "unavailable")
	cfg := druckerCfg(hc)
	cfg.Probe = func(context.Context, string) erreichbar.Antwort { return erreichbar.Nicht }
	form := url.Values{"geraet": {"sensor.epson_wf_3540_series"}, "tun": {"pruefen"}, "zurueck": {"/raeume#karte-buero"}}
	rec := pruefeForm(t, cfg, form)
	if rec.Code != 303 || rec.Header().Get("Location") != "/raeume#karte-buero" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Location"))
	}
	if calls := fake.Calls(); len(calls) != 1 || calls[0].Service != "reload_config_entry" {
		t.Fatalf("calls %+v", calls)
	}
	_, body, _ := familyPage(t, Raeume, cfg, "GET", "/raeume", "")
	mustContain(t, body, `Drucker · nicht erreichbar</a> <span class=k>Der Drucker antwortet nicht im Netz. Ist er an und im WLAN?</span>`,
		"Prüfen und neu verbinden")

	fake, hc = house(t)
	epsonAt(t, fake, "unavailable")
	cfg = druckerCfg(hc)
	go func() { // HA reaches it again while Homes waits
		time.Sleep(60 * time.Millisecond)
		epson(t, fake, "idle", 12)
		epsonAt(t, fake, "idle")
	}()
	if rec := pruefeForm(t, cfg, form); rec.Code != 303 {
		t.Fatalf("%d", rec.Code)
	}
	_, body, _ = familyPage(t, Raeume, cfg, "GET", "/raeume", "")
	mustContain(t, body, `Drucker · wieder erreichbar</a> <span class="k warn">Schwarz fast leer · 12`)
	mustNotContain(t, body, "Prüfen und neu verbinden")

	form.Set("zurueck", "https://example.org/")
	if rec := pruefeForm(t, cfg, form); rec.Code != 200 || rec.Header().Get("Location") != "" {
		t.Errorf("elsewhere: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// While a check runs, the row says so and offers no second one.
func TestDruckerRowWhileChecking(t *testing.T) {
	fake, hc := house(t)
	epsonAt(t, fake, "unavailable")
	release := make(chan struct{})
	p := starten(hc, "sensor.epson_wf_3540_series", func(*pruefung) { <-release })
	defer func() { close(release); <-p.done }()
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `Drucker · nicht erreichbar</a> <span class=k>Prüfe Verbindung …</span>`)
	mustNotContain(t, body, "Prüfen und neu verbinden")
}

// HA's IPP diagnostics answer 500 while the printer is unreachable (they
// read the coordinator's data). The host they gave while it was reachable
// is kept in HOMES_DATA/drucker.json by device and asked then, before the
// .local name; without a kept one the .local name alone.
func TestDruckerRemembersTheAddress(t *testing.T) {
	fastChecks(t)
	dir := t.TempDir()
	fake, hc := house(t)
	epsonAt(t, fake, "idle")
	cfg := druckerCfg(hc)
	cfg.Daten = dir
	pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series") // reachable: the diagnostics answer, the host is kept
	if b, err := os.ReadFile(filepath.Join(dir, "drucker.json")); err != nil || !strings.Contains(string(b), `"dev-epson": "192.168.178.87"`) {
		t.Fatalf("drucker.json %s %v", b, err)
	}

	fake, hc = house(t) // later, another start of Homes: the printer off, its diagnostics a 500
	epsonAt(t, fake, "unavailable")
	fake.SetDiagnostics("entry-ipp", nil)
	cfg = druckerCfg(hc)
	cfg.Daten = dir
	var asked []string
	cfg.Probe = func(_ context.Context, host string) erreichbar.Antwort {
		asked = append(asked, host)
		return erreichbar.Nicht
	}
	_, body := pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series")
	mustContain(t, body, "Der Drucker antwortet nicht im Netz. Ist er an und im WLAN?")
	if strings.Join(asked, ",") != "192.168.178.87" {
		t.Errorf("asked %v, want the kept host", asked)
	}

	fake, hc = house(t) // nothing kept: the .local name, not known here
	epsonAt(t, fake, "unavailable")
	fake.SetDiagnostics("entry-ipp", nil)
	cfg = druckerCfg(hc)
	cfg.Daten = t.TempDir()
	asked = nil
	cfg.Probe = func(_ context.Context, host string) erreichbar.Antwort {
		asked = append(asked, host)
		return erreichbar.Unbekannt
	}
	_, body = pruefe(t, cfg, "POST", "sensor.epson_wf_3540_series")
	mustContain(t, body, "Die Adresse des Druckers ist nicht bekannt.")
	if strings.Join(asked, ",") != "EPSON0A36A8.local." {
		t.Errorf("asked %v", asked)
	}
}

// The owner (2026-11-14): „Wird auch der Status ‚druckt‘ dargestellt?“ The
// row says the state in words, and what asks for a hand before the ink.
func TestDruckerRowSaysTheState(t *testing.T) {
	fake, hc := house(t)
	epson(t, fake, "printing", 12)
	_, body, _ := familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `Drucker · druckt</a> <span class="k warn">Schwarz fast leer · 12 %</span>`)
	fake, hc = house(t)
	epson(t, fake, "stopped", 40)
	fake.Set("sensor.epson_wf_3540_series", "stopped", map[string]any{"friendly_name": "EPSON WF-3540 Series", "device_class": "enum",
		"state_reason": "media-empty-error", "state_message": "Load paper in tray 1"})
	_, body, _ = familyPage(t, Raeume, druckerCfg(hc), "GET", "/raeume", "")
	mustContain(t, body, `Drucker · angehalten</a> <span class="k warn">Papier leer · Schwarz 40 %</span>`)
	mustNotContain(t, body, "Load paper")
}
