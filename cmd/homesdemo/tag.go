package main

import (
	"log"
	"net/http"

	"github.com/mdopp/homes/internal/ha/hafake"
	"github.com/mdopp/homes/internal/pages"
	"github.com/mdopp/homes/internal/tagaktion"
)

// tagSeiten are a tag's own page and the admins' answer to a wish (ADR
// 0004 §5, §8): „Bürotür“, which Flurlicht starts on, „Gute Nacht“ to
// link; a scanner tag that asks for whom. As another person (the cookie
// wer) a link waits for an admin; as Anna (an admin) on Deine Aufgaben
// it is answered. It returns the Dienst for Deine Aufgaben's rows.
func tagSeiten(mux *http.ServeMux, cfg pages.StartConfig, fake *hafake.HA, dir string) *tagaktion.Dienst {
	fake.AddTag("homes-b0e70e000001", "Bürotür")
	fake.AddTag("homes-5ca11e000001", "Scanner")
	fake.AddAutomation("1700000000001", "Gute Nacht", map[string]any{"id": "1700000000001", "alias": "Gute Nacht",
		"triggers": []any{map[string]any{"trigger": "time", "at": "23:00:00"}}, "actions": []any{}})
	fake.AddAutomation("1700000000002", "Flurlicht", map[string]any{"id": "1700000000002", "alias": "Flurlicht",
		"triggers": []any{map[string]any{"trigger": "tag", "tag_id": "homes-b0e70e000001"}}, "actions": []any{}})
	fake.RunsOnTag("1700000000002", "finished", "action/0") // what came of a touch, from HA's traces (§9)
	store, err := tagaktion.Open(dir)
	if err != nil {
		log.Fatal(err)
	}
	store.Setze("homes-5ca11e000001", tagaktion.ScannerFragen)
	cfg.Tags = &tagaktion.Dienst{HA: cfg.HA, Laib: cfg.Laib, Store: store, Bote: cfg.Bote, PublicURL: cfg.PublicURL, Dir: dir,
		Admins: func() []string { return []string{"anna"} }}
	mux.HandleFunc(pages.TagPath, pages.Tag(cfg))
	mux.HandleFunc(pages.TagFreigabePath, pages.TagFreigabe(cfg))
	return cfg.Tags
}
