package napi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/ha/hafake"
	"github.com/mdopp/homes/internal/tagaktion"
	"github.com/mdopp/homes/internal/telefon"
)

// ADR 0004 §7 for the app's reports: a phone that is a device in HA scans
// as HA's own scan_tag (never the raw tag_scanned too) and its charger is
// its sensors beside homes_laden; without a registration, as before.

func telefone(t *testing.T) (*hafake.HA, http.Handler) {
	t.Helper()
	geraete := map[string]telefon.Geraet{"dev1": {ID: "a1a1a1a1a1a1", Name: "Pixel 9", Person: "anna"}, "dev2": {ID: "b2b2b2b2b2b2", Name: "Pixel 7", Person: "ben"}}
	dir := t.TempDir()
	store, _ := tagaktion.Open(dir)
	return houseWith(t, func(c *Config) {
		c.Whoami = func(b string) (string, bool) { g, ok := geraete[b]; return g.Person, ok }
		c.Geraet = func(b string) (telefon.Geraet, bool) { g, ok := geraete[b]; return g, ok }
		c.Telefone = &telefon.Telefone{HA: c.HA, Dir: dir, Version: "test"}
		c.Tags = &tagaktion.Dienst{HA: c.HA, Laib: c.Laib, Store: store, Dir: dir, LaufWarte: 300 * time.Millisecond}
	})
}

func TestATouchedTagIsHAsOwnScanByThePhone(t *testing.T) {
	fake, h := telefone(t)
	fake.AddTag("homes-b0e70e000001", "Bürotür")
	for _, dev := range []string{"dev1", "dev2"} {
		code, body := raw(t, h, "POST", "/napi/ausloeser", dev, `{"art":"nfc","tag":"homes-b0e70e000001"}`)
		if code != 200 || reply(t, body)["name"] != "Bürotür" {
			t.Fatalf("%s: %d %s", dev, code, body)
		}
	}
	ev := fake.Events()
	if len(ev) != 2 || ev[0].Data["person"] != nil || ev[0].Data["device_id"] == ev[1].Data["device_id"] {
		t.Fatalf("one scan each, by each phone's device, none raw: %+v", ev)
	}
	if regs := fake.Registrations(); len(regs) != 2 || regs[0]["device_name"] != "Annas Pixel 9" || regs[1]["device_name"] != "Bens Pixel 7" {
		t.Fatalf("%v", regs)
	}
}

func TestTheChargerIsTheDevicesSensorsAndHomesLaden(t *testing.T) {
	fake, h := telefone(t)
	if code, body := raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"laden","zustand":"an","ladeart":"kabellos"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if ev := fake.Events(); len(ev) != 1 || ev[0].Type != LadenEvent || ev[0].Data["person"] != "anna" {
		t.Fatalf("homes_laden: %+v", ev)
	}
	calls := fake.Webhooks()
	if last := calls[len(calls)-1]; last.Type != "update_sensor_states" || last.DeviceID != "homes-a1a1a1a1a1a1" {
		t.Fatalf("%+v", calls)
	}
}

// HA without mobile_app: the answer as before, the raw event with the person.
func TestWithoutMobileAppTheRawEvent(t *testing.T) {
	fake, h := telefone(t)
	fake.MobileAppOff(true)
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev2", `{"art":"nfc","tag":"homes-b0e70e000001"}`)
	if code != 200 || reply(t, body)["ok"] != true {
		t.Fatalf("%d %s", code, body)
	}
	if ev := fake.Events(); len(ev) != 1 || ev[0].Data["person"] != "ben" || ev[0].Data["tag_id"] != "homes-b0e70e000001" {
		t.Fatalf("%+v", ev)
	}
	if code, _ := raw(t, h, "POST", "/napi/ausloeser", "dev2", `{"art":"laden","zustand":"aus","ladeart":""}`); code != 200 || len(fake.Webhooks()) != 0 {
		t.Fatalf("laden: %d %v", code, fake.Webhooks())
	}
}

// A tag with an action of Homes' own is not HA's scan.
func TestAScannerTagIsNoScanInHA(t *testing.T) {
	fake, h := telefone(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Scanner","aktion":"scanner-fragen"}`)
	id, _ := reply(t, body)["tag"].(string)
	raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"`+id+`"}`)
	if code != 200 || len(fake.Webhooks()) != 0 || len(fake.Events()) != 0 {
		t.Fatalf("%d %v %v", code, fake.Webhooks(), fake.Events())
	}
}

// ADR 0004 §9: the answer says what the tag's automations made of the
// touch, from HA's traces; the tags' list each tag's last run.
func TestTheAnswerSaysWhatCameOfIt(t *testing.T) {
	fake, h := telefone(t)
	fake.AddTag("homes-b0e70e000001", "Büro")
	fake.AddAutomation("1700000000005", "Büro und Bambi aus (nachts)", map[string]any{"id": "1700000000005",
		"triggers": []any{map[string]any{"trigger": "tag", "tag_id": "homes-b0e70e000001"}}, "conditions": []any{map[string]any{"condition": "time", "after": "23:00:00", "before": "05:00:00"}}})
	fake.RunsOnTag("1700000000005", "failed_conditions", "condition/0")
	fake.MobileAppOff(true) // the raw event, with the person
	_, body := raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"homes-b0e70e000001"}`)
	if want := `"ergebnisse":[{"automation":"1700000000005","grund":"nur zwischen 23 und 5 Uhr","lief":false,"name":"Büro und Bambi aus (nachts)"}],` +
		`"name":"Büro","ok":true,"text":"Büro: ‚Büro und Bambi aus (nachts)‘ nicht ausgeführt – nur zwischen 23 und 5 Uhr"}`; !strings.HasSuffix(strings.TrimSpace(body), want) {
		t.Fatalf("%s", body)
	}
	_, body = raw(t, h, "GET", "/napi/ausloeser/tags", "dev2", "")
	if !strings.Contains(body, `"zuletzt":{"person":"anna","text":"‚Büro und Bambi aus (nachts)‘ nicht ausgeführt – nur zwischen 23 und 5 Uhr","wann":"20`) {
		t.Fatalf("%s", body)
	}
	// nothing from HA in time: said so
	fake.RunsOnTag("1700000000005", "", "")
	fake.AddAutomation("1700000000006", "Später", map[string]any{"id": "1700000000006", "triggers": []any{map[string]any{"trigger": "tag", "tag_id": "homes-b0e70e000001"}}})
	_, body = raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"homes-b0e70e000001"}`)
	if !strings.Contains(body, `{"automation":"1700000000006","grund":"noch nicht gemeldet","lief":false,"name":"Später"}`) {
		t.Fatalf("%s", body)
	}
}
