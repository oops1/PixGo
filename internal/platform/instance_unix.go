//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package platform

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type instanceSys struct {
	lock *os.File
	ln   *net.UnixListener
	path string
	wg   sync.WaitGroup
}

func socketPath(name string) string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return filepath.Join(d, name+".sock")
		}
	}
	return filepath.Join(os.TempDir(), name+"-"+strconv.Itoa(os.Getuid())+".sock")
}

func (s *instanceSys) acquire(name string) (bool, error) {
	path := socketPath(name)
	lf, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return false, err
	}
	if err := ownedByMe(lf); err != nil {
		lf.Close()
		return false, err
	}
	if err := unix.Flock(int(lf.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lf.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}

	if _, err := os.Lstat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			c.Close()
			lf.Close()
			return false, nil
		}
		if err := os.Remove(path); err != nil {
			lf.Close()
			return false, err
		}
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		lf.Close()
		return false, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		lf.Close()
		return false, err
	}
	s.lock, s.ln, s.path = lf, ln, path
	return true, nil
}

func ownedByMe(f *os.File) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("platform: %s is owned by another user", f.Name())
	}
	return nil
}

func (s *instanceSys) send(name string, args []string) error {
	path := socketPath(name)
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("platform: socket is owned by another user")
	}
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(ioTimeout))
	if err := writeFrame(c, args); err != nil {
		return err
	}
	return readAck(c)
}

func (s *instanceSys) listen(handle func([]string)) error {
	if s.ln == nil {
		return errNotPrimary
	}
	ln := s.ln
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, err := ln.AcceptUnix()
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					continue
				}
				if errors.Is(err, net.ErrClosed) {
					return
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			args, ok := serveConn(c)
			if ok {
				handle(args)
			}
		}
	}()
	return nil
}

func serveConn(c *net.UnixConn) ([]string, bool) {
	defer c.Close()
	if !peerIsMe(c) {
		return nil, false
	}
	c.SetDeadline(time.Now().Add(ioTimeout))
	args, err := readFrame(c)
	if err != nil {
		return nil, false
	}
	if _, err := c.Write([]byte{ackByte}); err != nil {
		return nil, false
	}
	return args, true
}

func (s *instanceSys) close() error {
	var err error
	if s.ln != nil {
		err = s.ln.Close()
		s.wg.Wait()
		s.ln = nil
	}
	if s.lock != nil {
		s.lock.Close()
		s.lock = nil
	}
	return err
}
