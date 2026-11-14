//go:build !linux

package erreichbar

import (
	"net/netip"
	"time"
)

// ping is not tried off Linux: the TCP check answers alone.
func ping(netip.Addr, time.Duration) bool { return false }
