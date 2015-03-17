package mp4

import "io"

type Filter interface {
	FilterMoov(m *MoovBox) error
	FilterMdat(w io.Writer, m *MdatBox) error
}

type noopFilter struct{}

func Noop() *noopFilter {
	return &noopFilter{}
}

func (f *noopFilter) FilterMoov(m *MoovBox) error {
	return nil
}

func (f *noopFilter) FilterMdat(w io.Writer, m *MdatBox) error {
	err := EncodeHeader(m, w)
	if err == nil {
		_, err = io.Copy(w, m.r)
	}
	return err
}
