package napi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/laib/laibfake"
	"github.com/mdopp/homes/internal/tagaktion"
)

// ADR 0004 §8 for the app: a tag's automations, the change previewed,
// an admin's written, anyone else's waiting for an admin, its state polled.

const nachtID = "1700000000001"

// tagAutomationen: dev1 anna, dev2 ben (other people), devM michael (the
// owner, an admin), devP petra (an admin who is not the owner).
func tagAutomationen(t *testing.T) (*laibfake.Laib, http.Handler, func(string) map[string]any) {
	t.Helper()
	lf := laibfake.New()
	srv := lf.Server()
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	store, _ := tagaktion.Open(dir)
	q := &einrichten.Quellen{Admins: einrichten.OpenAdmins(dir)}
	q.SahAdmin("michael", true)
	q.SahAdmin("petra", true)
	q.SahAdmin("anna", false)
	fake, h := houseWith(t, func(c *Config) {
		c.Laib = &laib.Client{URL: srv.URL, Token: "ops"}
		c.Einrichtung = q
		c.Tags = &tagaktion.Dienst{HA: c.HA, Laib: c.Laib, Store: store, Dir: dir, PublicURL: "https://www.example.org", Admins: q.AdminListe, LaufWarte: 300 * time.Millisecond}
		c.Whoami = func(b string) (string, bool) {
			p, ok := map[string]string{"dev1": "anna", "dev2": "ben", "devM": "michael", "devP": "petra"}[b]
			return p, ok
		}
		c.Validate = StaticValidator([]string{"dev1", "dev2", "devM", "devP"})
	})
	fake.AddTag("homes-b0e70e000001", "Bürotür")
	fake.AddAutomation(nachtID, "Gute Nacht", map[string]any{"id": nachtID, "alias": "Gute Nacht", "triggers": []any{}, "actions": []any{}})
	fake.AddAutomation("1700000000002", "Flurlicht", map[string]any{"id": "1700000000002", "alias": "Flurlicht",
		"triggers": []any{map[string]any{"trigger": "tag", "tag_id": "homes-b0e70e000001"}}})
	return lf, h, func(id string) map[string]any { return fake.AutomationConfig(id) }
}

func TestTheTagsListTheirAutomations(t *testing.T) {
	_, h, _ := tagAutomationen(t)
	_, body := raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", "")
	if !strings.Contains(body, `"automationen":[{"id":"1700000000002","name":"Flurlicht"}],"name":"Bürotür","tag":"homes-b0e70e000001"`) {
		t.Fatalf("%s", body)
	}
	_, body = raw(t, h, "GET", "/napi/automationen?q=NACHT", "dev2", "")
	if body != `{"automationen":[{"an":true,"id":"1700000000001","name":"Gute Nacht"}],"ok":true}`+"\n" {
		t.Fatalf("%q", body)
	}
}

// Per viewer: the owner's and another admin's change is written once
// confirmed; another person's waits, and only they and the admins read its state.
func TestVerbindenPerViewer(t *testing.T) {
	for _, admin := range []string{"devM", "devP"} {
		_, h, cfg := tagAutomationen(t)
		code, body := raw(t, h, "POST", "/napi/ausloeser/tag/homes-b0e70e000001/verbinden", admin, `{"automation":"`+nachtID+`"}`)
		if m := reply(t, body); code != 200 || m["bestaetigen"] != true || m["freigabe_noetig"] != nil || !strings.Contains(m["satz"].(string), "startet ‚Gute Nacht‘") {
			t.Fatalf("%s preview: %d %s", admin, code, body)
		}
		if len(cfg(nachtID)["triggers"].([]any)) != 0 {
			t.Fatal("the preview wrote")
		}
		code, body = raw(t, h, "POST", "/napi/ausloeser/tag/homes-b0e70e000001/verbinden", admin, `{"automation":"`+nachtID+`","bestaetigt":true}`)
		if m := reply(t, body); code != 200 || m["satz"] != "Verbunden: ‚Bürotür‘ startet jetzt ‚Gute Nacht‘." || m["wartet"] != nil {
			t.Fatalf("%s: %d %s", admin, code, body)
		}
		if len(cfg(nachtID)["triggers"].([]any)) != 1 {
			t.Fatal("not written")
		}
		code, body = raw(t, h, "POST", "/napi/ausloeser/tag/homes-b0e70e000001/loesen", admin, `{"automation":"`+nachtID+`","bestaetigt":true}`)
		if code != 200 || !strings.Contains(body, "wieder genau wie vorher") || len(cfg(nachtID)["triggers"].([]any)) != 0 {
			t.Fatalf("loesen: %d %s", code, body)
		}
	}
	lf, h, cfg := tagAutomationen(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag/homes-b0e70e000001/verbinden", "dev1", `{"automation":"`+nachtID+`"}`)
	if m := reply(t, body); code != 200 || m["freigabe_noetig"] != true {
		t.Fatalf("anna's preview: %s", body)
	}
	code, body = raw(t, h, "POST", "/napi/ausloeser/tag/homes-b0e70e000001/verbinden", "dev1", `{"automation":"`+nachtID+`","bestaetigt":true}`)
	m := reply(t, body)
	id, _ := m["freigabe"].(string)
	if code != 200 || m["wartet"] != true || id == "" || len(cfg(nachtID)["triggers"].([]any)) != 0 {
		t.Fatalf("anna: %d %s", code, body)
	}
	if len(lf.Gesagte()) != 2 {
		t.Fatalf("the admins told: %v", lf.Gesagte())
	}
	for dev, want := range map[string]int{"dev1": 200, "devM": 200, "devP": 200, "dev2": 404} {
		code, body := raw(t, h, "GET", "/napi/ausloeser/freigabe/"+id, dev, "")
		if code != want || want == 200 && reply(t, body)["stand"] != "wartet" {
			t.Errorf("%s reads the wish: %d %s", dev, code, body)
		}
	}
}

func TestAendernRefusedInWords(t *testing.T) {
	_, h, _ := tagAutomationen(t)
	for _, c := range []struct {
		path, body string
		code       int
		has        string
	}{
		{"/napi/ausloeser/tag/homes-b0e70e000001/verbinden", `{"automation":"999"}`, 404, "nicht (mehr)"},
		{"/napi/ausloeser/tag/homes-b0e70e000001/verbinden", `{"automation":"1700000000002"}`, 409, "schon"},
		{"/napi/ausloeser/tag/homes-b0e70e000001/loesen", `{"automation":"` + nachtID + `"}`, 409, "startet nicht"},
		{"/napi/ausloeser/tag/homes-b0e70e000001/neu", `{"satz":"Licht an"}`, 503, "nicht eingerichtet"},
		{"/napi/ausloeser/tag/kein%20tag/verbinden", `{}`, 400, "nicht lesbar"},
		{"/napi/ausloeser/tag/homes-b0e70e000001/verbinden", `nope`, 400, "nicht lesbar"},
	} {
		code, body := raw(t, h, "POST", c.path, "devM", c.body)
		if code != c.code || reply(t, body)["ok"] != false || !strings.Contains(reply(t, body)["error"].(string), c.has) {
			t.Errorf("%s %s: %d %s", c.path, c.body, code, body)
		}
	}
	if code, _ := raw(t, h, "GET", "/napi/ausloeser/freigabe/nichts", "devM", ""); code != 404 {
		t.Errorf("an unknown wish: %d", code)
	}
}

// A new tag carries Homes' own address (§5); a scanner tag that asks
// says where to go on.
func TestTheTagsAddressAndScannerFragen(t *testing.T) {
	_, h, _ := tagAutomationen(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Scanner","aktion":"scanner-fragen"}`)
	m := reply(t, body)
	id, _ := m["tag"].(string)
	if code != 200 || m["uri"] != "https://www.example.org/tag/"+id || m["aktion"] != "scanner-fragen" {
		t.Fatalf("%d %s", code, body)
	}
	_, body = raw(t, h, "POST", "/napi/ausloeser", "dev2", `{"art":"nfc","tag":"`+id+`"}`)
	if m := reply(t, body); m["oeffnen"] != "https://www.example.org/dokumente#scannen" || m["aktion"] != "scanner-fragen" || m["text"] == "" {
		t.Fatalf("%s", body)
	}
}
