package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
)

// nummernBereinigen is `homes nummern-bereinigen [-apply] [-person login]…` (owner 2026-11-11): each number once on
// the household's cards and the named persons' mirrored phone contacts (caldav.NummernBereinigen); without -apply
// it only counts. It names no contact, number or person; 1 when a book or a card was not read or written.
func nummernBereinigen(ctx context.Context, args []string, getenv func(string) string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("nummern-bereinigen", flag.ContinueOnError)
	fs.SetOutput(errw)
	apply, people := fs.Bool("apply", false, "schreiben; ohne: nur zählen"), names{}
	fs.Var(&people, "person", "auch die Handy-Kontakte dieser Person (Login); mehrfach möglich")
	if fs.Parse(args) != nil {
		return 2
	}
	c := &caldav.Client{URL: strings.TrimRight(getenv("HOMES_RADICALE_URL"), "/"), User: getenv("HOMES_RADICALE_USER"),
		Password: getenv("HOMES_RADICALE_PASSWORD"), AddressBook: getenv("HOMES_RADICALE_ADDRESSBOOK")}
	books, err := c.HaushaltsBuecher(ctx)
	if err != nil {
		fmt.Fprintf(errw, "Das Haushalts-Adressbuch ist nicht lesbar (HOMES_RADICALE_*): %v\n", err)
		return 1
	}
	was := make([]string, len(books))
	for i, p := range people {
		was, books = append(was, fmt.Sprintf("Handy-Kontakte, Person %d", i+1)), append(books, "/"+url.PathEscape(p)+"/"+caldav.HandyBuch+"/")
	}
	fmt.Fprint(out, map[bool]string{false: "Probelauf: geschrieben wird nichts.\n"}[*apply])
	code, cc := 0, cmp.Or(strings.TrimPrefix(strings.TrimSpace(getenv("HOMES_PHONE_COUNTRY")), "+"), "49")
	for i, b := range books {
		r, err := c.NummernBereinigen(ctx, b, cc, *apply)
		fmt.Fprintf(out, "%s: %d Kontakte, %d mit einer Nummer mehrfach, %d doppelte Zeilen %s.\n", cmp.Or(was[i], "Haushalt"), r.Karten, r.Doppelt, r.Zeilen,
			map[bool]string{false: "würden wegfallen", true: fmt.Sprintf("weggefallen (%d Kontakte geschrieben)", r.Geschrieben)}[*apply])
		if err != nil {
			fmt.Fprintf(out, "  abgebrochen: %v; ein zweiter Lauf macht weiter.\n", err)
			code = 1
		}
	}
	return code
}
