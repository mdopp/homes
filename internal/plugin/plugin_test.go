package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The contract's example (testdata/vertrag-v1.json) is contract 1, and
// every field of it decodes: a field renamed here breaks this test before
// it breaks a plugin. The archive's own contract test checks its answer
// against the same field names (mdopp/archiv apps/archiv/homes_test.go).
func TestVertragV1(t *testing.T) {
	b, err := os.ReadFile("testdata/vertrag-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var a Answer
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil {
		t.Fatal(err)
	}
	if err := a.pruefen(); err != nil || a.Aufgaben == nil || len(*a.Aufgaben) != 2 || a.Start == nil || len(a.Widgets) != 1 {
		t.Fatalf("%v %+v", err, a)
	}
	for _, bad := range []string{
		`{"vertrag":2,"name":"X","link":"https://x.test"}`,                                                           // a newer contract
		`{"vertrag":1,"link":"https://x.test"}`,                                                                      // no name
		`{"vertrag":1,"name":"X","link":"http://x.test"}`,                                                            // not https
		`{"vertrag":1,"name":"X","link":"https://x.test","aufgaben":[{"id":"a","titel":"t","link":"javascript:x"}]}`, // a task's link
		`{"vertrag":1,"name":"X","link":"https://x.test","aufgaben":[{"id":"Mit Namen","titel":"t"}]}`,               // an id is a word, never a name
		`{"vertrag":1,"name":"X","link":"https://x.test","aufgaben":[{"id":"a","titel":"t","art":"jetzt"}]}`,
		`{"vertrag":1,"name":"X","link":"https://x.test","aufgaben":[{"id":"a","titel":""}]}`,
		`{"vertrag":1,"name":"X","link":"https://x.test","icon":"<svg>"}`,
		`{"vertrag":1,"name":"X","link":"https://x.test","aufgaben":[` + strings.Repeat(`{"id":"a","titel":"t"},`, maxAufgaben) + `{"id":"a","titel":"t"}]}`,
	} {
		var a Answer
		if json.Unmarshal([]byte(bad), &a) == nil && a.pruefen() == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

// Laib's homes template renders HOMES_PLUGIN_<NAME> (instances:): one
// address per person's instance, each with its own key, or one address
// for everyone; anything else is left out.
func TestFromEnv(t *testing.T) {
	got := FromEnv([]string{
		"HOMES_PLUGIN_ARCHIV=anna=http://host.containers.internal:18540 max-moritz=http://host.containers.internal:18541/",
		"HOMES_PLUGINKEY_ARCHIV_ANNA=k-anna", "HOMES_PLUGINKEY_ARCHIV_MAX_MORITZ=k-mm",
		"HOMES_PLUGIN_ABSTIMMUNG=http://host.containers.internal:9000",
		"HOMES_PLUGIN_KAPUTT=anna=ftp://x", "HOMES_PLUGIN_klein=http://x", "HOMES_PLUGINKEY_ARCHIV=nie", "PATH=/bin",
	})
	want := []Instance{{"abstimmung", "", "http://host.containers.internal:9000", ""},
		{"archiv", "anna", "http://host.containers.internal:18540", "k-anna"},
		{"archiv", "max-moritz", "http://host.containers.internal:18541", "k-mm"}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if FromEnv(nil) != nil {
		t.Error("no plugin registered, yet some")
	}
}

// fake is a plugin that serves answer at Path for key and person anna.
func fake(t *testing.T, answer *string, asked *atomic.Int32) Instance {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if r.URL.Path != Path || r.Header.Get(Header) != "k" || r.URL.Query().Get("person") != "anna" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(*answer))
	}))
	t.Cleanup(srv.Close)
	return Instance{"archiv", "anna", srv.URL, "k"}
}

// Handed-over tasks stay, with their text, while the plugin cannot say
// (no aufgaben: the vault locked) and while it does not answer (Still);
// they are replaced when it says anew, an empty list too. Other persons
// see nothing of anna's instance and are never asked for.
func TestLesenKeepsTheHandedOver(t *testing.T) {
	var asked atomic.Int32
	answer := `{"vertrag":1,"name":"Archiv","link":"https://a.test","aufgaben":[{"id":"f","titel":"Wie steht Ben zu Clara?"}]}`
	file := filepath.Join(t.TempDir(), "plugins.json")
	p := Open([]Instance{fake(t, &answer, &asked)}, file)
	t0 := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	now := t0
	p.Now = func() time.Time { return now }
	ctx := context.Background()

	if s := p.Lesen(ctx, "anna"); len(s) != 1 || !s[0].Jetzt || len(*s[0].Aufgaben) != 1 || !s[0].Am.Equal(t0) {
		t.Fatalf("%+v", s)
	}
	answer = `{"vertrag":1,"name":"Archiv","link":"https://a.test"}` // locked
	now = t0.Add(time.Hour)
	if s := p.Lesen(ctx, "anna"); len(s) != 1 || s[0].Jetzt || (*s[0].Aufgaben)[0].Titel != "Wie steht Ben zu Clara?" || !s[0].Am.Equal(t0) {
		t.Fatalf("locked: %+v", s)
	}
	answer = "{" // broken
	if s := p.Lesen(ctx, "anna"); len(s) != 1 || !s[0].Still || len(*s[0].Aufgaben) != 1 {
		t.Fatalf("broken: %+v", s)
	}
	if g := Open([]Instance{{"archiv", "anna", "http://127.0.0.1:1", "k"}}, file).Gemerkt("anna"); len(g) != 1 || !g[0].Still || len(*g[0].Aufgaben) != 1 {
		t.Fatalf("after a restart: %+v", g)
	}
	answer = `{"vertrag":1,"name":"Archiv","link":"https://a.test","aufgaben":[]}` // all answered
	if s := p.Lesen(ctx, "anna"); len(s) != 1 || s[0].Still || len(*s[0].Aufgaben) != 0 || !s[0].Am.Equal(now) {
		t.Fatalf("done: %+v", s)
	}
	n := asked.Load()
	if s := p.Lesen(ctx, "ben"); s != nil || p.Gemerkt("ben") != nil || asked.Load() != n {
		t.Errorf("ben sees anna's plugin or it was asked: %+v", s)
	}
}

// A plugin that never answered, or serves no contract (an older one,
// 404), leaves no trace; removing it from Laib removes what Homes kept.
func TestKeineSpur(t *testing.T) {
	var asked atomic.Int32
	answer := `{"vertrag":1,"name":"Archiv","link":"https://a.test","aufgaben":[{"id":"f","titel":"Frage"}]}`
	file := filepath.Join(t.TempDir(), "plugins.json")
	in := fake(t, &answer, &asked)
	old := Instance{in.Plugin, in.Person, in.URL + "/alt", in.Key} // 404 at /alt/homes/v1
	if s := Open([]Instance{old}, file).Lesen(context.Background(), "anna"); s != nil {
		t.Fatalf("an archive without the contract: %+v", s)
	}
	Open([]Instance{in}, file).Lesen(context.Background(), "anna")
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "Frage") {
		t.Fatalf("nothing kept: %s", b)
	}
	if g := Open(nil, file).Gemerkt("anna"); g != nil {
		t.Errorf("removed, still there: %+v", g)
	}
	if b, _ := os.ReadFile(file); strings.Contains(string(b), "Frage") {
		t.Errorf("removed, its tasks still on disk: %s", b)
	}
}
