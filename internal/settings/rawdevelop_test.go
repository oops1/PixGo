package settings

import "testing"

func TestRawDevelopDefaultsToOnAndSurvivesAFileFromAnOlderVersion(t *testing.T) {
	if !Defaults().RawDevelop {
		t.Fatal("DNG development is off by default")
	}
	old, err := Decode([]byte(`{"version":2,"theme":"dark"}`))
	if err != nil || !old.RawDevelop {
		t.Fatalf("a file without the key must keep the default: %v %v", err, old.RawDevelop)
	}
	off, err := Decode([]byte(`{"version":2,"rawDevelop":false}`))
	if err != nil || off.RawDevelop {
		t.Fatalf("an explicit false must stay: %v %v", err, off.RawDevelop)
	}
	data, err := off.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(data)
	if err != nil || back.RawDevelop {
		t.Fatalf("round trip: %v %v", err, back.RawDevelop)
	}
}

func TestRawDevelopIsStoredAndNotifiesSubscribers(t *testing.T) {
	st := NewMemory(Defaults())
	var seen []bool
	st.Subscribe(func(old, current Settings) { seen = append(seen, old.RawDevelop, current.RawDevelop) })
	st.Update(func(s *Settings) { s.RawDevelop = false })
	if st.Get().RawDevelop || len(seen) != 2 || !seen[0] || seen[1] {
		t.Fatalf("stored %v, seen %v", st.Get().RawDevelop, seen)
	}
}
