package tagaktion

import (
	"context"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/app"
	"github.com/mdopp/homes/internal/bote"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/laib"
)

// Dienst is what a tag does, one for the app (napi) and the page /tag/<id>.
type Dienst struct {
	HA        *ha.Client
	Laib      *laib.Client // the scan order while HA has no scanner button, and tell; nil: no scanner, nobody told
	Store     *Store       // which tag does what of Homes' own; nil: every tag reports to HA
	Bote      *bote.Client // a new automation by a sentence (§8); nil: none
	PublicURL string       // HOMES_PUBLIC_URL: the tag's address, the links in a wish
	Dir       string       // HOMES_DATA: the configs kept for Lösen, the wishes waiting
	Admins    func() []string
	Now       func() time.Time
	LaufWarte time.Duration // how long a report waits for its automations' runs (§9); 0: 4 s
	// PersonVon is whose phone an HA device is (a scan's device_id, §7);
	// nil, or "": none said
	PersonVon func(ctx context.Context, haDevice string) string

	vmu    sync.Mutex  // the configs kept for Lösen
	fmu    sync.Mutex  // the wishes
	laeuft sync.Map    // the wishes an admin's Freigeben is being written for
	spuren spurenCache // HA's runs, read for what came of a touch (§9)
}

// Nein is a refusal in the person's words, with the status the app gets.
type Nein struct {
	Status int
	Satz   string
}

func (n *Nein) Error() string { return n.Satz }

func nein(status int, satz string) error { return &Nein{status, satz} }

// Als is err as a refusal: a Nein as it is, anything else as Homes'
// failure in plain words (its detail goes to the log).
func Als(err error) *Nein {
	var n *Nein
	if errors.As(err, &n) {
		return n
	}
	log.Printf("homes: tag: %v", err)
	return &Nein{http.StatusInternalServerError, "Das hat nicht geklappt. Versuch es gleich noch einmal."}
}

// HANein is a failed HA call in the person's words; HA's detail goes to the log.
func HANein(was string, err error) *Nein {
	log.Printf("homes: ausloeser %s: %v", was, err)
	var he *ha.Error
	if errors.Is(err, ha.ErrNotAdmin) || errors.As(err, &he) && he.Status == http.StatusUnauthorized {
		return &Nein{http.StatusBadGateway, "Home Assistant lässt Homes das nicht tun. Bitte sag dem Admin Bescheid."}
	}
	return &Nein{http.StatusBadGateway, "Home Assistant antwortet gerade nicht. Versuch es gleich noch einmal."}
}

func (d *Dienst) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// URI is what a new tag carries (§5): Homes' own address for it, an
// Android App Link; without HOMES_PUBLIC_URL the app's scheme (§1).
func (d *Dienst) URI(tag string) string {
	if u := strings.TrimRight(d.PublicURL, "/"); u != "" {
		return u + "/tag/" + tag
	}
	return app.Scheme + "://tag/" + tag
}

// Scanner says whether a tag can do the scanner's actions here.
func (d *Dienst) Scanner() bool { return d != nil && d.Laib != nil && d.Store != nil }

// Aktionen are the actions this box offers a tag: HA's always, the
// scanner's with Laib and the store.
func (d *Dienst) Aktionen() []map[string]string {
	out := []map[string]string{}
	for _, t := range Titel {
		if t.ID == "" || d.Scanner() {
			out = append(out, map[string]string{"id": t.ID, "titel": t.Titel})
		}
	}
	return out
}

// Von is tag's action, "" for HA's.
func (d *Dienst) Von(tag string) string {
	if !d.Scanner() {
		return ""
	}
	return d.Store.Von(tag)
}

// Name is the tag's name in HA, "" for one HA does not know or when HA
// does not answer (the name is a courtesy).
func (d *Dienst) Name(ctx context.Context, tag string) string {
	tags, err := d.HA.Tags(ctx)
	if err != nil {
		return ""
	}
	for _, t := range tags {
		if t.ID == tag {
			return t.Name
		}
	}
	return ""
}

// Ergebnis is what touching a tag did.
type Ergebnis struct {
	Name    string // the tag's name in HA, "" for one it does not know
	Aktion  string // its action of Homes' own, "" for HA's
	Text    string // what the phone says; "" for HA's (the app says „gemeldet“)
	Oeffnen string // where it leads on: Homes' Scannen for scanner-fragen
	Laeufe  []Lauf // what the tag's automations made of it (§9); nil: none trigger on it
}

// scanLogin is a person the scan service takes (box.yml people, lower case),
// as the Scannen buttons send it.
var scanLogin = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// The scanner's buttons in HA (§6), by unique_id.
const (
	KnopfHaushalt = "laib-scan-haushalt"
	KnopfPerson   = "laib-scan-person-" // + the login
)

// Ausloesen is a tag touched by person: its action of Homes' own, else
// HA's tag_scanned (§1). „Für mich“ is always the toucher's, never the
// tag's maker's (§4).
func (d *Dienst) Ausloesen(ctx context.Context, tag, person string) (Ergebnis, error) {
	return d.AusloesenMit(ctx, tag, person, nil)
}

// AusloesenMit is Ausloesen from a phone that is a device in HA (§7): a
// tag that reports to HA is scan's, HA's own scan by that device; only
// when scan did not send it (nil, or false) the raw tag_scanned — never
// both, as HA's scan fires tag_scanned itself.
func (d *Dienst) AusloesenMit(ctx context.Context, tag, person string, scan func(context.Context, string) bool) (Ergebnis, error) {
	akt := d.Von(tag)
	if akt == "" {
		seit := time.Now()
		if scan == nil || !scan(ctx, tag) {
			if err := d.HA.Fire(ctx, "tag_scanned", map[string]any{"tag_id": tag, "person": person}); err != nil {
				return Ergebnis{}, HANein("nfc", err)
			}
		}
		out := Ergebnis{Name: d.Name(ctx, tag), Laeufe: d.Nachher(ctx, tag, seit)}
		if len(out.Laeufe) > 0 {
			out.Text = Zeile(out.Name, out.Laeufe)
		}
		return out, nil
	}
	out := Ergebnis{Aktion: akt}
	if akt == ScannerFragen {
		out.Text, out.Oeffnen = "Wähle auf Homes, für wen gescannt wird.", strings.TrimRight(d.PublicURL, "/")+"/dokumente#scannen"
	} else {
		text, err := d.scannen(ctx, akt, person)
		if err != nil {
			return Ergebnis{}, err
		}
		out.Text = text
	}
	out.Name = d.Name(ctx, tag)
	return out, nil
}

// scannen starts a stack: HA's scanner button when HA has it (§6), else
// Laib's scan order as the Scannen buttons send it (§4), so a box without
// Laib's §6 keeps scanning.
func (d *Dienst) scannen(ctx context.Context, akt, person string) (string, error) {
	if !scanLogin.MatchString(person) {
		person = ""
	}
	knopf, fuer, text := KnopfHaushalt, "", "Der Scanner startet – für den Haushalt."
	if akt == ScannerIch {
		if person == "" {
			return "", nein(http.StatusForbidden, "Für dich allein scannt nur ein Telefon, das mit deinem Login gekoppelt ist.")
		}
		knopf, fuer, text = KnopfPerson+person, person, "Der Scanner startet – nur für dich."
	}
	if reg, ok := d.HA.CachedRegistry(ctx); ok && reg.UniqueIDs[knopf] != "" {
		e := reg.UniqueIDs[knopf]
		if _, err := d.HA.Call(ctx, "button", "press", map[string]any{"entity_id": e}); err != nil {
			return "", HANein("scanner "+e, err)
		}
		log.Printf("homes: tag %s for %q: pressed %s in Home Assistant", akt, person, e)
		return text, nil
	}
	log.Printf("homes: tag %s for %q: Home Assistant has no button %s (ADR 0004 §6, Laib's, not on this box yet): laib scan", akt, person, knopf)
	if _, err := d.Laib.ScanOrder(ctx, "start", person, fuer); err != nil {
		log.Printf("homes: ausloeser scannen: %v", err)
		if errors.Is(err, laib.ErrNoScan) {
			return "", nein(http.StatusServiceUnavailable, "Auf dieser Box ist kein Scanner eingerichtet.")
		}
		return "", nein(http.StatusBadGateway, "Der Scanner antwortet gerade nicht. Versuch es gleich noch einmal.")
	}
	return text, nil
}
