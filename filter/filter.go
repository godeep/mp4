package filter

import (
	"io"

	"github.com/jfbus/mp4"
)

type Filter interface {
	FilterMoov(m *mp4.MoovBox) error
	FilterMdat(w io.Writer, m *mp4.MdatBox) error
}

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
