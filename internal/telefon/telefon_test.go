package telefon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/ha/hafake"
)

// ADR 0004 §7: each reporting phone a device in HA's mobile_app, its own
// registration per Homes device, its scans and its charger through its
// webhook; without a registration nothing is sent and the caller does as
// before.

var (
	annasPixel = Geraet{ID: "a1a1a1a1a1a1", Name: "Pixel 9", Person: "anna"}
	bensPixel  = Geraet{ID: "b2b2b2b2b2b2", Name: "Pixel 7", Person: "ben"}
	bensTablet = Geraet{ID: "b3b3b3b3b3b3", Name: "Tablet", Person: "ben"}
)

func rig(t *testing.T) (*hafake.HA, *Telefone, *time.Time) {
	t.Helper()
	fake := hafake.New("t")
	t.Cleanup(fake.Close)
	now := time.Date(2026, 11, 26, 22, 0, 0, 0, time.UTC)
	return fake, &Telefone{HA: &ha.Client{URL: fake.URL, Token: "t"}, Dir: t.TempDir(), Version: "test", Now: func() time.Time { return now }}, &now
}

func TestNames(t *testing.T) {
	for g, want := range map[Geraet]string{{Person: "michael", Name: "Pixel 9"}: "Michaels Pixel 9", {Person: "hans", Name: "Telefon"}: "Hans’ Telefon",
		{Person: "fritz", Name: "Handy"}: "Fritz’ Handy", {Person: "", Name: "Pixel"}: "Pixel"} {
		if got := Name(g); got != want {
			t.Errorf("%+v: %q", g, got)
		}
	}
}

// The first report registers the phone, under its own device_id, with the
// charger's sensors; the webhook id kept for Homes only.
func TestTheFirstReportRegistersThePhone(t *testing.T) {
	fake, tel, _ := rig(t)
	ctx := context.Background()
	if !tel.Tag(ctx, annasPixel, "homes-b0e70e000001") || !tel.Laden(ctx, annasPixel, "an", "kabellos") {
		t.Fatal("not sent")
	}
	regs := fake.Registrations()
	if len(regs) != 1 {
		t.Fatalf("registrations: %v", regs)
	}
	for k, v := range map[string]any{"device_id": "homes-a1a1a1a1a1a1", "app_id": "cloud.dopp.homes", "app_name": "Homes", "device_name": "Annas Pixel 9",
		"os_name": "Android", "supports_encryption": false, "manufacturer": "Homes", "app_version": "test"} {
		if regs[0][k] != v {
			t.Errorf("%s: %v", k, regs[0][k])
		}
	}
	calls := fake.Webhooks()
	var types []string
	for _, c := range calls {
		types = append(types, c.Type)
	}
	if strings.Join(types, " ") != "register_sensor register_sensor scan_tag update_sensor_states" {
		t.Fatalf("webhook: %v", types)
	}
	laedt := calls[0].Data.(map[string]any)
	if laedt["unique_id"] != "laedt" || laedt["device_class"] != "battery_charging" || laedt["type"] != "binary_sensor" || calls[1].Data.(map[string]any)["unique_id"] != "ladeart" {
		t.Fatalf("sensors: %+v", calls[:2])
	}
	states := calls[3].Data.([]any)
	if len(states) != 2 || states[0].(map[string]any)["state"] != true || states[1].(map[string]any)["state"] != "kabellos" {
		t.Fatalf("states: %v", states)
	}
	if ev := fake.Events(); len(ev) != 1 || ev[0].Type != "tag_scanned" || ev[0].Data["device_id"] == nil {
		t.Fatalf("HA's own scan: %+v", ev)
	}
	fi, err := os.Stat(filepath.Join(tel.Dir, "telefone.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the file: %v %v", fi, err)
	}
	// off the charger: Lädt off, Ladeart keine; a charge of no known kind leaves Ladeart
	tel.Laden(ctx, annasPixel, "aus", "")
	tel.Laden(ctx, annasPixel, "an", "")
	calls = fake.Webhooks()
	if s := calls[4].Data.([]any); s[0].(map[string]any)["state"] != false || s[1].(map[string]any)["state"] != "keine" {
		t.Fatalf("aus: %v", s)
	}
	if s := calls[5].Data.([]any); len(s) != 1 {
		t.Fatalf("an without a kind: %v", s)
	}
	if len(fake.Registrations()) != 1 {
		t.Fatal("registered again")
	}
}

// Per device: Anna's phone and each of Ben's are devices of their own,
// each its own webhook.
func TestEachPhoneIsItsOwnDevice(t *testing.T) {
	fake, tel, _ := rig(t)
	ctx := context.Background()
	for _, g := range []Geraet{annasPixel, bensPixel, bensTablet, annasPixel} {
		tel.Tag(ctx, g, "x")
	}
	regs := fake.Registrations()
	if len(regs) != 3 || regs[1]["device_name"] != "Bens Pixel 7" || regs[2]["device_name"] != "Bens Tablet" || regs[2]["device_id"] != "homes-b3b3b3b3b3b3" {
		t.Fatalf("%v", regs)
	}
	seen := map[string]string{}
	for _, c := range fake.Webhooks() {
		if c.Type == "scan_tag" {
			if was, ok := seen[c.DeviceID]; ok && was != c.ID {
				t.Fatalf("one device, two webhooks")
			}
			seen[c.DeviceID] = c.ID
		}
	}
	if len(seen) != 3 {
		t.Fatalf("%v", seen)
	}
}

// An HA without mobile_app: nothing sent (the caller does as before),
// HA not asked again for an hour.
func TestWithoutMobileAppNothingIsSent(t *testing.T) {
	fake, tel, now := rig(t)
	fake.MobileAppOff(true)
	ctx := context.Background()
	if tel.Tag(ctx, annasPixel, "x") || tel.Laden(ctx, bensPixel, "an", "usb") {
		t.Fatal("sent")
	}
	fake.MobileAppOff(false)
	if tel.Tag(ctx, annasPixel, "x") {
		t.Fatal("asked again at once")
	}
	*now = now.Add(Pause + time.Second)
	if !tel.Tag(ctx, annasPixel, "x") || len(fake.Registrations()) != 1 {
		t.Fatal("not asked again after the pause")
	}
	var none *Telefone
	if none.Tag(ctx, annasPixel, "x") {
		t.Fatal("nil sent")
	}
}

// Removed in HA: HA's 410 forgets it, the next report registers again.
func TestARegistrationRemovedInHAIsMadeAgain(t *testing.T) {
	fake, tel, _ := rig(t)
	ctx := context.Background()
	tel.Tag(ctx, annasPixel, "x")
	entries, _ := tel.HA.MobileAppEntries(ctx)
	tel.HA.DeleteConfigEntry(ctx, entries["homes-a1a1a1a1a1a1"])
	if tel.Tag(ctx, annasPixel, "x") {
		t.Fatal("sent to a removed registration")
	}
	if !tel.Tag(ctx, annasPixel, "x") || len(fake.Registrations()) != 2 {
		t.Fatalf("not registered again: %d", len(fake.Registrations()))
	}
}

// Unpaired: its registration removed in HA, its webhook forgotten; the
// hourly look removes what was unpaired meanwhile and forgets what HA lost.
func TestUnpairedIsRemovedInHA(t *testing.T) {
	fake, tel, _ := rig(t)
	ctx := context.Background()
	for _, g := range []Geraet{annasPixel, bensPixel, bensTablet} {
		tel.Tag(ctx, g, "x")
	}
	if err := tel.Entfernen(ctx, annasPixel.ID); err != nil {
		t.Fatal(err)
	}
	entries, _ := tel.HA.MobileAppEntries(ctx)
	if _, ok := entries["homes-a1a1a1a1a1a1"]; ok || len(entries) != 2 {
		t.Fatalf("entries: %v", entries)
	}
	b, _ := os.ReadFile(filepath.Join(tel.Dir, "telefone.json"))
	if strings.Contains(string(b), "a1a1a1a1a1a1") {
		t.Fatalf("kept: %s", b)
	}
	// Ben's tablet unpaired while Homes was down; Ben's phone removed in HA by hand
	tel.HA.DeleteConfigEntry(ctx, entries["homes-b2b2b2b2b2b2"])
	tel.pflegen(ctx, func(id string) bool { return id != bensTablet.ID })
	entries, _ = tel.HA.MobileAppEntries(ctx)
	b, _ = os.ReadFile(filepath.Join(tel.Dir, "telefone.json"))
	if len(entries) != 0 || strings.TrimSpace(string(b)) != "{}" {
		t.Fatalf("after: %v %s", entries, b)
	}
	if !tel.Tag(ctx, bensPixel, "x") || len(fake.Registrations()) != 4 {
		t.Fatal("ben's phone not registered again")
	}
}

// Seen 2026-11-28: HA took a registration without os_version and then
// failed to set its entry up; the webhook answered 200 and did nothing, so
// a touched tag was lost. Now a registration whose device does not appear
// counts as none (the caller fires the raw tag_scanned), and the entry
// that failed is removed before the next one.
func TestARegistrationHACannotSetUpIsNoneAndIsCleanedUp(t *testing.T) {
	fake, tel, now := rig(t)
	ctx := context.Background()
	fake.MobileAppSetupFails(true)
	if tel.Tag(ctx, annasPixel, "homes-aaaaaaaaaaaa") {
		t.Fatal("a scan through a registration HA could not set up counted as sent")
	}
	if b := fake.BrokenEntries(); len(b) != 1 || b[0] != Name(annasPixel) {
		t.Fatalf("broken entries: %v", b)
	}
	fake.MobileAppSetupFails(false)
	*now = now.Add(Pause + time.Second)
	if !tel.Tag(ctx, annasPixel, "homes-aaaaaaaaaaaa") {
		t.Fatal("not registered again once HA can set it up")
	}
	if b := fake.BrokenEntries(); len(b) != 0 {
		t.Fatalf("the failed entry stayed: %v", b)
	}
	for _, r := range fake.Registrations() {
		if r["os_version"] == "" || r["os_version"] == nil {
			t.Fatalf("registered without os_version: %v", r)
		}
	}
}
