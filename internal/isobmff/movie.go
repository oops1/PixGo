package isobmff

const (
	trackHeaderSkip = 52
	fixedPointShift = 16
)

func (f *File) parseMovie(payload []byte) error {
	return eachBox(payload, func(kind string, body []byte) error {
		if kind != "trak" {
			return nil
		}
		if len(f.Tracks) >= maxTracks {
			return tooLarge("heif: more than %d tracks", maxTracks)
		}
		track, err := parseTrack(body)
		if err != nil {
			return err
		}
		f.Tracks = append(f.Tracks, track)
		return nil
	})
}

func parseTrack(body []byte) (Track, error) {
	var track Track
	err := eachBox(body, func(kind string, payload []byte) error {
		switch kind {
		case "tkhd":
			return parseTrackHeader(payload, &track)
		case "mdia":
			return eachBox(payload, func(kind string, payload []byte) error {
				switch kind {
				case "hdlr":
					c := newCursor(payload)
					c.fullBox()
					c.skip(4)
					track.Handler = c.fourCC()
					if c.failed {
						return corrupt("heif: truncated hdlr box")
					}
				case "minf":
					return eachBox(payload, func(kind string, payload []byte) error {
						if kind != "stbl" {
							return nil
						}
						return eachBox(payload, func(kind string, payload []byte) error {
							if kind != "stsz" {
								return nil
							}
							return parseSampleSizes(payload, &track)
						})
					})
				}
				return nil
			})
		}
		return nil
	})
	return track, err
}

func parseTrackHeader(payload []byte, track *Track) error {
	c := newCursor(payload)
	version, _ := c.fullBox()
	if version == 1 {
		c.skip(8 + 8 + 4 + 4 + 8)
	} else {
		c.skip(4 + 4 + 4 + 4 + 4)
	}
	c.skip(trackHeaderSkip)
	width, height := c.u32(), c.u32()
	if c.failed {
		return corrupt("heif: truncated tkhd box")
	}
	track.Width, track.Height = int(width>>fixedPointShift), int(height>>fixedPointShift)
	return nil
}

func parseSampleSizes(payload []byte, track *Track) error {
	c := newCursor(payload)
	c.fullBox()
	uniformSize := c.u32()
	count := c.u32()
	if c.failed {
		return corrupt("heif: truncated stsz box")
	}
	if count > maxSamples {
		return tooLarge("heif: %d samples in one track", count)
	}
	if uniformSize == 0 && c.remaining()/4 < int(count) {
		return corrupt("heif: stsz lists fewer than %d sizes", count)
	}
	track.Samples = int(count)
	return nil
}

func (f *File) Sequence() (Track, bool) {
	for _, track := range f.Tracks {
		if track.Handler == "pict" && track.Samples > 0 {
			return track, true
		}
	}
	return Track{}, false
}
