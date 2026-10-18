//go:build !windows

package platform

type noDisplayProfile struct{}

func NewDisplayProfile() DisplayProfile { return noDisplayProfile{} }

func (noDisplayProfile) Profile() ([]byte, string) { return nil, "" }
