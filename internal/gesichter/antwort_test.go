package gesichter

import (
	"maps"
	"testing"
)

// A background run that read before the owner's answer must not write
// its older stand over it (the „Ist das …?“ question came back after Ja).
func TestNachziehenAnswerWins(t *testing.T) {
	g := &Gesichter{}
	g.beantworten([]string{"privat/alt"}, nil)
	seit := g.zug // the run reads here
	g.beantworten([]string{"privat/tinka", "privat/weg"}, []string{"p-alt"})

	jetzt := map[string]string{"privat/tinka": "ja", "privat/alt": "jetzt"}
	neu := map[string]string{"privat/tinka": "frage", "privat/weg": "frage", "privat/alt": "lauf", "privat/neu": "lauf"}
	nachziehen(g, neu, jetzt, seit)

	want := map[string]string{"privat/tinka": "ja", "privat/alt": "lauf", "privat/neu": "lauf"}
	if !maps.Equal(neu, want) {
		t.Fatalf("got %v, want %v", neu, want)
	}
	if !g.weg["p-alt"] {
		t.Fatal("a person merged away is not noted")
	}
}
