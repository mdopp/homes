package pages

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/ha/hafake"
)

// the owner's living room (2026-11-12): a JBL Bar through Cast and its
// Music Assistant twin (no area, as Music Assistant's players have none),
// a Voice PE's own speaker; a kitchen Chromecast Music Assistant does not
// play on. An own label on the bar's Music Assistant player must stay.
// With sonos, a second speaker Music Assistant plays on in the living room
// (in its area itself): only then is there a choice (owner 2026-11-13).
func standardHouse(t *testing.T, sonos bool) (*hafake.HA, *ha.Client) {
	t.Helper()
	fake, hc := house(t)
	entities := []map[string]any{
		{"entity_id": "media_player.jbl_bar_300_ecdf", "device_id": "dev-bar", "platform": "cast"},
		{"entity_id": "media_player.jbl_bar_300_ecdf_2", "device_id": "dev-bar-ma", "platform": "music_assistant", "labels": []any{"musik"}},
		{"entity_id": "media_player.home_assistant_voice_0907c9_media_player", "device_id": "dev-pe", "platform": "esphome"},
		{"entity_id": "media_player.kuche", "device_id": "dev-kueche", "platform": "cast"},
		{"entity_id": "light.sofa", "area_id": "wohnzimmer"},
		{"entity_id": "light.kueche", "area_id": "kueche"},
	}
	names := map[string]string{"media_player.jbl_bar_300_ecdf": "JBL Bar 300", "media_player.jbl_bar_300_ecdf_2": "JBL Bar 300",
		"media_player.home_assistant_voice_0907c9_media_player": "Voice PE", "media_player.kuche": "Küche"}
	if sonos {
		entities = append(entities, map[string]any{"entity_id": "media_player.sonos_one", "area_id": "wohnzimmer", "platform": "music_assistant"})
		names["media_player.sonos_one"] = "Sonos One"
	}
	fake.SetRegistries(
		[]map[string]any{{"area_id": "wohnzimmer", "name": "Wohnzimmer"}, {"area_id": "kueche", "name": "Küche"}},
		[]map[string]any{
			{"id": "dev-bar", "area_id": "wohnzimmer", "identifiers": [][]string{{"cast", "ecdf3b687070104cf22c168061d9a3d1"}}},
			{"id": "dev-bar-ma", "identifiers": [][]string{{"music_assistant", "ecdf3b68-7070-104c-f22c-168061d9a3d1"}}},
			{"id": "dev-pe", "area_id": "wohnzimmer", "identifiers": [][]string{{"esphome", "0907c9"}}},
			{"id": "dev-kueche", "area_id": "kueche", "identifiers": [][]string{{"cast", "cf400ad00cab64f2adba0627bb6683d0"}}},
		}, entities)
	fake.SetLabels([]map[string]any{{"label_id": "musik", "name": "Musik"}})
	for id, name := range names {
		fake.Set(id, "idle", map[string]any{"friendly_name": name})
	}
	fake.Set("light.sofa", "off", map[string]any{"friendly_name": "Sofalicht"})
	fake.Set("light.kueche", "off", map[string]any{"friendly_name": "Küchenlicht"})
	return fake, hc
}

// standardPost is an admin's (or anyone's) choice, sent from Geräte itself.
func standardPost(t *testing.T, cfg StartConfig, user, groups string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/raeume", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Remote-User", user)
	req.Header.Set("Remote-Groups", groups)
	rec := httptest.NewRecorder()
	Raeume(cfg).ServeHTTP(rec, req)
	return rec
}

// labels is an entity's labels in the fake's registry now.
func labels(fake *hafake.HA, id string) any { return fake.Entity(id)["labels"] }

// owner 2026-11-12: a room's standard speaker, chosen on Geräte by an
// admin (the household's, not one person's), kept as HA's label
// „Standard-Lautsprecher“ on the Music Assistant player, and shown to
// everyone once chosen; owner 2026-11-13: one quiet last row of the room's
// card, and only where there is a choice. Per viewer: the box's owner (an
// admin), another person, an admin who is not the owner.
func TestRaeumeStandardSpeakerPerViewer(t *testing.T) {
	fake, hc := standardHouse(t, true)
	cfg := StartConfig{HA: hc}
	row := `<li class="tu standard"><div>` + lautsprecherIcon + `<span class=was>Musik spielt auf</span>`
	choice := `</div><form method=post action="/raeume#karte-wohnzimmer-standard"><input type=hidden name=raum value="Wohnzimmer">` +
		`<select id=karte-wohnzimmer-standard name=standard aria-label="Musik im Raum Wohnzimmer spielt auf" onchange="this.setAttribute('aria-busy','true');this.form.requestSubmit()">`
	for _, admin := range []struct{ user, groups string }{{"mdopp", "family,admins"}, {"ben", "admins"}} {
		_, body, _ := familyPageAs(t, Raeume, cfg, "GET", "/raeume", admin.user, admin.groups)
		mustContain(t, body, row+choice+`<option value="">automatisch</option><option value="media_player.jbl_bar_300_ecdf_2">JBL Bar 300</option>`+
			`<option value="media_player.sonos_one">Sonos One</option></select><noscript><button>Speichern</button></noscript></form></li></ul></section>`)
		mustNotContain(t, body, `value="media_player.home_assistant_voice_0907c9_media_player"`, `value="media_player.jbl_bar_300_ecdf"`, `name=raum value="Küche"`,
			`<h2 id=karte-wohnzimmer>Wohnzimmer</h2><form`, "Gespeichert")
	}
	_, family, _ := familyPageAs(t, Raeume, cfg, "GET", "/raeume", "anna", "family")
	mustNotContain(t, family, "Musik spielt auf", "<select name=standard")

	// anna may not choose; nothing changes in HA
	if rec := standardPost(t, cfg, "anna", "family", url.Values{"raum": {"Wohnzimmer"}, "standard": {"media_player.jbl_bar_300_ecdf_2"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("a family member's choice: %d", rec.Code)
	}
	if got := labels(fake, "media_player.jbl_bar_300_ecdf_2"); !reflect.DeepEqual(got, []any{"musik"}) {
		t.Fatalf("labels after a refused choice: %v", got)
	}

	// ben, an admin, chooses the bar: the label is made, put on Music Assistant's player with its own kept, and that
	// player put in the room, so the voice finds it by the satellite's area; the row says it was saved
	rec := standardPost(t, cfg, "ben", "admins", url.Values{"raum": {"Wohnzimmer"}, "standard": {"media_player.jbl_bar_300_ecdf_2"}})
	if rec.Code != 200 {
		t.Fatalf("choosing: %d %s", rec.Code, rec.Body)
	}
	mustContain(t, rec.Body.String(), row+`<span class=gespeichert role=status>Gespeichert</span>`+choice,
		`<option value="media_player.jbl_bar_300_ecdf_2" selected>JBL Bar 300</option>`)
	e := fake.Entity("media_player.jbl_bar_300_ecdf_2")
	if !reflect.DeepEqual(e["labels"], []any{"musik", "standard_lautsprecher"}) || e["area_id"] != "wohnzimmer" {
		t.Fatalf("the bar's Music Assistant player: %v", e)
	}
	_, family, _ = familyPageAs(t, Raeume, cfg, "GET", "/raeume", "anna", "family")
	mustContain(t, family, row+`</div><span class=wahl>JBL Bar 300</span></li></ul></section>`)
	mustNotContain(t, family, "<select", "Gespeichert")

	// automatisch takes the label off again, the player's own stays
	rec = standardPost(t, cfg, "mdopp", "family,admins", url.Values{"raum": {"Wohnzimmer"}, "standard": {""}})
	mustContain(t, rec.Body.String(), "Gespeichert", `<option value="">automatisch</option><option value="media_player.jbl_bar_300_ecdf_2">`)
	if got := labels(fake, "media_player.jbl_bar_300_ecdf_2"); !reflect.DeepEqual(got, []any{"musik"}) {
		t.Fatalf("labels after automatisch: %v", got)
	}
	_, family, _ = familyPageAs(t, Raeume, cfg, "GET", "/raeume", "anna", "family")
	mustNotContain(t, family, "Musik spielt auf")
}

// One speaker Music Assistant plays on is the room's anyway: no row, for
// an admin or anyone, even with the label on it; none at all, no row
// either (the kitchen's Chromecast); two, the row (above).
func TestRaeumeStandardSpeakerOnlyWithAChoice(t *testing.T) {
	fake, hc := standardHouse(t, false)
	cfg := StartConfig{HA: hc}
	if rec := standardPost(t, cfg, "ben", "admins", url.Values{"raum": {"Wohnzimmer"}, "standard": {"media_player.jbl_bar_300_ecdf_2"}}); rec.Code != 200 {
		t.Fatalf("choosing the one: %d %s", rec.Code, rec.Body)
	}
	if got := labels(fake, "media_player.jbl_bar_300_ecdf_2"); !reflect.DeepEqual(got, []any{"musik", "standard_lautsprecher"}) {
		t.Fatalf("labels: %v", got)
	}
	for _, v := range []struct{ user, groups string }{{"mdopp", "family,admins"}, {"ben", "admins"}, {"anna", "family"}} {
		_, body, _ := familyPageAs(t, Raeume, cfg, "GET", "/raeume", v.user, v.groups)
		mustContain(t, body, `<h2 id=karte-wohnzimmer>Wohnzimmer</h2>`, `<h2 id=karte-kueche>Küche</h2>`)
		mustNotContain(t, body, "Musik spielt auf", "name=standard", "Standard-Lautsprecher")
	}
}

// What cannot be the standard is refused in a sentence, HA unchanged: a
// speaker Music Assistant does not play on (the voice could not play
// there), a room that is not one, a post from elsewhere.
func TestRaeumeStandardSpeakerRefuses(t *testing.T) {
	fake, hc := standardHouse(t, true)
	cfg := StartConfig{HA: hc}
	for _, c := range []struct {
		form url.Values
		code int
		says string
	}{
		{url.Values{"raum": {"Wohnzimmer"}, "standard": {"media_player.home_assistant_voice_0907c9_media_player"}}, http.StatusConflict, "Diesen Lautsprecher gibt es in diesem Raum nicht mehr."},
		{url.Values{"raum": {"Küche"}, "standard": {"media_player.kuche"}}, http.StatusConflict, "Diesen Lautsprecher gibt es in diesem Raum nicht mehr."},
		{url.Values{"raum": {"Keller"}, "standard": {""}}, http.StatusConflict, "Diesen Lautsprecher gibt es in diesem Raum nicht mehr."},
	} {
		rec := standardPost(t, cfg, "ben", "admins", c.form)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.says) {
			t.Errorf("%v: %d %q", c.form, rec.Code, rec.Body)
		}
	}
	req := httptest.NewRequest("POST", "/raeume", strings.NewReader("raum=Wohnzimmer&standard=media_player.jbl_bar_300_ecdf_2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Remote-User", "ben")
	req.Header.Set("Remote-Groups", "admins")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	Raeume(cfg).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a post from elsewhere: %d", rec.Code)
	}
	if got := labels(fake, "media_player.jbl_bar_300_ecdf_2"); !reflect.DeepEqual(got, []any{"musik"}) {
		t.Errorf("labels after refusals: %v", got)
	}
	// HA not answering: a sentence, no page that pretends
	_, hc2 := standardHouse(t, true)
	hc2.URL = "http://127.0.0.1:1"
	if rec := standardPost(t, StartConfig{HA: hc2}, "ben", "admins", url.Values{"raum": {"Wohnzimmer"}, "standard": {""}}); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("HA away: %d %q", rec.Code, rec.Body)
	}
}
