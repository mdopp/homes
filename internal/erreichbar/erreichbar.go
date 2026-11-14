// Package erreichbar asks whether a device in the house answers on the
// network (owner 2026-11-14: a printer Home Assistant cannot reach — is it
// off, or is it HA?). A ping first, as an unprivileged ICMP datagram
// socket where the kernel allows one (Linux, net.ipv4.ping_group_range);
// where it does not, or nothing came back, a TCP connect to the ports a
// printer serves. The caller names the device's own address, never one a
// person typed.
package erreichbar

import (
	"context"
	"net"
	"net/netip"
	"time"
)

// Antwort is how the device answered.
type Antwort int

const (
	Nicht Antwort = iota // no answer, or the name did not resolve
	Ping                 // it answered a ping
	Port                 // it accepted a TCP connection, a ping not tried or not answered
)

// Wait bounds the whole check; Ports are what a printer listens on (IPP, its web page).
var (
	Wait  = 2 * time.Second
	Ports = []int{631, 80}
)

// Pruefe checks host (a name or an IPv4 address) within Wait.
func Pruefe(ctx context.Context, host string) Antwort {
	ctx, cancel := context.WithTimeout(ctx, Wait)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return Nicht
	}
	ip := ips[0].Unmap()
	deadline, _ := ctx.Deadline()
	if ping(ip, time.Until(deadline)/2) {
		return Ping
	}
	answered := make(chan bool, len(Ports))
	for _, p := range Ports {
		go func() {
			c, err := (&net.Dialer{}).DialContext(ctx, "tcp", netip.AddrPortFrom(ip, uint16(p)).String())
			if err == nil {
				c.Close()
			}
			answered <- err == nil
		}()
	}
	for range Ports {
		if <-answered {
			return Port
		}
	}
	return Nicht
}

// checksum is the Internet checksum of an ICMP message.
func checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// echo is an ICMP echo request: type 8, code 0, its checksum, id, seq 1, "homes".
func echo() []byte {
	b := []byte{8, 0, 0, 0, 0, 1, 0, 1, 'h', 'o', 'm', 'e', 's'}
	c := checksum(b)
	b[2], b[3] = byte(c>>8), byte(c)
	return b
}
