// Package app holds the names the Homes app (mdopp/homes-android) and Homes
// share: its deep-link scheme, its device-token prefix and where it reads
// the box. They are the app's contract (laib ADR 0022 §1), one set; no
// older name is carried (owner 2026-11-28).
package app

const (
	// Scheme is the app's deep-link scheme: <scheme>://pair#token=…&id=… opens
	// the app after pairing, <scheme>://setup?server=…&token=… is what its QR
	// scanner reads.
	Scheme = "cloud.dopp.homes"
	// TokenPrefix starts every device token Homes mints; the app checks it
	// before it stores one, and Homes takes no bearer without it.
	TokenPrefix = "homes_device_"
	// Paket is the Homes app's package: its Android app links, its device in
	// Home Assistant's mobile_app, the setup step that installs it.
	Paket = "cloud.dopp.homes"
	// Quelle is where the Homes app is released (Obtainium follows it).
	Quelle = "https://github.com/mdopp/homes-android"
	// BoxPrefix is where the app reads the box: home, whether all is well.
	BoxPrefix = "/napi/box"
)
