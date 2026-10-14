package view

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/viewer"
)

func TestPreviewFlagFollowsWhatIsShown(t *testing.T) {
	steps := []struct {
		name string
		do   func(c *Canvas)
		want bool
	}{
		{"a fresh canvas", func(c *Canvas) {}, false},
		{"flag without an image", func(c *Canvas) { c.SetPreview(true) }, false},
		{"preview image", func(c *Canvas) {
			c.SetPreview(true)
			c.SetImage(source(40, 30), codec.OrientNormal, viewer.ZoomFit)
		}, true},
		{"preview image kept while the view is kept", func(c *Canvas) {
			c.SetPreview(true)
			c.SetImageKeepView(source(40, 30), codec.OrientNormal)
		}, true},
		{"ordinary image", func(c *Canvas) {
			c.SetPreview(false)
			c.SetImage(source(40, 30), codec.OrientNormal, viewer.ZoomFit)
		}, false},
		{"next image is not a preview", func(c *Canvas) {
			c.SetPreview(true)
			c.SetImage(source(40, 30), codec.OrientNormal, viewer.ZoomFit)
			c.SetPreview(false)
			c.SetImage(source(50, 30), codec.OrientNormal, viewer.ZoomFit)
		}, false},
		{"cleared canvas forgets the flag", func(c *Canvas) {
			c.SetPreview(true)
			c.SetImage(source(40, 30), codec.OrientNormal, viewer.ZoomFit)
			c.Clear()
		}, false},
		{"flag set again after a clear needs an image", func(c *Canvas) {
			c.SetPreview(true)
			c.SetImage(source(40, 30), codec.OrientNormal, viewer.ZoomFit)
			c.Clear()
			c.SetPreview(true)
		}, false},
		{"animation frame swap keeps the flag", func(c *Canvas) {
			c.SetPreview(true)
			c.SetImage(source(40, 30), codec.OrientNormal, viewer.ZoomFit)
			c.ReplaceFrame(source(40, 30))
		}, true},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			c := newRig(t).c

			tc.do(c)

			if got := c.Preview(); got != tc.want {
				t.Fatalf("Preview() = %v, want %v", got, tc.want)
			}
		})
	}
}
