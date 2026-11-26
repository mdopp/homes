// Package telefon makes each paired phone that reports a device in Home
// Assistant (homes ADR 0004 §7): a registration with HA's mobile_app
// integration under Homes' HA user, the open API HA's own app speaks, named
// after the person and the phone („Michaels Pixel 9“). Through the
// registration's webhook a touched tag is HA's own scan (scan_tag: HA
// records when and by which device) and the charger two sensors of that
// device, „Lädt“ and „Ladeart“, with HA's history. The app's requests do
// not change.
//
// A phone is registered on its first report, not when it is paired: only
// a phone that reports becomes a device, a phone paired before this
// existed registers too, and pairing does not wait for HA. Without a
// registration (HA without mobile_app, HA refusing or silent) the caller
// does as before: the raw tag_scanned. The webhook ids are secrets, kept in
// HOMES_DATA/telefone.json (0600) and never logged; a phone unpaired is its
// registration removed in HA.
package telefon

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/app"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/jsonfile"
)

// Geraet is one paired phone: its id in Homes, its name, its person.
type Geraet struct {
	ID, Name, Person string
}

// Telefone are the registrations.
type Telefone struct {
	HA      *ha.Client
	Dir     string // HOMES_DATA
	Version string // Homes' version, the registration's app_version
	Now     func() time.Time

	mu        sync.Mutex
	gesagt    bool      // the fallback said in the log, until a registration works again
	bisWieder time.Time // after a refusal, HA is not asked again before
}

// Pause is how long Homes waits after HA refused a registration.
const Pause = time.Hour

// eintrag is one registration kept.
type eintrag struct {
	Webhook  string `json:"webhook_id"`
	Name     string `json:"name"`
	Sensoren bool   `json:"sensoren"` // Lädt and Ladeart registered
}

// DeviceID is the phone's device_id in HA: stable per Homes device.
func DeviceID(id string) string { return "homes-" + id }

// Name is the device's name in HA: „Michaels Pixel 9“.
func Name(g Geraet) string {
	p := strings.TrimSpace(g.Person)
	if r, size := utf8.DecodeRuneInString(p); size > 0 {
		p = string(unicode.ToUpper(r)) + p[size:]
	}
	last, _ := utf8.DecodeLastRuneInString(p)
	switch {
	case p == "":
	case strings.ContainsRune("sxzß", unicode.ToLower(last)):
		p += "’"
	default:
		p += "s"
	}
	return strings.TrimSpace(p + " " + strings.TrimSpace(g.Name))
}

func (t *Telefone) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

func (t *Telefone) pfad() string { return filepath.Join(t.Dir, "telefone.json") }

// laden reads the registrations; mu is held.
func (t *Telefone) laden() map[string]eintrag {
	m := map[string]eintrag{}
	if b, err := os.ReadFile(t.pfad()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (t *Telefone) sichern(m map[string]eintrag) {
	if err := jsonfile.Save(t.pfad(), m); err != nil {
		log.Printf("homes: telefone: %v", err)
	}
}

// Sensors are the charger's, as the webhook registers them.
var sensoren = []map[string]any{
	{"type": "binary_sensor", "unique_id": "laedt", "name": "Lädt", "device_class": "battery_charging", "state": false, "icon": "mdi:battery-charging"},
	{"type": "sensor", "unique_id": "ladeart", "name": "Ladeart", "state": "keine", "icon": "mdi:power-plug-off"},
}

// webhook is g's registration, made now if it has none; false: none (the
// caller does as before).
func (t *Telefone) webhook(ctx context.Context, g Geraet) (string, bool) {
	if t == nil || t.HA == nil || t.Dir == "" || g.ID == "" {
		return "", false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.laden()
	e, ok := m[g.ID]
	if !ok {
		if t.now().Before(t.bisWieder) {
			return "", false
		}
		w, err := t.HA.RegisterMobileApp(ctx, ha.Registrierung{DeviceID: DeviceID(g.ID), AppID: app.Paket, AppName: "Homes", AppVersion: t.Version,
			DeviceName: Name(g), Manufacturer: "Homes", Model: "Homes-App", OSName: "Android"})
		if err != nil {
			t.bisWieder = t.now().Add(Pause)
			if !t.gesagt {
				t.gesagt = true
				log.Printf("homes: telefon: Home Assistant takes no registration (%v): tags as tag_scanned, the charger as homes_laden only, asked again in %v", err, Pause)
			}
			return "", false
		}
		t.gesagt = false
		e = eintrag{Webhook: w, Name: Name(g)}
		log.Printf("homes: telefon: %s registered in Home Assistant as %q", DeviceID(g.ID), e.Name)
	}
	if !e.Sensoren {
		e.Sensoren = true
		for _, s := range sensoren {
			if err := t.HA.MobileWebhook(ctx, e.Webhook, "register_sensor", s); err != nil {
				log.Printf("homes: telefon: %s: registering %s: %v", DeviceID(g.ID), s["unique_id"], err)
				e.Sensoren = false
			}
		}
	}
	m[g.ID] = e
	t.sichern(m)
	return e.Webhook, true
}

// vergessen drops g's registration once HA said it is gone; the next
// report registers it again.
func (t *Telefone) vergessen(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.laden()
	delete(m, id)
	t.sichern(m)
}

// senden is one command through g's webhook; false: not sent.
func (t *Telefone) senden(ctx context.Context, g Geraet, typ string, data any) bool {
	w, ok := t.webhook(ctx, g)
	if !ok {
		return false
	}
	err := t.HA.MobileWebhook(ctx, w, typ, data)
	if errors.Is(err, ha.ErrGone) {
		log.Printf("homes: telefon: %s was removed in Home Assistant: registered again on its next report", DeviceID(g.ID))
		t.vergessen(g.ID)
		return false
	}
	if err != nil {
		log.Printf("homes: telefon: %s: %s: %v", DeviceID(g.ID), typ, err)
		return false
	}
	return true
}

// Tag is a touched tag as HA's own scan of g; false: not sent, the caller
// fires the raw tag_scanned (not both: scan_tag fires it in HA).
func (t *Telefone) Tag(ctx context.Context, g Geraet, tag string) bool {
	return t.senden(ctx, g, "scan_tag", map[string]any{"tag_id": tag})
}

// ladeIcon is Ladeart's picture per value.
var ladeIcon = map[string]string{"kabellos": "mdi:battery-charging-wireless", "kabel": "mdi:power-plug", "usb": "mdi:usb", "keine": "mdi:power-plug-off"}

// Laden is a charger report as g's sensors: Lädt, and Ladeart (keine once
// it is off; a charge of no known kind leaves it as it was).
func (t *Telefone) Laden(ctx context.Context, g Geraet, zustand, ladeart string) bool {
	an := zustand == "an"
	states := []map[string]any{{"type": "binary_sensor", "unique_id": "laedt", "state": an, "icon": "mdi:battery-charging"}}
	if !an {
		ladeart = "keine"
	}
	if ladeart != "" {
		states = append(states, map[string]any{"type": "sensor", "unique_id": "ladeart", "state": ladeart, "icon": ladeIcon[ladeart]})
	}
	return t.senden(ctx, g, "update_sensor_states", states)
}

// Entfernen removes the phone id's registration in HA (its config entry,
// with its device and sensors) and forgets its webhook; one HA never had is
// only forgotten.
func (t *Telefone) Entfernen(ctx context.Context, id string) error {
	if t == nil || t.HA == nil || t.Dir == "" {
		return nil
	}
	entries, err := t.HA.MobileAppEntries(ctx)
	if err != nil {
		return err
	}
	if entry, ok := entries[DeviceID(id)]; ok {
		if err := t.HA.DeleteConfigEntry(ctx, entry); err != nil && !errors.Is(err, ha.ErrNotFound) {
			return err
		}
		log.Printf("homes: telefon: %s removed from Home Assistant", DeviceID(id))
	}
	t.vergessen(id)
	return nil
}

// Pflegen looks every tick at what is kept: a phone no longer paired
// (gibt false: unpaired while Homes was down) is removed in HA, one removed
// in HA is forgotten (HA answers an unknown webhook with an empty 200, so
// a lost registration is seen here) and registered again on its next report.
func (t *Telefone) Pflegen(ctx context.Context, every time.Duration, gibt func(id string) bool) {
	tk := time.NewTicker(every)
	defer tk.Stop()
	for {
		t.pflegen(ctx, gibt)
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
	}
}

func (t *Telefone) pflegen(ctx context.Context, gibt func(id string) bool) {
	t.mu.Lock()
	m := t.laden()
	t.mu.Unlock()
	if len(m) == 0 {
		return
	}
	entries, err := t.HA.MobileAppEntries(ctx)
	if err != nil {
		log.Printf("homes: telefone: the device registry: %v", err)
		return
	}
	for id := range m {
		switch _, inHA := entries[DeviceID(id)]; {
		case !gibt(id):
			if err := t.Entfernen(ctx, id); err != nil {
				log.Printf("homes: telefon: removing %s: %v", DeviceID(id), err)
			}
		case !inHA:
			log.Printf("homes: telefon: %s is no longer in Home Assistant: registered again on its next report", DeviceID(id))
			t.vergessen(id)
		}
	}
}
