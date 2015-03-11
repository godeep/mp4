package mp4

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrTruncatedHeader = errors.New("truncated header")
	ErrBadFormat       = errors.New("bad format")
)

type MP4 struct {
	Ftyp *FtypBox
	Moov *MoovBox
	Mdat io.Reader
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
	for {
		h, err = DecodeHeader(r)
		if err != nil {
			break
		}
		if h.Type != "mdat" {
			DecodeBox(h, r)
		} else {
			break
		}

	}
	return &MP4{
		Ftyp: ftyp.(*FtypBox),
		Moov: moov.(*MoovBox),
		Mdat: r,
	}, nil
}

func DecodeHeader(r io.Reader) (BoxHeader, error) {
	buf := make([]byte, BOX_HEADER_SIZE)
	n, err := r.Read(buf)
	if err != nil {
		return BoxHeader{}, err
	}
	// TODO: add error
	if n != BOX_HEADER_SIZE {
		return BoxHeader{}, ErrTruncatedHeader
	}
	return BoxHeader{string(buf[4:8]), int64(binary.BigEndian.Uint32(buf[0:4]))}, nil
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
	return nil
}
