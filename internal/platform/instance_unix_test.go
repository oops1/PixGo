//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package platform

import (
	"net"
	"os"
	"testing"
)

func isolateInstance(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
}

func TestInstanceStaleSocket(t *testing.T) {
	isolateInstance(t)
	name := randomName(t)
	path := socketPath(name)

	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ln.SetUnlinkOnClose(false)
	ln.Close()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("stale socket missing: %v", err)
	}
	roundTrip(t, name)

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, name)
}

func TestInstanceSocketMode(t *testing.T) {
	isolateInstance(t)
	name := randomName(t)
	a := NewInstance(name)
	if p, err := a.Acquire(); err != nil || !p {
		t.Fatal(p, err)
	}
	defer a.Close()
	fi, err := os.Stat(socketPath(name))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v", fi.Mode().Perm())
	}
}
