package service

import (
	"fmt"
	"net"
	"net/netip"
	"syscall"

	mailboxmodule "zyad.cloud/internal/modules/mailbox"
)

// Mail server hosts come from users, so connecting to them is an SSRF
// vector (internal Redis, MinIO, metadata endpoints...). Only well-known
// mail ports are accepted, and the address is checked at connect time so a
// DNS answer cannot point the connection at a private network.
var (
	allowedSMTPPorts = map[int]bool{25: true, 465: true, 587: true, 2525: true}
	allowedIMAPPorts = map[int]bool{143: true, 993: true}
)

var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

func isPublicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!carrierGradeNAT.Contains(addr)
}

// publicDialControl rejects connections to non-public addresses unless
// allowPrivate is set (local development against a mail catcher).
func publicDialControl(allowPrivate bool) func(network, address string, conn syscall.RawConn) error {
	return func(_ string, address string, _ syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("%w: %s", mailboxmodule.ErrHostNotAllowed, host)
		}
		if !isPublicAddress(addr) {
			return mailboxmodule.ErrHostNotAllowed
		}
		return nil
	}
}
