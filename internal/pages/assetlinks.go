package pages

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/app"
)

// AssetLinksPath is where Android verifies that Homes' address belongs to
// the Homes app (ADR 0004 §5): with it, a tag's <HOMES_PUBLIC_URL>/tag/<id>
// opens the app directly (an App Link, autoVerify), without it the browser.
const AssetLinksPath = "/.well-known/assetlinks.json"

// AssetLinks is the statement for the app's package and the SHA-256 of
// its signing certificates (HOMES_APP_SHA256, comma-separated: the release
// key, and Play's app signing key if there is one); none: 404.
func AssetLinks(shas string) http.HandlerFunc {
	var fps []string
	for _, s := range strings.Split(shas, ",") {
		s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), ":", ""))
		if s == "" {
			continue
		}
		if b, err := hex.DecodeString(s); err != nil || len(b) != 32 {
			log.Printf("homes: HOMES_APP_SHA256: %q is no SHA-256 fingerprint, left out", s)
			continue
		}
		var parts []string
		for i := 0; i < len(s); i += 2 {
			parts = append(parts, s[i:i+2])
		}
		fps = append(fps, strings.Join(parts, ":"))
	}
	body, _ := json.Marshal([]map[string]any{{"relation": []string{"delegate_permission/common.handle_all_urls"},
		"target": map[string]any{"namespace": "android_app", "package_name": app.Paket, "sha256_cert_fingerprints": fps}}})
	return func(w http.ResponseWriter, r *http.Request) {
		if len(fps) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}
