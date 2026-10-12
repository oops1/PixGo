//go:build !windows && !linux

package platform

func NewAssociations() Associations { return unsupportedAssociations{} }
