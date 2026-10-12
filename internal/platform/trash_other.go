//go:build !windows && !linux

package platform

import "context"

type noTrash struct{}

func NewTrash() Trash { return noTrash{} }

func (noTrash) Available(string) bool { return false }

func (noTrash) Trash(context.Context, string) error { return ErrUnavailable }
