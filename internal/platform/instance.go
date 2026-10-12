package platform

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	maxFrame  = 1 << 20
	ioTimeout = 5 * time.Second
	ackByte   = 0x06
)

var errNotPrimary = errors.New("platform: instance is not primary")

type instance struct {
	name string

	mu      sync.Mutex
	primary bool
	sys     instanceSys
}

func NewInstance(name string) Instance {
	return &instance{name: sanitizeName(name)}
}

func (i *instance) Acquire() (primary bool, err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.primary {
		return true, nil
	}
	p, err := i.sys.acquire(i.name)
	i.primary = p
	return p, err
}

func (i *instance) Send(args []string) error {
	return i.sys.send(i.name, args)
}

func (i *instance) Listen(handle func(args []string)) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.primary {
		return errNotPrimary
	}
	return i.sys.listen(handle)
}

func (i *instance) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.primary = false
	return i.sys.close()
}

func sanitizeName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, s)
	s = strings.TrimLeft(s, ".")
	if s == "" {
		s = "pixgo"
	}
	return s
}

func writeFrame(w io.Writer, args []string) error {
	if args == nil {
		args = []string{}
	}
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	if len(body) > maxFrame {
		return fmt.Errorf("platform: message too large")
	}
	buf := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(buf, uint32(len(body)))
	copy(buf[4:], body)
	_, err = w.Write(buf)
	return err
}

func readFrame(r io.Reader) ([]string, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, fmt.Errorf("platform: message too large")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var args []string
	if err := json.Unmarshal(body, &args); err != nil {
		return nil, err
	}
	return args, nil
}

func readAck(r io.Reader) error {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return fmt.Errorf("platform: no receipt: %w", err)
	}
	if b[0] != ackByte {
		return fmt.Errorf("platform: bad receipt")
	}
	return nil
}
