// Package addressfamily selects which IP addresses of a server are used. It is
// shared by the node address code and the load balancer target code.
package addressfamily

import (
	"errors"
	"strings"
)

// Family selects which IP addresses of a server are used.
type Family int

const (
	// DualStack uses both the IPv4 and the IPv6 address.
	DualStack Family = iota
	// IPv6 uses the IPv6 address only.
	IPv6
	// IPv4 uses the IPv4 address only.
	IPv4
)

// ErrInvalid is returned by Parse for a value that names no address family.
var ErrInvalid = errors.New("invalid value, expected one of: ipv4,ipv6,dualstack")

// Parse reads the value used in environment variables and service annotations.
func Parse(v string) (Family, error) {
	switch strings.ToLower(v) {
	case "ipv6":
		return IPv6, nil
	case "ipv4":
		return IPv4, nil
	case "dualstack":
		return DualStack, nil
	default:
		return -1, ErrInvalid
	}
}

// UsesIPv4 reports whether f covers the IPv4 address.
func (f Family) UsesIPv4() bool {
	return f == IPv4 || f == DualStack
}

// UsesIPv6 reports whether f covers the IPv6 address.
func (f Family) UsesIPv6() bool {
	return f == IPv6 || f == DualStack
}
