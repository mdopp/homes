package erreichbar

import (
	"net/netip"
	"syscall"
	"time"
)

// ping sends one echo request over an unprivileged ICMP datagram socket
// and waits up to wait for its reply; false when the kernel refuses the
// socket or nothing came back.
func ping(ip netip.Addr, wait time.Duration) bool {
	if !ip.Is4() || wait <= 0 {
		return false
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, syscall.IPPROTO_ICMP)
	if err != nil {
		return false // EACCES: this group may not ping (net.ipv4.ping_group_range)
	}
	defer syscall.Close(fd)
	tv := syscall.NsecToTimeval(wait.Nanoseconds())
	if syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv) != nil {
		return false
	}
	if syscall.Sendto(fd, echo(), 0, &syscall.SockaddrInet4{Addr: ip.As4()}) != nil {
		return false
	}
	end, buf := time.Now().Add(wait), make([]byte, 512)
	for time.Now().Before(end) {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return false // the timeout
		}
		if n >= 8 && buf[0] == 0 { // an echo reply, its ICMP header first (the kernel strips the IP header)
			return true
		}
	}
	return false
}
