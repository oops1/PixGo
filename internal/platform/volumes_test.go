package platform

import (
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseMounts(t *testing.T) {
	src := `/dev/sda1 / ext4 rw 0 0
proc /proc proc rw 0 0
/dev/sdb1 /media/user/My\040Disk vfat rw 0 0
/dev/sdc1 /mnt/data ext4 rw 0 0
srv:/share /mnt/share nfs4 rw 0 0
/dev/sdd1 /run/media/user/USB ext4 rw 0 0
/dev/sdd1 /run/media/user/USB ext4 rw 0 0
tmpfs /media tmpfs rw 0 0
short
`
	got := parseMounts(strings.NewReader(src))
	want := []Volume{
		{Path: "/media/user/My Disk", Name: "My Disk", Label: "My Disk", Kind: VolumeRemovable},
		{Path: "/mnt/data", Name: "data", Label: "data", Kind: VolumeFixed},
		{Path: "/mnt/share", Name: "share", Label: "share", Kind: VolumeNetwork},
		{Path: "/run/media/user/USB", Name: "USB", Label: "USB", Kind: VolumeRemovable},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestUnescapeMount(t *testing.T) {
	for in, want := range map[string]string{
		`/media/a\040b`:  "/media/a b",
		`/mnt/tab\011x`:  "/mnt/tab\tx",
		`/mnt/back\134x`: `/mnt/back\x`,
		`/mnt/plain`:     "/mnt/plain",
		`/mnt/bad\0zz`:   `/mnt/bad\0zz`,
		`/mnt/end\04`:    `/mnt/end\04`,
	} {
		if got := unescapeMount(in); got != want {
			t.Errorf("unescapeMount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVolumesWatch(t *testing.T) {
	var state atomic.Value
	state.Store([]Volume{{Path: "/", Kind: VolumeRoot}})
	v := osVolumes{list: func() []Volume { return state.Load().([]Volume) }, poll: 10 * time.Millisecond}
	changed := make(chan struct{}, 4)
	stop := v.Watch(func() { changed <- struct{}{} })
	defer stop()
	select {
	case <-changed:
		t.Fatal("reported a change without one")
	case <-time.After(60 * time.Millisecond):
	}
	state.Store([]Volume{{Path: "/", Kind: VolumeRoot}, {Path: "/mnt/x", Label: "x"}})
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("change not reported")
	}
	stop()
	stop()
}

func TestListVolumesNotEmpty(t *testing.T) {
	vs := NewVolumes().List()
	if len(vs) == 0 {
		t.Fatal("no volumes")
	}
	for _, v := range vs {
		if v.Path == "" || v.Name == "" {
			t.Errorf("incomplete volume %+v", v)
		}
	}
}
