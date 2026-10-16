package main

import (
	"encoding/json"
	"os"
	"testing"
)

type winres struct {
	Manifest map[string]map[string]struct {
		DPIAwareness string `json:"dpi-awareness"`
		LongPaths    bool   `json:"long-path-aware"`
	} `json:"RT_MANIFEST"`
}

func TestWindowsManifestDeclaresPerMonitorDPIAwareness(t *testing.T) {
	data, err := os.ReadFile("winres/winres.json")
	if err != nil {
		t.Fatal(err)
	}
	var res winres
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	m, ok := res.Manifest["#1"]["0409"]
	if !ok {
		t.Fatal("the manifest resource is missing")
	}
	if m.DPIAwareness != "per monitor v2" {
		t.Errorf("dpi-awareness is %q: go-winres then writes a system-DPI manifest, the window is stretched on monitors with another scale and never gets WM_DPICHANGED", m.DPIAwareness)
	}
	if !m.LongPaths {
		t.Error("long paths are not enabled")
	}
}
