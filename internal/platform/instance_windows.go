//go:build windows

package platform

import (
	"errors"
	"fmt"
	"io"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procWaitNamedPipeW              = modkernel32.NewProc("WaitNamedPipeW")
	procGetNamedPipeServerProcessId = modkernel32.NewProc("GetNamedPipeServerProcessId")

	errStopped = errors.New("platform: stopped")
	errTimeout = errors.New("platform: i/o timeout")
)

type instanceSys struct {
	path string
	sd   *windows.SECURITY_DESCRIPTOR
	pipe windows.Handle
	stop windows.Handle
	done chan struct{}
}

func pipeName(name string) (string, *windows.SID, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", nil, err
	}
	sid := tu.User.Sid
	return `\\.\pipe\` + name + "-" + sid.String(), sid, nil
}

func createPipe(path string, sd *windows.SECURITY_DESCRIPTOR, first bool) (windows.Handle, error) {
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	flags := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	mode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE |
		windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	return windows.CreateNamedPipe(utf16(path), flags, mode,
		windows.PIPE_UNLIMITED_INSTANCES, 64<<10, 64<<10, 0, sa)
}

func (s *instanceSys) acquire(name string) (bool, error) {
	path, sid, err := pipeName(name)
	if err != nil {
		return false, err
	}

	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + sid.String() + ")")
	if err != nil {
		return false, err
	}
	h, err := createPipe(path, sd, true)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s.path, s.sd, s.pipe = path, sd, h
	return true, nil
}

func (s *instanceSys) listen(handle func([]string)) error {
	if s.pipe == 0 {
		return errNotPrimary
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	op, err := newIOOp()
	if err != nil {
		windows.CloseHandle(stop)
		return err
	}
	h := s.pipe
	s.pipe, s.stop, s.done = 0, stop, make(chan struct{})
	go s.serve(h, op, stop, s.done, handle)
	return nil
}

func (s *instanceSys) serve(h windows.Handle, op *ioOp, stop windows.Handle, done chan struct{}, handle func([]string)) {
	defer close(done)
	defer op.close()
	for {
		_, err := op.do(h, stop, windows.INFINITE, func(ov *windows.Overlapped) error {
			return windows.ConnectNamedPipe(h, ov)
		})
		if err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			windows.CloseHandle(h)
			if err == errStopped {
				return
			}

			if h, err = createPipe(s.path, s.sd, false); err != nil {
				return
			}
			continue
		}
		next, nerr := createPipe(s.path, s.sd, false)
		c := &pipeConn{h: h, stop: stop, op: op, deadline: time.Now().Add(ioTimeout)}
		args, rerr := readFrame(c)
		if rerr == nil {
			rerr = c.writeAll([]byte{ackByte})
		}
		if rerr == nil {

			c.deadline = time.Now().Add(time.Second)
			var b [1]byte
			c.Read(b[:])
		}
		windows.DisconnectNamedPipe(h)
		windows.CloseHandle(h)
		if rerr == nil {
			handle(args)
		}
		if errors.Is(rerr, errStopped) || nerr != nil {
			if nerr == nil {
				windows.CloseHandle(next)
			}
			return
		}
		h = next
	}
}

func (s *instanceSys) close() error {
	if s.stop != 0 {
		windows.SetEvent(s.stop)
		<-s.done
		windows.CloseHandle(s.stop)
		s.stop, s.done = 0, nil
	}
	if s.pipe != 0 {
		windows.CloseHandle(s.pipe)
		s.pipe = 0
	}
	return nil
}

func (s *instanceSys) send(name string, args []string) error {
	path, sid, err := pipeName(name)
	if err != nil {
		return err
	}
	until := time.Now().Add(2 * time.Second)
	var h windows.Handle
	for {
		h, err = windows.CreateFile(utf16(path), windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
			windows.OPEN_EXISTING,
			windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) || time.Now().After(until) {
			return err
		}
		procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(utf16(path))), 250)
	}
	defer windows.CloseHandle(h)
	if err := checkServer(h, sid); err != nil {
		return err
	}
	op, err := newIOOp()
	if err != nil {
		return err
	}
	defer op.close()
	c := &pipeConn{h: h, op: op, deadline: time.Now().Add(ioTimeout)}
	if err := writeFrame(c, args); err != nil {
		return err
	}
	return readAck(c)
}

func checkServer(h windows.Handle, me *windows.SID) error {
	var pid uint32
	if r, _, e := procGetNamedPipeServerProcessId.Call(uintptr(h), uintptr(unsafe.Pointer(&pid))); r == 0 {
		return fmt.Errorf("platform: pipe server: %w", e)
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("platform: pipe server: %w", err)
	}
	defer windows.CloseHandle(ph)
	var tok windows.Token
	if err := windows.OpenProcessToken(ph, windows.TOKEN_QUERY, &tok); err != nil {
		return fmt.Errorf("platform: pipe server: %w", err)
	}
	defer tok.Close()
	tu, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	if !windows.EqualSid(tu.User.Sid, me) {
		return fmt.Errorf("platform: pipe served by another user")
	}
	return nil
}

type ioOp struct {
	ov   windows.Overlapped
	done uint32
}

//go:noinline
func newIOOp() (*ioOp, error) {
	op := new(ioOp)
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	op.ov.HEvent = ev
	return op, nil
}

func (op *ioOp) close() { windows.CloseHandle(op.ov.HEvent) }

func (op *ioOp) do(h, stop windows.Handle, ms uint32, start func(*windows.Overlapped) error) (uint32, error) {
	ev := op.ov.HEvent
	op.ov = windows.Overlapped{HEvent: ev}
	windows.ResetEvent(ev)
	if err := start(&op.ov); err != nil && err != windows.ERROR_IO_PENDING {
		return 0, err
	}
	waits := []windows.Handle{ev}
	if stop != 0 {
		waits = append(waits, stop)
	}
	r, werr := windows.WaitForMultipleObjects(waits, false, ms)
	var n uint32
	if werr != nil || r != windows.WAIT_OBJECT_0 {
		windows.CancelIoEx(h, &op.ov)
		windows.GetOverlappedResult(h, &op.ov, &n, true)
		if werr == nil && r == windows.WAIT_OBJECT_0+1 {
			return 0, errStopped
		}
		return 0, errTimeout
	}
	err := windows.GetOverlappedResult(h, &op.ov, &n, false)
	return n, err
}

type pipeConn struct {
	h, stop  windows.Handle
	op       *ioOp
	deadline time.Time
}

func (c *pipeConn) left() uint32 {
	d := time.Until(c.deadline)
	if d <= 0 {
		return 0
	}
	return uint32(d / time.Millisecond)
}

//go:noinline
func heapBuf(n int) []byte { return make([]byte, n) }

func (c *pipeConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	buf := heapBuf(len(p))
	n, err := c.op.do(c.h, c.stop, c.left(), func(ov *windows.Overlapped) error {
		return windows.ReadFile(c.h, buf, &c.op.done, ov)
	})
	copy(p, buf[:n])
	if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) ||
		(err == nil && n == 0) {
		err = io.EOF
	}
	return int(n), err
}

func (c *pipeConn) Write(p []byte) (int, error) {
	if err := c.writeAll(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *pipeConn) writeAll(p []byte) error {
	buf := heapBuf(len(p))
	copy(buf, p)
	for len(buf) > 0 {
		n, err := c.op.do(c.h, c.stop, c.left(), func(ov *windows.Overlapped) error {
			return windows.WriteFile(c.h, buf, &c.op.done, ov)
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		buf = buf[n:]
	}
	return nil
}
