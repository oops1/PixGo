package platform

import "image"

type unsupportedSaveDialog struct{}

func (unsupportedSaveDialog) Save(SaveOptions) (string, bool, error) {
	return "", false, ErrUnsupported
}

type unsupportedClipboard struct{}

func (unsupportedClipboard) SetImage(img *image.NRGBA, png []byte) error { return ErrUnsupported }

type unsupportedAssociations struct{}

func (unsupportedAssociations) OpenDefaultsSettings() error { return ErrUnsupported }

func (unsupportedAssociations) SetAsDefault() error { return ErrUnsupported }
