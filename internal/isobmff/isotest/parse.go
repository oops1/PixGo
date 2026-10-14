package isotest

import (
	"bytes"
	"context"
	"fmt"

	"github.com/oops1/PixGo/internal/isobmff"
)

func Parse(data []byte) (File, error) {
	parsed, err := isobmff.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return File{}, err
	}
	file := File{
		Major:      parsed.Brands.Major,
		Minor:      parsed.Brands.Minor,
		Compatible: parsed.Brands.Compatible,
		Primary:    parsed.PrimaryID,
	}
	for _, id := range parsed.Order {
		item := parsed.Items[id]
		content, err := parsed.ReadItem(item, int64(len(data)))
		if err != nil {
			return File{}, fmt.Errorf("item %d: %w", id, err)
		}
		built := Item{ID: id, Type: item.Type, Data: content, Hidden: item.Hidden, ContentType: item.ContentType}
		for _, assoc := range item.Associations {
			property := parsed.Properties[assoc.Index]
			built.Props = append(built.Props, Prop{Box: Box(property.Type, property.Payload), Essential: assoc.Essential})
		}
		file.Items = append(file.Items, built)
	}
	for _, ref := range parsed.References {
		file.Refs = append(file.Refs, Ref{Type: ref.Type, From: ref.From, To: ref.To})
	}
	return file, nil
}
