//go:build !windows

package platform

func NewAssociations() Associations { return unsupportedAssociations{} }
