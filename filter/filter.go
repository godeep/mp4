package filter

import (
	"io"

	"github.com/jfbus/mp4"
)

// A filter
type Filter interface {
	// Updates the moov box
	FilterMoov(m *mp4.MoovBox) error
	// Filters the Mdat data
	FilterMdat(w io.Writer, m *mp4.MdatBox) error
}

// Encode video, filtering the media using a filter
func EncodeFiltered(w io.Writer, m *mp4.MP4, f Filter) error {
	err := m.Ftyp.Encode(w)
	if err != nil {
		return err
	}
	err = f.FilterMoov(m.Moov)
	if err != nil {
		return err
	}
	err = m.Moov.Encode(w)
	if err != nil {
		return err
	}
	return f.FilterMdat(w, m.Mdat)
}
