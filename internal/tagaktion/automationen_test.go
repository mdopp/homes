package tagaktion

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/bote"
	"github.com/mdopp/homes/internal/bote/botefake"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/ha/hafake"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/laib/laibfake"
)

// ADR 0004 §8: a tag's automations, linked and loosed through HA's config
// API, a new one by a sentence to the Bote; an admin's change after their
// confirmation, anyone else's after an admin's Freigeben.

const buero = "homes-b0e70e000001"

type rig struct {
	ha   *hafake.HA
	laib *laibfake.Laib
	bote *botefake.Server
	d    *Dienst
	now  time.Time
}

var (
	michael = Wer{Login: "michael", Name: "Michael", Admin: true} // the owner
	petra   = Wer{Login: "petra", Admin: true}                    // an admin who is not the owner
	anna    = Wer{Login: "anna"}                                  // another person
)

func gutenacht() map[string]any {
	return map[string]any{"id": "1700000000001", "alias": "Gute Nacht", "triggers": []any{map[string]any{"trigger": "time", "at": "23:00:00"}},
		"actions": []any{map[string]any{"action": "light.turn_off", "target": map[string]any{"area_id": "buero"}}}}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{ha: hafake.New("t"), laib: laibfake.New(), bote: botefake.New("k"), now: time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC)}
	t.Cleanup(r.ha.Close)
	t.Cleanup(r.bote.Close)
	srv := r.laib.Server()
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	r.d = &Dienst{HA: &ha.Client{URL: r.ha.URL, Token: "t"}, Laib: &laib.Client{URL: srv.URL, Token: "x"}, Store: store,
		Bote: &bote.Client{URL: r.bote.Base(), Key: "k"}, PublicURL: "https://www.example.org", Dir: dir,
		Admins: func() []string { return []string{"michael", "petra"} }, Now: func() time.Time { return r.now }}
	r.ha.AddTag(buero, "Bürotür")
	r.ha.AddAutomation("1700000000001", "Gute Nacht", gutenacht())
	r.ha.AddAutomation("1700000000002", "Flurlicht", map[string]any{"id": "1700000000002", "alias": "Flurlicht",
		"trigger": []any{map[string]any{"platform": "tag", "tag_id": buero}}, "action": []any{}})
	return r
}

func neinIst(t *testing.T, err error, status int, has string) {
	t.Helper()
	var n *Nein
	if !errors.As(err, &n) || n.Status != status || !strings.Contains(n.Satz, has) {
		t.Fatalf("want %d %q, got %v", status, has, err)
	}
}

func TestTheTriggersOnATagAreFound(t *testing.T) {
	for name, c := range map[string]struct {
		cfg  map[string]any
		want []string
	}{
		"new style":    {map[string]any{"triggers": []any{map[string]any{"trigger": "tag", "tag_id": "a"}}}, []string{"a"}},
		"old style":    {map[string]any{"trigger": []any{map[string]any{"platform": "tag", "tag_id": "a"}}}, []string{"a"}},
		"one alone":    {map[string]any{"trigger": map[string]any{"platform": "tag", "tag_id": "a"}}, []string{"a"}},
		"a list":       {map[string]any{"triggers": []any{map[string]any{"trigger": "tag", "tag_id": []any{"a", "b"}}}}, []string{"a", "b"}},
		"an event":     {map[string]any{"triggers": []any{map[string]any{"trigger": "event", "event_type": "tag_scanned", "event_data": map[string]any{"tag_id": "c"}}}}, []string{"c"}},
		"other events": {map[string]any{"triggers": []any{map[string]any{"trigger": "event", "event_type": "homes_laden", "event_data": map[string]any{"tag_id": "c"}}}}, nil},
		"no trigger":   {map[string]any{}, nil},
	} {
		if got := tagsIn(c.cfg); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v", name, got)
		}
	}
	// taken out, another id in the same list stays
	got := ohneTag(map[string]any{"triggers": []any{map[string]any{"trigger": "tag", "tag_id": []any{"a", "b"}}, map[string]any{"trigger": "time"}}}, "a")
	if !reflect.DeepEqual(tagsIn(got), []string{"b"}) || len(got["triggers"].([]any)) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestVerbundenReadsEachAutomationsTriggers(t *testing.T) {
	r := newRig(t)
	mit, err := r.d.Verbunden(context.Background())
	if err != nil || !reflect.DeepEqual(mit[buero], []Verbindung{{"1700000000002", "Flurlicht"}}) {
		t.Fatalf("%v %v", mit, err)
	}
	all, _ := r.d.Automationen(context.Background(), "nacht")
	if len(all) != 1 || all[0].Name != "Gute Nacht" || !all[0].An {
		t.Fatalf("search: %+v", all)
	}
}

// An admin's link: first the sentence, nothing written; then written, in
// the config's own style; Lösen brings back exactly what was there.
func TestAnAdminLinksAfterConfirmingAndLoesenRestoresExactly(t *testing.T) {
	for _, admin := range []Wer{michael, petra} {
		r := newRig(t)
		w := Wunsch{Art: Verbinden, Tag: buero, Automation: "1700000000001"}
		got, err := r.d.Aendern(context.Background(), w, admin, false)
		if err != nil || !got.Bestaetigen || got.FreigabeNoetig || got.Satz != "Wenn jemand ‚Bürotür‘ berührt, startet ‚Gute Nacht‘. Was die Automation sonst auslöst, bleibt." {
			t.Fatalf("%s preview: %+v %v", admin.Login, got, err)
		}
		if !reflect.DeepEqual(r.ha.AutomationConfig("1700000000001"), gutenacht()) {
			t.Fatal("the preview wrote")
		}
		got, err = r.d.Aendern(context.Background(), w, admin, true)
		if err != nil || got.Satz != "Verbunden: ‚Bürotür‘ startet jetzt ‚Gute Nacht‘." || got.Wartet {
			t.Fatalf("%s confirmed: %+v %v", admin.Login, got, err)
		}
		trig := r.ha.AutomationConfig("1700000000001")["triggers"].([]any)
		if len(trig) != 2 || !reflect.DeepEqual(trig[1], map[string]any{"trigger": "tag", "tag_id": buero}) {
			t.Fatalf("written: %v", trig)
		}
		_, err = r.d.Aendern(context.Background(), w, admin, false)
		neinIst(t, err, http.StatusConflict, "schon")
		got, err = r.d.Aendern(context.Background(), Wunsch{Art: Loesen, Tag: buero, Automation: "1700000000001"}, admin, true)
		if err != nil || got.Satz != "Gelöst: ‚Gute Nacht‘ ist wieder genau wie vorher." || !reflect.DeepEqual(r.ha.AutomationConfig("1700000000001"), gutenacht()) {
			t.Fatalf("loesen: %+v %v %v", got, err, r.ha.AutomationConfig("1700000000001"))
		}
		if len(r.laib.Gesagte()) != 0 {
			t.Fatalf("an admin's own change told someone: %v", r.laib.Gesagte())
		}
	}
}

// Changed in HA in between: Lösen takes only the tag out, and says so.
func TestLoesenAfterAChangeTakesOnlyTheTag(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.d.Aendern(ctx, Wunsch{Art: Verbinden, Tag: buero, Automation: "1700000000001"}, michael, true)
	changed := r.ha.AutomationConfig("1700000000001")
	changed["alias"] = "Gute Nacht (22:30)"
	r.ha.AddAutomation("1700000000001", "Gute Nacht", changed)
	got, err := r.d.Aendern(ctx, Wunsch{Art: Loesen, Tag: buero, Automation: "1700000000001"}, michael, true)
	if err != nil || !strings.Contains(got.Satz, "inzwischen geändert") {
		t.Fatalf("%+v %v", got, err)
	}
	now := r.ha.AutomationConfig("1700000000001")
	if now["alias"] != "Gute Nacht (22:30)" || len(tagsIn(now)) != 0 || len(now["triggers"].([]any)) != 1 {
		t.Fatalf("after: %v", now)
	}
	// one linked in HA itself: taken out too, the old style kept
	got, err = r.d.Aendern(ctx, Wunsch{Art: Loesen, Tag: buero, Automation: "1700000000002"}, michael, true)
	if err != nil || !strings.Contains(got.Satz, "aus ‚Flurlicht‘ genommen") || r.ha.AutomationConfig("1700000000002")["trigger"] == nil {
		t.Fatalf("%+v %v %v", got, err, r.ha.AutomationConfig("1700000000002"))
	}
}

func TestAChangeRefusedInWords(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_, err := r.d.Aendern(ctx, Wunsch{Art: Verbinden, Tag: buero, Automation: "999"}, michael, false)
	neinIst(t, err, http.StatusNotFound, "gibt es in Home Assistant nicht")
	_, err = r.d.Aendern(ctx, Wunsch{Art: Loesen, Tag: buero, Automation: "1700000000001"}, michael, false)
	neinIst(t, err, http.StatusConflict, "startet nicht")
	_, err = r.d.Aendern(ctx, Wunsch{Art: NeueAutomation, Tag: buero, Satz: "  "}, michael, false)
	neinIst(t, err, http.StatusBadRequest, "einem Satz")
	r.ha.Down(true)
	_, err = r.d.Aendern(ctx, Wunsch{Art: Verbinden, Tag: buero, Automation: "1700000000001"}, michael, true)
	neinIst(t, err, http.StatusBadGateway, "Home Assistant antwortet gerade nicht")
	r.ha.Down(false)
	r.d.Bote = nil
	_, err = r.d.Aendern(ctx, Wunsch{Art: NeueAutomation, Tag: buero, Satz: "Licht an"}, michael, false)
	neinIst(t, err, http.StatusServiceUnavailable, "nicht eingerichtet")
}

// Anyone else's change waits: every admin is told with the link, it is a
// task until the first admin answers; then the person is told.
func TestAnotherPersonsChangeWaitsForTheFirstAdmin(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	w := Wunsch{Art: Verbinden, Tag: buero, Automation: "1700000000001"}
	got, _ := r.d.Aendern(ctx, w, anna, false)
	if !got.Bestaetigen || !got.FreigabeNoetig {
		t.Fatalf("preview: %+v", got)
	}
	got, err := r.d.Aendern(ctx, w, anna, true)
	if err != nil || !got.Wartet || got.Freigabe == "" || !strings.HasPrefix(got.Satz, "Wartet auf Freigabe: Anna möchte ‚Bürotür‘ mit ‚Gute Nacht‘ verbinden.") {
		t.Fatalf("%+v %v", got, err)
	}
	if !reflect.DeepEqual(r.ha.AutomationConfig("1700000000001"), gutenacht()) {
		t.Fatal("written before Freigeben")
	}
	line := "Anna möchte ‚Bürotür‘ mit ‚Gute Nacht‘ verbinden. Freigeben: https://www.example.org/tag/freigabe/" + got.Freigabe
	if s := r.laib.Gesagte(); !reflect.DeepEqual(s, []string{"michael: " + line, "petra: " + line}) {
		t.Fatalf("told: %v", s)
	}
	if o := r.d.Offen(); len(o) != 1 || o[0].ID != got.Freigabe {
		t.Fatalf("open: %+v", o)
	}
	if f, _ := r.d.Freigabe(got.Freigabe); f.Stand != Wartet || f.Satz() != "Wartet auf Freigabe durch einen Admin." {
		t.Fatalf("stand: %+v", f)
	}
	if _, _, err := r.d.Entscheiden(ctx, got.Freigabe, anna, true); err == nil {
		t.Fatal("anna decided her own")
	}
	f, _, err := r.d.Entscheiden(ctx, got.Freigabe, petra, true)
	if err != nil || f.Stand != Freigegeben || f.Durch != "petra" {
		t.Fatalf("petra: %+v %v", f, err)
	}
	if len(tagsIn(r.ha.AutomationConfig("1700000000001"))) != 1 {
		t.Fatal("not written on Freigeben")
	}
	_, _, err = r.d.Entscheiden(ctx, got.Freigabe, michael, false)
	neinIst(t, err, http.StatusConflict, "schon entschieden")
	s := r.laib.Gesagte()
	if s[len(s)-1] != "anna: Petra hat deinen Wunsch freigegeben. Verbunden: ‚Bürotür‘ startet jetzt ‚Gute Nacht‘." {
		t.Fatalf("anna told: %v", s)
	}
	if len(r.d.Offen()) != 0 {
		t.Fatal("still a task")
	}
	// Lösen by an admin afterwards restores what was there before anna's wish
	r.d.Aendern(ctx, Wunsch{Art: Loesen, Tag: buero, Automation: "1700000000001"}, michael, true)
	if !reflect.DeepEqual(r.ha.AutomationConfig("1700000000001"), gutenacht()) {
		t.Fatal("not as before")
	}
}

func TestARefusedWishIsToldAndChangesNothing(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	got, _ := r.d.Aendern(ctx, Wunsch{Art: Verbinden, Tag: buero, Automation: "1700000000001"}, anna, true)
	f, _, err := r.d.Entscheiden(ctx, got.Freigabe, michael, false)
	if err != nil || f.Stand != Abgelehnt || f.Satz() != "Michael hat es nicht freigegeben." {
		t.Fatalf("%+v %v", f, err)
	}
	if !reflect.DeepEqual(r.ha.AutomationConfig("1700000000001"), gutenacht()) {
		t.Fatal("written")
	}
	if s := r.laib.Gesagte(); !strings.HasPrefix(s[len(s)-1], "anna: Michael hat deinen Wunsch nicht freigegeben: ") {
		t.Fatalf("%v", s)
	}
}

// Nothing waits longer than 7 days: then it lapses, the person is told,
// and an admin can no longer answer it.
func TestAWishLapsesAfterSevenDays(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	got, _ := r.d.Aendern(ctx, Wunsch{Art: Verbinden, Tag: buero, Automation: "1700000000001"}, anna, true)
	r.now = r.now.Add(Frist - time.Minute)
	r.d.pflegen(ctx)
	if len(r.d.Offen()) != 1 {
		t.Fatal("gone too early")
	}
	r.now = r.now.Add(2 * time.Minute)
	if f, _ := r.d.Freigabe(got.Freigabe); f.Stand != Verfallen || len(r.d.Offen()) != 0 {
		t.Fatalf("read after the Frist: %+v", f)
	}
	_, _, err := r.d.Entscheiden(ctx, got.Freigabe, michael, true)
	neinIst(t, err, http.StatusConflict, "verfallen")
	r.d.pflegen(ctx)
	r.d.pflegen(ctx) // told once
	n := 0
	for _, s := range r.laib.Gesagte() {
		if strings.HasPrefix(s, "anna: Dein Wunsch ist nach 7 Tagen") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("anna told %d times: %v", n, r.laib.Gesagte())
	}
	r.now = r.now.Add(behalten)
	r.d.pflegen(ctx)
	if _, ok := r.d.Freigabe(got.Freigabe); ok {
		t.Fatal("kept for ever")
	}
}

// A new automation: an admin's sentence goes to the Bote as theirs, with
// the tag's id; anyone else's to the admins first.
func TestANewAutomationGoesToTheBote(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	w := Wunsch{Art: NeueAutomation, Tag: buero, Satz: "Licht im Flur an"}
	got, err := r.d.Aendern(ctx, w, michael, true)
	if err != nil || got.Bote != "Erledigt: das Licht im Flur ist an." || !strings.Contains(got.Satz, "Sprechen-Feld") {
		t.Fatalf("%+v %v", got, err)
	}
	seen := r.bote.Seen()
	if len(seen) != 1 || seen[0].SessionKey != "homes:michael" || !strings.Contains(seen[0].Messages()[1]["content"], "tag_id: "+buero) ||
		!strings.Contains(seen[0].Messages()[1]["content"], "Licht im Flur an") {
		t.Fatalf("the Bote got: %+v", seen)
	}
	got, _ = r.d.Aendern(ctx, w, anna, true)
	if !got.Wartet || len(r.bote.Seen()) != 1 {
		t.Fatalf("anna's went on: %+v", got)
	}
	f, antwort, err := r.d.Entscheiden(ctx, got.Freigabe, petra, true)
	if err != nil || antwort == "" || f.Satz() != "Petra richtet die Automation mit Homes ein." || r.bote.Seen()[1].SessionKey != "homes:petra" {
		t.Fatalf("%+v %q %v", f, antwort, err)
	}
}

func TestTheBoteFailingOrLateIsSaid(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	w := Wunsch{Art: NeueAutomation, Tag: buero, Satz: "Licht an"}
	r.bote.Status = http.StatusBadGateway
	_, err := r.d.Aendern(ctx, w, michael, true)
	neinIst(t, err, http.StatusBadGateway, "antwortet gerade nicht")
	r.bote.Status, r.bote.Hang = 0, time.Second
	defer func(was time.Duration) { boteWarte = was }(boteWarte)
	boteWarte = 50 * time.Millisecond
	_, err = r.d.Aendern(ctx, w, michael, true)
	neinIst(t, err, http.StatusGatewayTimeout, "nicht rechtzeitig")
	// a wish whose Freigeben failed stays waiting, for another try or Ablehnen
	got, _ := r.d.Aendern(ctx, w, anna, true)
	if _, _, err := r.d.Entscheiden(ctx, got.Freigabe, michael, true); err == nil {
		t.Fatal("no error")
	}
	if f, _ := r.d.Freigabe(got.Freigabe); f.Stand != Wartet {
		t.Fatalf("%+v", f)
	}
}
