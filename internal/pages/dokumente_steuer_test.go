package pages

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/paperless"
	"github.com/mdopp/homes/internal/paperless/paperlessfake"
)

// steuerCfg is a Paperless after dokumente's ordnen marked what counts
// for the tax return; today is 9 November 2026, so last year is 2025.
func steuerCfg(t *testing.T) (*paperlessfake.Server, StartConfig) {
	f := paperlessfake.New("pl")
	t.Cleanup(f.Close)
	f.DocType(1, "Rechnung")
	f.Tag(10, "Haus")
	f.Tag(12, "steuerlich")
	f.Correspondent(20, "Malerbetrieb Beispiel")
	f.Correspondent(21, "Kinderhilfe e. V.")
	f.Field(37, "Betrag", "monetary")
	f.Field(40, "Steuer-Art", "select")
	f.Field(41, "Steuerjahr", "integer")
	f.Field(42, "Lohnkosten", "monetary")
	f.Options(40, "Handwerkerleistungen", "Spenden")
	f.Add(
		paperlessfake.Doc{ID: 1, Title: "Anstrich Wohnzimmer", DocumentType: 1, Correspondent: 20, Tags: []int{10, 12}, Created: "2025-03-14",
			Fields: map[int]any{37: "EUR1600.00", 40: "opt-Handwerkerleistungen", 41: 2025, 42: "EUR480.00"}},
		paperlessfake.Doc{ID: 2, Title: "Fenster", DocumentType: 1, Correspondent: 20, Tags: []int{12}, Created: "2025-06-01",
			Fields: map[int]any{37: "EUR150.00", 40: "opt-Handwerkerleistungen", 42: "EUR90.50"}}, // no Steuerjahr: its year
		paperlessfake.Doc{ID: 3, Title: "Zuwendungsbestätigung", Correspondent: 21, Tags: []int{12}, Created: "2026-01-10",
			Fields: map[int]any{37: "EUR100.00", 40: "opt-Spenden", 41: 2025}}, // made in 2026, for 2025
		paperlessfake.Doc{ID: 4, Title: "Heizungswartung 2024", DocumentType: 1, Tags: []int{12}, Created: "2025-01-05",
			Fields: map[int]any{37: "EUR200.00", 40: "opt-Handwerkerleistungen", 41: 2024}},
		paperlessfake.Doc{ID: 5, Title: "Stromrechnung", DocumentType: 1, Tags: []int{10}, Created: "2025-02-01", Fields: map[int]any{37: "EUR84.50"}},
	)
	return f, StartConfig{Docs: &paperless.Client{URL: f.URL, Token: "pl"}, PaperlessPublicURL: "https://paperless.box.test",
		Now: func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) }}
}

func getSteuer(cfg StartConfig, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Remote-User", "anna")
	rec := httptest.NewRecorder()
	DokumenteSteuer(cfg).ServeHTTP(rec, req)
	return rec
}

// owner 2026-11-09: last year's documents for the tax return, by
// Steuer-Art with a sum each and the Lohnkosten for §35a, the year
// switched with ‹ ›, one button for the tax adviser.
func TestDokumenteSteuer(t *testing.T) {
	_, cfg := steuerCfg(t)
	page := getSteuer(cfg, "/dokumente/steuer").Body.String()
	mustContain(t, page, "<h1>Für die Steuer</h1>", `<a href="/dokumente/steuer?jahr=2024" aria-label="2024">‹</a><b>2025</b>`,
		`<a href="/dokumente/steuer?jahr=2026" aria-label="2026">›</a>`, `href="/dokumente/steuer?jahr=2025&amp;zip=1" download>Für den Steuerberater herunterladen</a>`,
		"Handwerkerleistungen · 1.750,00 €", "davon Lohnkosten 570,50 €", "Spenden · 100,00 €", "Anstrich Wohnzimmer", "Fenster", "Zuwendungsbestätigung")
	before(t, page, "Handwerkerleistungen · ", "Spenden · ")
	for _, not := range []string{"Heizungswartung", "Stromrechnung"} {
		if strings.Contains(page, not) {
			t.Errorf("%s is not 2025's, or not marked:\n%s", not, page)
		}
	}
	page = getSteuer(cfg, "/dokumente/steuer?jahr=2024").Body.String()
	mustContain(t, page, "<b>2024</b>", "Handwerkerleistungen · 200,00 €", "Heizungswartung 2024")
	page = getSteuer(cfg, "/dokumente/steuer?jahr=2026").Body.String()
	mustContain(t, page, "<b>2026</b>", "Für 2026 ist nichts als steuerlich markiert.")
	if strings.Contains(page, "›") || strings.Contains(page, "herunterladen") {
		t.Errorf("no next year after this one, nothing to download:\n%s", page)
	}
	home := getDokumente(cfg, "/dokumente", map[string]string{"X-Forwarded-For": "192.168.1.23"}).Body.String()
	mustContain(t, home, `<a href="/dokumente/steuer">Für die Steuer</a>`)
}

// Paperless away: said in the family's words, no download offered.
func TestDokumenteSteuerFehler(t *testing.T) {
	f, cfg := steuerCfg(t)
	f.Status = http.StatusInternalServerError
	page := getSteuer(cfg, "/dokumente/steuer?jahr=2025&zip=1").Body.String()
	mustContain(t, page, "Paperless antwortet gerade nicht.")
	if strings.Contains(page, "herunterladen") {
		t.Errorf("a download without the list:\n%s", page)
	}
	cfg.Docs = nil
	mustContain(t, getSteuer(cfg, "/dokumente/steuer").Body.String(), "Die Dokumente sind hier noch nicht eingerichtet.")
}

// The download: each file in its Steuer-Art's folder and the list a
// German spreadsheet opens; a file Paperless no longer hands out is a
// row that says so.
func TestDokumenteSteuerZip(t *testing.T) {
	f, cfg := steuerCfg(t)
	getSteuer(cfg, "/dokumente/steuer") // the list, kept a minute
	f.Remove(2)
	rec := getSteuer(cfg, "/dokumente/steuer?jahr=2025&zip=1")
	if ct, cd := rec.Header().Get("Content-Type"), rec.Header().Get("Content-Disposition"); ct != "application/zip" || cd != `attachment; filename="Steuer-2025.zip"` {
		t.Fatalf("headers: %q %q", ct, cd)
	}
	z, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range z.File {
		r, _ := e.Open()
		b, _ := io.ReadAll(r)
		files[e.Name] = string(b)
	}
	if got := files["Handwerkerleistungen/2025-03-14 Malerbetrieb Beispiel - Anstrich Wohnzimmer (1).pdf"]; got != "%PDF-1.4 Anstrich Wohnzimmer" {
		t.Errorf("the painter's invoice: %q of %v", got, files)
	}
	if _, ok := files["Spenden/2026-01-10 Kinderhilfe e. V. - Zuwendungsbestätigung (3).pdf"]; !ok || len(files) != 3 {
		t.Errorf("the files: %v", files)
	}
	want := "\xEF\xBB\xBFDatum;Absender;Titel;Steuer-Art;Betrag;Lohnkosten;Datei\n" +
		"14.03.2025;Malerbetrieb Beispiel;Anstrich Wohnzimmer;Handwerkerleistungen;1600,00;480,00;Handwerkerleistungen/2025-03-14 Malerbetrieb Beispiel - Anstrich Wohnzimmer (1).pdf\n" +
		"01.06.2025;Malerbetrieb Beispiel;Fenster;Handwerkerleistungen;150,00;90,50;nicht geladen, bitte in Paperless öffnen\n" +
		"10.01.2026;Kinderhilfe e. V.;Zuwendungsbestätigung;Spenden;100,00;;Spenden/2026-01-10 Kinderhilfe e. V. - Zuwendungsbestätigung (3).pdf\n"
	if got := files["Steuer-2025.csv"]; got != want {
		t.Errorf("the list:\n%s\nwant:\n%s", got, want)
	}
}
