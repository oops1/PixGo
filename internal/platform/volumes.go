package platform

import (
	"bufio"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const volumePoll = 3 * time.Second

var mountRoots = []string{"/media/", "/mnt/", "/run/media/"}

var networkFSTypes = map[string]bool{
	"cifs": true, "smb3": true, "smbfs": true, "nfs": true, "nfs4": true, "fuse.sshfs": true,
	"davfs": true, "fuse.davfs2": true, "9p": true, "ceph": true, "glusterfs": true, "fuse.rclone": true,
}

func parseMounts(r io.Reader) []Volume {
	var out []Volume
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		mp := unescapeMount(f[1])
		for _, root := range mountRoots {
			if !strings.HasPrefix(mp, root) || len(mp) <= len(root) || seen[mp] {
				continue
			}
			seen[mp] = true
			kind := VolumeFixed
			switch {
			case len(f) > 2 && networkFSTypes[f[2]]:
				kind = VolumeNetwork
			case strings.HasPrefix(mp, "/media/") || strings.HasPrefix(mp, "/run/media/"):
				kind = VolumeRemovable
			}
			name := filepath.Base(mp)
			out = append(out, Volume{Path: mp, Name: name, Label: name, Kind: kind})
			break
		}
	}
	return out
}

func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func volumeSnapshot(vs []Volume) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString(v.Path)
		b.WriteByte(0)
		b.WriteString(v.Label)
		b.WriteByte(0)
		b.WriteString(strconv.Itoa(int(v.Kind)))
		b.WriteByte('\n')
	}
	return b.String()
}

type osVolumes struct {
	list func() []Volume
	poll time.Duration
}

func NewVolumes() Volumes { return osVolumes{list: listVolumes, poll: volumePoll} }

func (v osVolumes) List() []Volume { return v.list() }

func (v osVolumes) Watch(changed func()) func() {
	done := make(chan struct{})
	var once sync.Once
	last := volumeSnapshot(v.list())
	go func() {
		tk := time.NewTicker(v.poll)
		defer tk.Stop()
		for {
			select {
			case <-done:
				return
			case <-tk.C:
			}
			if cur := volumeSnapshot(v.list()); cur != last {
				last = cur
				changed()
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}
