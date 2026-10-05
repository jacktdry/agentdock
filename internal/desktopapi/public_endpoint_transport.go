package desktopapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var errUnsafePublicAddress = errors.New("public endpoint address rejected")

type publicEndpointDialer struct {
	lookup func(context.Context, string) ([]net.IPAddr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func newPublicEndpointTransport() *http.Transport {
	dialer := &publicEndpointDialer{
		lookup: net.DefaultResolver.LookupIPAddr,
		dial:   (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	}
	return &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10,
	}
}

func publicEndpointAddress(ip net.IPAddr) bool {
	addr, ok := netip.AddrFromSlice(ip.IP)
	if !ok || ip.Zone != "" {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	// Shared-address space is not publicly routable either.
	if netip.MustParsePrefix("100.64.0.0/10").Contains(addr) {
		return false
	}
	return true
}

func (d *publicEndpointDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errUnsafePublicAddress
	}
	// Resolve once, validate the entire set, then dial a numeric address. Never
	// resolve the hostname a second time between validation and connection.
	addresses, err := d.lookup(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, errUnsafePublicAddress
	}
	for _, ip := range addresses {
		if !publicEndpointAddress(ip) {
			return nil, errUnsafePublicAddress
		}
	}
	for _, ip := range addresses {
		conn, err := d.dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.New("public endpoint connection failed")
}
