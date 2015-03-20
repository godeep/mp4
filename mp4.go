package mp4

import "io"

// A MPEG-4 content
//
// A MPEG-4 media contains three main boxes :
//
//   ftyp : the file type box
//   moov : the movie box (meta-data)
//   mdat : the media data (chunks and samples)
//
// Other boxes can also be present (pdin, moof, mfra, free, ...), but are not decoded.
type MP4 struct {
	Ftyp *FtypBox
	Moov *MoovBox
	Mdat *MdatBox
}

func Decode(r io.Reader) (*MP4, error) {
	h, err := DecodeHeader(r)
	if err != nil {
		return nil, err
	}
	if h.Type != "ftyp" {
		return nil, ErrBadFormat
	}
	ftyp, err := DecodeBox(h, r)
	if err != nil {
		return nil, err
	}
	h, err = DecodeHeader(r)
	if h.Type != "moov" {
		return nil, ErrBadFormat
	}
	moov, err := DecodeBox(h, r)
	if err != nil {
		return nil, err
	}
	v := &MP4{
		Ftyp: ftyp.(*FtypBox),
		Moov: moov.(*MoovBox),
	}
	for {
		h, err = DecodeHeader(r)
		if err != nil {
			break
		}
		if h.Type != "mdat" {
			DecodeBox(h, r)
		} else {
			mdat, err := DecodeBox(h, r)
			if err != nil {
				return nil, err
			}
			v.Mdat = mdat.(*MdatBox)
			v.Mdat.ContentSize = h.Size - BoxHeaderSize
			break
		}

	}
	return v, nil
}

func (m *MP4) Dump() {
	m.Ftyp.Dump()
	m.Moov.Dump()
}

func (m *MP4) Encode(w io.Writer) error {
	err := m.Ftyp.Encode(w)
	if err != nil {
		return err
	}
	err = m.Moov.Encode(w)
	if err != nil {
		return err
	}
	return m.Mdat.Encode(w)
}
