package ha

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// A paired phone as a device of HA's mobile_app integration (homes ADR
// 0004 §7), the open API HA's own app speaks: a registration under Homes'
// HA user, then the registration's webhook for its sensors and its scans.

// MobileApp is the integration's domain, a device's identifier's first half.
const MobileApp = "mobile_app"

// ErrGone is HA's 410 for the webhook of a registration removed in HA.
var ErrGone = errors.New("ha: the mobile_app registration is gone")

// Registrierung is what HA's registration asks of a device.
type Registrierung struct {
	DeviceID, AppID, AppName, AppVersion, DeviceName, Manufacturer, Model, OSName string
	// OSVersion: HA reads it when it sets the entry up (its device's
	// sw_version) though its schema calls it optional; without it the
	// registration is taken and the entry fails (seen 2026-11-28).
	OSVersion string
}

// RegisterMobileApp registers a device (POST /api/mobile_app/registrations)
// and answers its webhook id, a secret: whoever holds it reports as the
// device. ErrNotFound when HA has no mobile_app integration loaded.
func (c *Client) RegisterMobileApp(ctx context.Context, r Registrierung) (string, error) {
	var out struct {
		WebhookID string `json:"webhook_id"`
	}
	err := c.do(ctx, "POST", "/api/mobile_app/registrations", nil, map[string]any{
		"device_id": r.DeviceID, "app_id": r.AppID, "app_name": r.AppName, "app_version": r.AppVersion,
		"device_name": r.DeviceName, "manufacturer": r.Manufacturer, "model": r.Model, "os_name": r.OSName,
		"os_version": r.OSVersion, "supports_encryption": false, "app_data": map[string]any{}}, &out)
	var he *Error
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return "", ErrNotFound
	}
	if err == nil && out.WebhookID == "" {
		err = errors.New("ha: the registration answered no webhook_id")
	}
	return out.WebhookID, err
}

// MobileWebhook sends one command (register_sensor, update_sensor_states,
// scan_tag) through a registration's webhook; ErrGone when HA removed it.
func (c *Client) MobileWebhook(ctx context.Context, webhookID, typ string, data any) error {
	err := c.do(ctx, "POST", "/api/webhook/"+url.PathEscape(webhookID), nil, map[string]any{"type": typ, "data": data}, nil)
	var he *Error
	if errors.As(err, &he) && he.Status == http.StatusGone {
		return ErrGone
	}
	return err
}

// MobileAppEntries is, for each mobile_app device in HA's device registry,
// its device_id → the config entry of its registration.
func (c *Client) MobileAppEntries(ctx context.Context) (map[string]string, error) {
	return c.mobileApp(ctx, func(d mobileDevice) string { return d.ConfigEntries[0] })
}

// MobileAppDevices is, for each mobile_app device, its id in HA's device
// registry (what a scan's tag_scanned names) → its registration's device_id.
func (c *Client) MobileAppDevices(ctx context.Context) (map[string]string, error) {
	byDevice, err := c.mobileApp(ctx, func(d mobileDevice) string { return d.ID })
	out := map[string]string{}
	for dev, id := range byDevice {
		out[id] = dev
	}
	return out, err
}

type mobileDevice struct {
	ID            string     `json:"id"`
	Identifiers   [][]string `json:"identifiers"`
	ConfigEntries []string   `json:"config_entries"`
}

// mobileApp is each mobile_app device's device_id → what of reads of it.
func (c *Client) mobileApp(ctx context.Context, of func(mobileDevice) string) (map[string]string, error) {
	var devices []mobileDevice
	if err := c.command(ctx, map[string]any{"type": "config/device_registry/list"}, &devices); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, d := range devices {
		for _, id := range d.Identifiers {
			if len(id) == 2 && id[0] == MobileApp && len(d.ConfigEntries) > 0 {
				out[id[1]] = of(d)
			}
		}
	}
	return out, nil
}

// DeleteConfigEntry removes a config entry (a mobile_app registration with
// its device and entities); ErrNotFound when HA has none under id.
func (c *Client) DeleteConfigEntry(ctx context.Context, entryID string) error {
	err := c.do(ctx, "DELETE", "/api/config/config_entries/entry/"+url.PathEscape(entryID), nil, nil, nil)
	var he *Error
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return ErrNotFound
	}
	return err
}

// MobileAppBroken is the config entries of mobile_app whose setup failed
// under title (a registration HA took but could not set up): its entry
// ids, for removal before the device registers again.
func (c *Client) MobileAppBroken(ctx context.Context, title string) ([]string, error) {
	var entries []struct {
		EntryID string `json:"entry_id"`
		Domain  string `json:"domain"`
		Title   string `json:"title"`
		State   string `json:"state"`
	}
	if err := c.do(ctx, "GET", "/api/config/config_entries/entry", nil, nil, &entries); err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Domain == MobileApp && e.Title == title && e.State == "setup_error" {
			out = append(out, e.EntryID)
		}
	}
	return out, nil
}
