package erreichbar

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

// A device that accepts a connection on a printer's port answers, by ping
// where the kernel allows one, else by the port.
func TestPruefeAnswers(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	defer func(p []int) { Ports = p }(Ports)
	Ports = []int{l.Addr().(*net.TCPAddr).Port}
	if a := Pruefe(t.Context(), "127.0.0.1"); a != Ping && a != Port {
		t.Errorf("answer %v, want Ping or Port", a)
	}
}

// A ping with no time left is not sent: no answer, and the TCP check decides.
func TestPingWithoutTimeIsNo(t *testing.T) {
	if ping(netipLoopback(t), 0) {
		t.Fatal("a ping with no time to wait answered")
	}
}

// An address nothing answers on (TEST-NET-1, RFC 5737) is Nicht, a name
// that does not resolve (a printer's .local name without mDNS) Unbekannt,
// each within Wait.
func TestPruefeNothing(t *testing.T) {
	defer func(w time.Duration) { Wait = w }(Wait)
	Wait = 400 * time.Millisecond
	began := time.Now()
	if a := Pruefe(t.Context(), "192.0.2.1"); a != Nicht {
		t.Errorf("TEST-NET answered %v", a)
	}
	if a := Pruefe(t.Context(), "drucker.invalid"); a != Unbekannt {
		t.Errorf("a name that does not resolve answered %v", a)
	}
	if a := Pruefe(t.Context(), "EPSON0A36A8.local."); a != Unbekannt && a != Nicht {
		t.Errorf("a .local name answered %v", a) // Nicht only where an mDNS resolver answers it and the printer does not
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("took %v, bounded by Wait", took)
	}
}

// The echo request's checksum makes the whole message sum to zero.
func TestEchoChecksum(t *testing.T) {
	if c := checksum(echo()); c != 0 {
		t.Errorf("checksum over the message %#x, want 0", c)
	}
}

func netipLoopback(t *testing.T) netip.Addr {
	t.Helper()
	return netip.MustParseAddr("127.0.0.1")
}
