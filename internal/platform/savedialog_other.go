//go:build !windows

package platform

func NewSaveDialog() SaveDialog { return unsupportedSaveDialog{} }
