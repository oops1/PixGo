//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package platform

import "net"

func peerIsMe(*net.UnixConn) bool { return true }
