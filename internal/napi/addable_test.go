package napi

import (
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/laib"
)

// The device picker reads /napi/portal/start/addable: the actuators, by
// room; a camera is nothing a widget switches (the camera widget is gone,
// owner 2026-11-28).

// addable is what both pickers read: the device picker (actuators) and the
// camera picker (it keeps domain == camera). Grouped by room, the room-less last.
func TestStartAddable(t *testing.T) {
	fake, h := house(t)
	fake.Set("camera.einfahrt", "idle", map[string]any{"friendly_name": "Einfahrt"})
	fake.Set("camera.garten", "streaming", map[string]any{"friendly_name": "Garten"})
	fake.SetRegistries(
		[]map[string]any{{"area_id": "kueche", "name": "Küche"}, {"area_id": "aussen", "name": "Außen"}},
		nil,
		[]map[string]any{{"entity_id": "light.kueche", "area_id": "kueche"}, {"entity_id": "camera.einfahrt", "area_id": "aussen"}, {"entity_id": "camera.garten", "area_id": "aussen"}},
	)
	code, body := raw(t, h, "GET", "/napi/portal/start/addable", "dev1", "")
	if code != 200 {
		t.Fatalf("addable: %d %s", code, body)
	}
	out := decode(t, body)
	if out["ok"] != true {
		t.Fatalf("addable: %s", body)
	}
	if a, ok := out["automations"].([]any); !ok || len(a) != 0 {
		t.Fatalf("automations is an empty list: %s", body)
	}
	rooms, _ := out["rooms"].([]any)
	var order []string
	seen := map[string]map[string]any{}
	roomOf := map[string]string{}
	for _, r := range rooms {
		rm := r.(map[string]any)
		order = append(order, rm["room"].(string))
		for _, c := range rm["cards"].([]any) {
			cm := c.(map[string]any)
			seen[cm["entity_id"].(string)] = cm
			roomOf[cm["entity_id"].(string)] = rm["room"].(string)
		}
	}
	if strings.Join(order, "|") != "Küche|" { // Außen holds only cameras
		t.Fatalf("rooms by name, none last: %v", order)
	}
	for _, id := range []string{"light.kueche", "light.flur", "switch.kaffee", "cover.garage", "lock.haustuer", "climate.bad"} {
		if seen[id] == nil {
			t.Errorf("%s missing", id)
		}
	}
	for _, id := range []string{"sensor.temp", "button.tuer", "camera.einfahrt", "camera.garten"} {
		if seen[id] != nil {
			t.Errorf("%s is nothing a widget switches or shows", id)
		}
	}
	if c := seen["light.kueche"]; c["domain"] != "light" || roomOf["light.kueche"] != "Küche" {
		t.Fatalf("kitchen light: %v in %q", c, roomOf["light.kueche"])
	}
	if c := seen["cover.garage"]; c["device_class"] != "garage" {
		t.Fatalf("device_class rides along: %v", c)
	}
	if _, ok := seen["light.flur"]["device_class"]; ok {
		t.Fatalf("no class is no key: %v", seen["light.flur"])
	}
	for _, bearer := range []string{"", "wrong"} {
		if code, _ := raw(t, h, "GET", "/napi/portal/start/addable", bearer, ""); code != 401 {
			t.Errorf("addable with %q → %d", bearer, code)
		}
	}
	fake.Down(true)
	if code, _ := raw(t, h, "GET", "/napi/portal/start/addable", "dev1", ""); code != 502 {
		t.Fatalf("HA down: %d", code)
	}
}

func TestWithoutHAAddableIs404(t *testing.T) {
	var calls []string
	lb := fakeLaib(t, &calls)
	defer lb.Close()
	h := Handler(Config{Laib: &laib.Client{URL: lb.URL, Token: "ops"}, Validate: StaticValidator([]string{"dev1"})})
	for _, p := range []string{"/napi/portal/camera/camera.einfahrt/snapshot", "/napi/portal/entity-history", "/napi/portal/start/addable"} {
		code, body := raw(t, h, "GET", p, "dev1", "")
		notFound(t, p, code, body)
	}
}
