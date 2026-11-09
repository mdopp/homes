package paperless

import (
	"context"
	"io"
	"net/url"
	"strconv"
)

// SteuerTag is the tag dokumente's ordnen gives what counts for the tax
// return (owner 2026-11-09), beside the fields Steuer-Art, Steuerjahr
// and, for §35a, Lohnkosten.
const SteuerTag = "steuerlich"

// Steuerbelege are the documents tagged SteuerTag (at most 1000, a
// household's years of them), the newest first; none without the tag.
// Kept a minute.
func (c *Client) Steuerbelege(ctx context.Context) ([]Eintrag, error) {
	o, err := c.Ordnung(ctx)
	if err != nil {
		return nil, err
	}
	id, ok := o.tag[fold(SteuerTag)]
	if !ok {
		return nil, nil
	}
	q := url.Values{"tags__id__all": {strconv.Itoa(id)}, "ordering": {"-created"}}
	return merken(c, q.Encode(), func() ([]Eintrag, error) { return c.eintraege(ctx, o, q, 1000) })
}

// Datei is a document's file as Paperless hands it out (its archived PDF,
// else the original) and its media type; the caller closes it. A GET, as
// everything here.
func (c *Client) Datei(ctx context.Context, id int) (io.ReadCloser, string, error) {
	resp, err := c.open(ctx, "/api/documents/"+strconv.Itoa(id)+"/download/", url.Values{})
	if err != nil {
		return nil, "", err
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}
