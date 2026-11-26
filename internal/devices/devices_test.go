package devices

import (
	"strings"
	"testing"
)

func TestMintLookupRevoke(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, tok, err := s.Mint("Pixel", "mdopp")
	if err != nil || !strings.HasPrefix(tok, "homes_device_") || d.UID != "mdopp" {
		t.Fatalf("mint: %v %q %+v", err, tok, d)
	}
	if got, ok := s.Lookup(tok); !ok || got.ID != d.ID || got.LastSeen.IsZero() {
		t.Fatalf("lookup: %v %+v", ok, got)
	}
	if _, ok := s.Lookup("homes_device_nope"); ok {
		t.Fatal("unknown token accepted")
	}
	// haus#26: a device paired before the new app is not migrated; its hash
	// in devices.json matches nothing, the phone pairs again
	old := "old_device_" + strings.Repeat("x", 43)
	s.list = append(s.list, Device{ID: "alt", Name: "Altes Telefon", UID: "mdopp", Hash: hash(old)})
	if _, ok := s.Lookup(old); ok {
		t.Fatal("a token without homes_device_ accepted")
	}
	s.list = s.list[:len(s.list)-1]
	// laib#1021: a phone paired with the old Haus app keeps its token; the
	// stored hash is the one minted then
	legacy := "haus_device_" + strings.Repeat("y", 43)
	s.list = append(s.list, Device{ID: "haus", Name: "Telefon mit der Haus-App", UID: "mdopp", Hash: hash(legacy)})
	if got, ok := s.Lookup(legacy); !ok || got.ID != "haus" {
		t.Fatalf("a haus_device_ token of a paired phone refused: %v %+v", ok, got)
	}
	if _, ok := s.Lookup("haus_device_nope"); ok {
		t.Fatal("unknown legacy token accepted")
	}
	s.list = s.list[:len(s.list)-1]
	// the file holds the hash, never the token
	s2, _ := Open(s.path[:len(s.path)-len("/devices.json")])
	if _, ok := s2.Lookup(tok); !ok {
		t.Fatal("token lost on reload")
	}
	if strings.Contains(string(mustRead(t, s.path)), tok) {
		t.Fatal("plaintext token on disk")
	}
	if err := s.Revoke(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup(tok); ok {
		t.Fatal("revoked token still valid")
	}
}

// A revoked device is told to Weg once the file is written (its HA
// registration goes, ADR 0004 §7); an unknown one is not.
func TestRevokeTellsWeg(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var weg []Device
	s.Weg = func(d Device) { weg = append(weg, d) }
	d, _, _ := s.Mint("Pixel 9", "anna")
	if !s.Gibt(d.ID) || s.Revoke(d.ID) != nil || s.Gibt(d.ID) {
		t.Fatal("revoke")
	}
	if s.Revoke("nope") == nil || len(weg) != 1 || weg[0].ID != d.ID || weg[0].UID != "anna" {
		t.Fatalf("%+v", weg)
	}
}
