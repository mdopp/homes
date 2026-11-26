package napi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/ha/hafake"
)

// ADR 0004 §1: the phone reports, Homes fires it on HA's bus as the
// person's, and decides nothing.

func triggers(t *testing.T) (*hafake.HA, http.Handler) {
	t.Helper()
	return houseWith(t, func(c *Config) {
		c.Whoami = func(b string) (string, bool) {
			return map[string]string{"dev1": "anna", "dev2": "ben"}[b], b == "dev1" || b == "dev2"
		}
	})
}

func reply(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("not JSON: %q", body)
	}
	return m
}

func TestAusloeserWantsAPairedDevice(t *testing.T) {
	fake, h := triggers(t)
	for _, p := range []struct{ method, path, body string }{
		{"POST", "/napi/ausloeser", `{"art":"nfc","tag":"x"}`},
		{"POST", "/napi/ausloeser/tag", `{"name":"Tür"}`},
		{"GET", "/napi/ausloeser/tags", ""},
	} {
		if code, _ := raw(t, h, p.method, p.path, "", p.body); code != http.StatusUnauthorized {
			t.Errorf("%s %s without a device: %d", p.method, p.path, code)
		}
	}
	if len(fake.Events()) != 0 {
		t.Fatalf("fired without a device: %v", fake.Events())
	}
}

func TestNFCTagFiresHAsTagScannedAsThePerson(t *testing.T) {
	fake, h := triggers(t)
	fake.AddTag("homes-0123456789ab", "Bürotür")
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"homes-0123456789ab"}`)
	if code != http.StatusOK || reply(t, body)["name"] != "Bürotür" || reply(t, body)["ok"] != true {
		t.Fatalf("%d %s", code, body)
	}
	ev := fake.Events()
	if len(ev) != 1 || ev[0].Type != "tag_scanned" || ev[0].Data["tag_id"] != "homes-0123456789ab" ||
		ev[0].Data["person"] != "anna" || len(ev[0].Data) != 2 {
		t.Fatalf("events: %+v", ev)
	}
}

func TestATagHAsCompanionAppWroteIsFiredTooAndSaidUnnamed(t *testing.T) {
	fake, h := triggers(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev2", `{"art":"nfc","tag":"5f0ba733-172f-430d-a7f8-e4ad940c88d7"}`)
	if code != http.StatusOK || reply(t, body)["name"] != "" {
		t.Fatalf("%d %s", code, body)
	}
	if ev := fake.Events(); len(ev) != 1 || ev[0].Data["person"] != "ben" {
		t.Fatalf("events: %+v", ev)
	}
}

func TestChargerFiresHomesLaden(t *testing.T) {
	fake, h := triggers(t)
	for _, z := range []string{"an", "aus"} {
		code, body := raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"laden","zustand":"`+z+`","ladeart":"kabellos"}`)
		if code != http.StatusOK || reply(t, body)["ok"] != true {
			t.Fatalf("%s: %d %s", z, code, body)
		}
	}
	ev := fake.Events()
	if len(ev) != 2 || ev[0].Type != LadenEvent || ev[0].Data["zustand"] != "an" || ev[1].Data["zustand"] != "aus" ||
		ev[0].Data["ladeart"] != "kabellos" || ev[0].Data["person"] != "anna" || len(ev[0].Data) != 3 {
		t.Fatalf("events: %+v", ev)
	}
}

func TestAusloeserRefusesInWords(t *testing.T) {
	fake, h := triggers(t)
	for _, body := range []string{
		`not json`,
		`{"art":"nfc","tag":""}`,
		`{"art":"nfc","tag":"a/b"}`,
		`{"art":"nfc","tag":"` + strings.Repeat("a", 65) + `"}`,
		`{"art":"laden","zustand":"halb"}`,
		`{"art":"laden","zustand":"an","ladeart":"solar"}`,
		`{"art":"licht"}`,
	} {
		code, got := raw(t, h, "POST", "/napi/ausloeser", "dev1", body)
		m := reply(t, got)
		if code != http.StatusBadRequest || m["ok"] != false || m["error"] == "" || strings.Contains(got, "json") {
			t.Errorf("%s: %d %s", body, code, got)
		}
	}
	if len(fake.Events()) != 0 {
		t.Fatalf("fired on a refusal: %v", fake.Events())
	}
}

func TestAusloeserWhenHAIsDownOrRefuses(t *testing.T) {
	fake, h := triggers(t)
	fake.Down(true)
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"laden","zustand":"an","ladeart":""}`)
	if code != http.StatusBadGateway || !strings.Contains(reply(t, body)["error"].(string), "antwortet gerade nicht") {
		t.Fatalf("down: %d %s", code, body)
	}
	fake.Down(false)
	fake.AssistAdmin(false)
	code, body = raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"t1"}`)
	if code != http.StatusBadGateway || !strings.Contains(reply(t, body)["error"].(string), "Admin") || strings.Contains(body, "401") {
		t.Fatalf("not admin: %d %s", code, body)
	}
}

func TestANewTagIsNamedInHA(t *testing.T) {
	fake, h := triggers(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"  Bürotür "}`)
	m := reply(t, body)
	id, _ := m["tag"].(string)
	if code != http.StatusOK || !tagID.MatchString(id) || !strings.HasPrefix(id, "homes-") || len(id) != len("homes-")+12 ||
		m["uri"] != "cloud.dopp.homes://tag/"+id {
		t.Fatalf("%d %s", code, body)
	}
	code, body = raw(t, h, "GET", "/napi/ausloeser/tags", "dev2", "")
	var list struct {
		OK   bool                `json:"ok"`
		Tags []map[string]string `json:"tags"`
	}
	json.Unmarshal([]byte(body), &list)
	if code != http.StatusOK || !list.OK || len(list.Tags) != 1 || list.Tags[0]["tag"] != id || list.Tags[0]["name"] != "Bürotür" {
		t.Fatalf("%d %s", code, body)
	}
	if _, again := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Küche"}`); reply(t, again)["tag"] == id {
		t.Fatal("two tags, one id")
	}
	_ = fake
}

func TestANewTagWantsAName(t *testing.T) {
	_, h := triggers(t)
	for _, body := range []string{`{"name":"  "}`, `{"name":"` + strings.Repeat("ä", 61) + `"}`, `{}`} {
		if code, got := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", body); code != http.StatusBadRequest || !strings.Contains(got, "Namen") {
			t.Errorf("%s: %d %s", body, code, got)
		}
	}
}

func TestTagsListIsEmptyNotNull(t *testing.T) {
	_, h := triggers(t)
	if code, body := raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", ""); code != http.StatusOK || !strings.Contains(body, `"tags":[]`) {
		t.Fatalf("%d %s", code, body)
	}
}

// ADR 0004 §2: a tag is named where it is in the person's hand, once, in HA.

func TestAnUnknownIdIsNamedNotRewritten(t *testing.T) {
	fake, h := triggers(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Bett","tag":"5f0ba733-172f"}`)
	if m := reply(t, body); code != http.StatusOK || m["tag"] != "5f0ba733-172f" || m["uri"] != "cloud.dopp.homes://tag/5f0ba733-172f" {
		t.Fatalf("%d %s", code, body)
	}
	if _, body = raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"5f0ba733-172f"}`); reply(t, body)["name"] != "Bett" {
		t.Fatalf("touched after naming: %s", body)
	}
	_ = fake
}

func TestRenameKeepsTheId(t *testing.T) {
	fake, h := triggers(t)
	fake.AddTag("homes-aaaaaaaaaaaa", "Bürotür")
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"aktion":"","name":"Büro Tür","tag":"homes-aaaaaaaaaaaa"}`)
	if code != http.StatusOK || reply(t, body)["tag"] != "homes-aaaaaaaaaaaa" {
		t.Fatalf("%d %s", code, body)
	}
	_, body = raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", "")
	if !strings.Contains(body, `{"aktion":"","name":"Büro Tür","tag":"homes-aaaaaaaaaaaa"}`) || strings.Count(body, `"tag":"`) != 1 {
		t.Fatalf("after rename: %s", body)
	}
	// its own name again, in another case, is no clash with itself
	if code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"büro tür","tag":"homes-aaaaaaaaaaaa"}`); code != http.StatusOK {
		t.Fatalf("own name: %d %s", code, body)
	}
}

func TestANameIsUniqueCaseAside(t *testing.T) {
	fake, h := triggers(t)
	fake.AddTag("homes-aaaaaaaaaaaa", "Bürotür")
	fake.AddTag("homes-bbbbbbbbbbbb", "Bett")
	for _, body := range []string{`{"name":" bürotür "}`, `{"name":"BÜROTÜR","tag":"homes-bbbbbbbbbbbb"}`} {
		code, got := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", body)
		if m := reply(t, got); code != http.StatusConflict || m["ok"] != false || !strings.Contains(m["error"].(string), "„Bürotür“") {
			t.Errorf("%s: %d %s", body, code, got)
		}
	}
	if _, body := raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", ""); strings.Count(body, `"tag":"`) != 2 || !strings.Contains(body, `"name":"Bett"`) {
		t.Fatalf("a refused name changed the registry: %s", body)
	}
}

func TestRemoveATag(t *testing.T) {
	fake, h := triggers(t)
	fake.AddTag("homes-aaaaaaaaaaaa", "Bürotür")
	if code, body := raw(t, h, "DELETE", "/napi/ausloeser/tag/homes-aaaaaaaaaaaa", "dev1", ""); code != http.StatusOK || reply(t, body)["ok"] != true {
		t.Fatalf("%d %s", code, body)
	}
	if _, body := raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", ""); !strings.Contains(body, `"tags":[]`) {
		t.Fatalf("still there: %s", body)
	}
	code, body := raw(t, h, "DELETE", "/napi/ausloeser/tag/homes-aaaaaaaaaaaa", "dev1", "")
	if m := reply(t, body); code != http.StatusNotFound || m["ok"] != false || !strings.Contains(m["error"].(string), "nicht mehr") {
		t.Fatalf("twice: %d %s", code, body)
	}
	if code, _ := raw(t, h, "DELETE", "/napi/ausloeser/tag/homes-aaaaaaaaaaaa", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("without a device: %d", code)
	}
}

func TestTheNameFieldSuggestsTheRoomsThenPlaces(t *testing.T) {
	fake, h := triggers(t)
	fake.SetRegistries([]map[string]any{{"area_id": "buero", "name": "Büro"}, {"area_id": "bad", "name": "Bad"}}, nil, nil)
	_, body := raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", "")
	var m struct {
		Vorschlaege []string `json:"vorschlaege"`
	}
	json.Unmarshal([]byte(body), &m)
	if strings.Join(m.Vorschlaege, ",") != "Bad,Büro,Tür,Bett,Schreibtisch,Ladestation" {
		t.Fatalf("%s", body)
	}
}
