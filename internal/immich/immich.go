// Package immich is Homes' way into Immich, the household's photos
// (photos.<domain>, laib's immich template): the people its face
// recognition knows by name, each one's face, and a person per contact
// (owner 2026-11-06). Immich keeps people per user, so Homes works with an
// API key of the owner's account (HOMES_IMMICH_KEY: person.read,
// person.create, person.update, person.merge, user.read; asset.upload,
// face.read and asset.delete for the contacts' pictures; person.statistics
// if it may count photos; face.read and asset.view for the small faces
// on „Dieselbe Person?“). The API as Immich 3.3.5 serves it
// (server/src/controllers/person.controller.ts, search.controller.ts,
// user.controller.ts, asset-media.controller.ts, face.controller.ts).
package immich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Person is one of Immich's people.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Geburtstag is Immich's birthDate, JJJJ-MM-TT; "" none
	Geburtstag string `json:"birthDate,omitempty"`
	// Stand is Immich's updatedAt: it changes with the name or the face
	Stand     string `json:"updatedAt,omitempty"`
	Versteckt bool   `json:"isHidden,omitempty"`
	Favorit   bool   `json:"isFavorite,omitempty"`
}

// Client reaches Immich inside the box.
type Client struct {
	URL  string       // HOMES_IMMICH_URL, http://host.containers.internal:2283
	Key  string       // HOMES_IMMICH_KEY: the owner's API key
	HTTP *http.Client // nil: a client with a 20 s timeout
}

// FromEnv is Immich from HOMES_IMMICH_URL and HOMES_IMMICH_KEY; nil
// without either.
func FromEnv() *Client {
	u, k := strings.TrimRight(os.Getenv("HOMES_IMMICH_URL"), "/"), strings.TrimSpace(os.Getenv("HOMES_IMMICH_KEY"))
	if u == "" || k == "" {
		return nil
	}
	return &Client{URL: u, Key: k}
}

// maxSeiten bounds the people read: 500 a page, 10,000 in all.
const maxSeiten = 20

// People are the people Immich does not hide, every page of them: the
// named, and the unnamed (ohne) in Immich's order, the most faces first.
func (c *Client) People(ctx context.Context) (named, ohne []Person, err error) {
	for page := 1; page <= maxSeiten; page++ {
		ps, next, err := c.seite(ctx, url.Values{"size": {"500"}, "page": {fmt.Sprint(page)}})
		if err != nil {
			return nil, nil, err
		}
		for _, p := range ps {
			if strings.TrimSpace(p.Name) != "" {
				named = append(named, p)
			} else {
				ohne = append(ohne, p)
			}
		}
		if !next {
			break
		}
	}
	return named, ohne, nil
}

// seite is one page of GET /api/people, hidden ones left out: the
// favourites first, then (with closestPersonId) by how close the face is,
// then the named by how many faces (person.repository.ts getAllForUser).
func (c *Client) seite(ctx context.Context, q url.Values) ([]Person, bool, error) {
	var answer struct {
		People []Person `json:"people"`
		Next   bool     `json:"hasNextPage"`
	}
	q.Set("withHidden", "false")
	err := c.getJSON(ctx, "/api/people?"+q.Encode(), &answer)
	return slices.DeleteFunc(answer.People, func(p Person) bool { return p.Versteckt || p.ID == "" }), answer.Next, err
}

// Naechste are the n people whose face is closest to id's, the unnamed
// too, id itself among them, closest first after the favourites; Immich
// gives the order, no distance (closestPersonId).
func (c *Client) Naechste(ctx context.Context, id string, n int) ([]Person, error) {
	ps, _, err := c.seite(ctx, url.Values{"closestPersonId": {id}, "size": {fmt.Sprint(n)}, "page": {"1"}})
	return ps, err
}

// Fotos is how many photos show id (GET /api/people/{id}/statistics,
// person.statistics).
func (c *Client) Fotos(ctx context.Context, id string) (int, error) {
	var answer struct{ Assets int }
	err := c.getJSON(ctx, "/api/people/"+url.PathEscape(id)+"/statistics", &answer)
	return answer.Assets, err
}

// Zusammenfuehren merges quellen into ziel (POST /api/people/merge, ids in
// order, the first the one that stays: its name and birthday win, a
// missing one comes from the others; person.merge) and answers the
// quellen Immich merged.
func (c *Client) Zusammenfuehren(ctx context.Context, ziel string, quellen []string) ([]string, error) {
	b, _ := json.Marshal(map[string][]string{"ids": append([]string{ziel}, quellen...)})
	body, err := c.send(ctx, http.MethodPost, "/api/people/merge", b)
	var answer []struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
	}
	if err == nil {
		err = json.Unmarshal(body, &answer)
	}
	var out []string
	for _, a := range answer {
		if a.Success && a.ID != ziel && slices.Contains(quellen, a.ID) {
			out = append(out, a.ID)
		}
	}
	return out, err
}

// Gesicht is the person's face as Immich cut it: a JPEG of 250 px.
func (c *Client) Gesicht(ctx context.Context, id string) ([]byte, error) {
	return c.get(ctx, "/api/people/"+url.PathEscape(id)+"/thumbnail")
}

// Braucht are the permissions Homes needs of the key (asset.read: a
// person's places and company, Umfeld; asset.view: a photo's preview,
// Vorschau); person.statistics only counts the photos.
var Braucht = append([]string{"person.read", "person.create", "person.update", "person.merge", "user.read", "asset.read", "asset.view"}, BildRechte...)

// Foto is one photo as Umfeld reads it: which, when, where, and who is in it.
type Foto struct {
	ID   string `json:"id"`
	Wann string `json:"localDateTime"`
	Exif struct {
		Stadt string `json:"city"`
		Land  string `json:"country"`
	} `json:"exifInfo"`
	Personen []Person `json:"people"`
}

// Umfeld are the newest n photos of person id with their place and
// people (POST /api/search/metadata, personIds, withExif, withPeople;
// asset.read).
func (c *Client) Umfeld(ctx context.Context, id string, n int) ([]Foto, error) {
	b, _ := json.Marshal(map[string]any{"personIds": []string{id}, "withExif": true, "withPeople": true, "size": n, "order": "desc"})
	body, err := c.send(ctx, http.MethodPost, "/api/search/metadata", b)
	var answer struct{ Assets struct{ Items []Foto } }
	if err == nil {
		err = json.Unmarshal(body, &answer)
	}
	return answer.Assets.Items, err
}

// Aufnahme is one photo as the contact net reads it (owner 2026-11-09):
// its id, when it was taken (dateTimeOriginal, an instant, not the
// localDateTime), where, and who is in it.
type Aufnahme struct {
	ID   string `json:"id"`
	Exif struct {
		Zeit  string   `json:"dateTimeOriginal"`
		Lat   *float64 `json:"latitude"`
		Lng   *float64 `json:"longitude"`
		Stadt string   `json:"city"`
	} `json:"exifInfo"`
	Personen []Person `json:"people"`
}

// maxAufnahmeSeiten bounds Aufnahmen: 500 a page, 200,000 photos.
const maxAufnahmeSeiten = 400

// Aufnahmen are all of the account's photos with at least one person,
// every page of them (POST /api/search/metadata, withExif, withPeople,
// page and its nextPage; asset.read). 500 a page, not Immich's 1000, so a
// page with its exif and faces stays under maxAntwort.
func (c *Client) Aufnahmen(ctx context.Context) (out []Aufnahme, err error) {
	for page := 1; page > 0 && page <= maxAufnahmeSeiten; {
		b, _ := json.Marshal(map[string]any{"withExif": true, "withPeople": true, "size": 500, "page": page})
		var answer struct {
			Assets struct {
				Items []Aufnahme
				Next  *string `json:"nextPage"`
			}
		}
		body, err := c.send(ctx, http.MethodPost, "/api/search/metadata", b)
		if err == nil {
			err = json.Unmarshal(body, &answer)
		}
		if err != nil {
			return nil, err
		}
		for _, a := range answer.Assets.Items {
			if len(a.Personen) > 0 {
				out = append(out, a)
			}
		}
		page = 0
		if answer.Assets.Next != nil {
			page, _ = strconv.Atoi(*answer.Assets.Next)
		}
	}
	return out, nil
}

// BildRechte are those the contacts' pictures need (Hochladen, Funde,
// Entfernen).
var BildRechte = []string{"asset.upload", "face.read", "asset.delete"}

// Hochladen puts a contact's picture into Immich as an archived photo
// (POST /api/assets, visibility archive; asset.upload): out of the
// timeline, its face found and recognised all the same. „hidden“ is
// skipped by the face detection, „locked“ needs the PIN a key has not
// (person.service.ts handleDetectFaces, access.repository.ts). doppelt:
// Immich had these bytes already, id is that photo, not Homes'.
func (c *Client) Hochladen(ctx context.Context, name string, jpeg []byte, wann time.Time) (id string, doppelt bool, err error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	t := wann.UTC().Format(time.RFC3339)
	for _, f := range [][2]string{{"fileCreatedAt", t}, {"fileModifiedAt", t}, {"filename", name}, {"visibility", "archive"}} {
		mw.WriteField(f[0], f[1])
	}
	if fw, err := mw.CreateFormFile("assetData", name); err == nil {
		fw.Write(jpeg)
	}
	mw.Close()
	b, err := c.sendAls(ctx, http.MethodPost, "/api/assets", body.Bytes(), mw.FormDataContentType())
	var answer struct{ ID, Status string }
	if err == nil {
		err = json.Unmarshal(b, &answer)
	}
	return answer.ID, answer.Status == "duplicate", err
}

// Fund is a face Immich found in a photo; Person nil while it has none.
type Fund struct {
	ID     string  `json:"id"`
	Person *Person `json:"person"`
	Rahmen
}

// Rahmen is where a face is in its photo, in pixels of Breite×Hoehe
// (Immich's imageWidth and imageHeight, not the preview's).
type Rahmen struct {
	X1     int `json:"boundingBoxX1"`
	Y1     int `json:"boundingBoxY1"`
	X2     int `json:"boundingBoxX2"`
	Y2     int `json:"boundingBoxY2"`
	Breite int `json:"imageWidth"`
	Hoehe  int `json:"imageHeight"`
}

// Vorschau is the photo asset as Immich's preview, a JPEG of about
// 1440 px (GET /api/assets/{id}/thumbnail?size=preview; asset.view).
func (c *Client) Vorschau(ctx context.Context, asset string) ([]byte, error) {
	return c.get(ctx, "/api/assets/"+url.PathEscape(asset)+"/thumbnail?size=preview")
}

// Funde are the faces Immich found in the photo asset (GET
// /api/faces?id=; face.read): none before its face detection ran, or in a
// picture without a face.
func (c *Client) Funde(ctx context.Context, asset string) ([]Fund, error) {
	var out []Fund
	err := c.getJSON(ctx, "/api/faces?"+url.Values{"id": {asset}}.Encode(), &out)
	return out, err
}

// Entfernen deletes Homes' own photos for good, their faces with them
// (DELETE /api/assets, force: not into the trash; asset.delete).
func (c *Client) Entfernen(ctx context.Context, ids ...string) error {
	b, _ := json.Marshal(map[string]any{"ids": ids, "force": true})
	_, err := c.send(ctx, http.MethodDelete, "/api/assets", b)
	return err
}

// Rechte are the key's permissions, "all" for every one (GET
// /api/api-keys/me, which any key may ask).
func (c *Client) Rechte(ctx context.Context) ([]string, error) {
	var answer struct{ Permissions []string }
	err := c.getJSON(ctx, "/api/api-keys/me", &answer)
	return answer.Permissions, err
}

// Wessen is the e-mail of the Immich account the key belongs to.
func (c *Client) Wessen(ctx context.Context) (string, error) {
	var answer struct{ Email string }
	err := c.getJSON(ctx, "/api/users/me", &answer)
	return answer.Email, err
}

// Suchen are the people Immich finds for name, hidden and faceless ones
// too (GET /api/search/person; Immich's trigram match: compare the names).
func (c *Client) Suchen(ctx context.Context, name string) ([]Person, error) {
	var out []Person
	err := c.getJSON(ctx, "/api/search/person?"+url.Values{"name": {name}, "withHidden": {"true"}}.Encode(), &out)
	return out, err
}

// Anlegen makes a person with no face yet (POST /api/people): Immich's
// grid shows it only once a face is merged into it.
func (c *Client) Anlegen(ctx context.Context, name, geburtstag string) (Person, error) {
	var p Person
	body, err := c.send(ctx, http.MethodPost, "/api/people", felder(name, geburtstag))
	if err == nil {
		err = json.Unmarshal(body, &p)
	}
	return p, err
}

// Aendern sets a person's name and birthday (PUT /api/people/{id}; "" is
// none).
func (c *Client) Aendern(ctx context.Context, id, name, geburtstag string) error {
	_, err := c.send(ctx, http.MethodPut, "/api/people/"+url.PathEscape(id), felder(name, geburtstag))
	return err
}

func felder(name, geburtstag string) []byte {
	f := map[string]any{"name": name, "birthDate": nil}
	if geburtstag != "" {
		f["birthDate"] = geburtstag
	}
	b, _ := json.Marshal(f)
	return b
}

// FehltRecht is Immich refusing for a permission the key lacks (its auth
// guard: 403 „Missing required permission: <recht>“).
type FehltRecht struct{ Recht string }

func (e FehltRecht) Error() string { return "immich: the key lacks " + e.Recht }

// Antwort is Immich answering other than 200, 201 or 204.
type Antwort struct {
	Pfad string
	Code int
}

func (a Antwort) Error() string { return fmt.Sprintf("immich: %s answered %d", a.Pfad, a.Code) }

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	body, err := c.get(ctx, path)
	if err == nil {
		err = json.Unmarshal(body, v)
	}
	return err
}

// maxAntwort bounds an answer: a page of people, or one face.
const maxAntwort = 8 << 20

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	return c.send(ctx, http.MethodGet, path, nil)
}

func (c *Client) send(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return c.sendAls(ctx, method, path, body, "application/json")
}

func (c *Client) sendAls(ctx context.Context, method, path string, body []byte, typ string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.Key)
	req.Header.Set("Accept", "application/json, image/jpeg")
	if body != nil {
		req.Header.Set("Content-Type", typ)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("immich: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		var answer struct{ Message string }
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&answer)
		if recht, ok := strings.CutPrefix(answer.Message, "Missing required permission: "); ok {
			return nil, FehltRecht{recht}
		}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return nil, Antwort{strings.SplitN(path, "?", 2)[0], resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxAntwort))
}
