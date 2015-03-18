package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"time"
)

const (
	BoxHeaderSize = 8
)

var (
	ErrUnknownBoxType  = errors.New("unknown box type")
	ErrTruncatedHeader = errors.New("truncated header")
	ErrBadFormat       = errors.New("bad format")
)

var decoders map[string]BoxDecoder

func init() {
	decoders = map[string]BoxDecoder{
		"ftyp": DecodeFtyp,
		"moov": DecodeMoov,
		"mvhd": DecodeMvhd,
		"iods": DecodeIods,
		"trak": DecodeTrak,
		"udta": DecodeUdta,
		"tkhd": DecodeTkhd,
		"edts": DecodeEdts,
		"elst": DecodeElst,
		"mdia": DecodeMdia,
		"minf": DecodeMinf,
		"mdhd": DecodeMdhd,
		"hdlr": DecodeHdlr,
		"vmhd": DecodeVmhd,
		"smhd": DecodeSmhd,
		"dinf": DecodeDinf,
		"dref": DecodeDref,
		"stbl": DecodeStbl,
		"stco": DecodeStco,
		"stsc": DecodeStsc,
		"stsz": DecodeStsz,
		"ctts": DecodeCtts,
		"stsd": DecodeStsd,
		"stts": DecodeStts,
		"stss": DecodeStss,
		"meta": DecodeMeta,
		"mdat": DecodeMdat,
	}
}

type BoxHeader struct {
	Type string
	Size uint32
}

func DecodeHeader(r io.Reader) (BoxHeader, error) {
	buf := make([]byte, BoxHeaderSize)
	n, err := r.Read(buf)
	if err != nil {
		return BoxHeader{}, err
	}
	if n != BoxHeaderSize {
		return BoxHeader{}, ErrTruncatedHeader
	}
	return BoxHeader{string(buf[4:8]), binary.BigEndian.Uint32(buf[0:4])}, nil
}

func EncodeHeader(b Box, w io.Writer) error {
	buf := make([]byte, BoxHeaderSize)
	binary.BigEndian.PutUint32(buf, uint32(b.Size()))
	strtobuf(buf[4:], b.Type(), 4)
	_, err := w.Write(buf)
	return err
}

type Box interface {
	Type() string
	Size() int
}

type BoxDecoder func(r io.Reader) (Box, error)

func DecodeBox(h BoxHeader, r io.Reader) (Box, error) {
	fmt.Printf("Found %s with size %d\n", h.Type, h.Size)
	d := decoders[h.Type]
	if d == nil {
		log.Printf("Error while decoding %s : unknown box type", h.Type)
		return nil, ErrUnknownBoxType
	}
	b, err := d(io.LimitReader(r, int64(h.Size-BoxHeaderSize)))
	if err != nil {
		log.Printf("Error while decoding %s : %s", h.Type, err)
		return nil, err
	}
	return b, nil
}

func DecodeContainer(r io.Reader) ([]Box, error) {
	l := []Box{}
	for {
		h, err := DecodeHeader(r)
		if err == io.EOF {
			return l, nil
		}
		if err != nil {
			return l, err
		}
		b, err := DecodeBox(h, r)
		if err != nil {
			return l, err
		}
		l = append(l, b)
	}
}

type FtypBox struct {
	MajorBrand       string
	MinorVersion     []byte
	CompatibleBrands []string
}

func DecodeFtyp(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &FtypBox{
		MajorBrand:       string(data[0:4]),
		MinorVersion:     data[4:8],
		CompatibleBrands: []string{},
	}
	if len(data) > 8 {
		for i := 8; i < len(data); i += 4 {
			b.CompatibleBrands = append(b.CompatibleBrands, string(data[i:i+4]))
		}
	}
	return b, nil
}

func (b *FtypBox) Type() string {
	return "ftyp"
}

func (b *FtypBox) Size() int {
	return BoxHeaderSize + 8 + 4*len(b.CompatibleBrands)
}

func (b *FtypBox) Dump() {
	fmt.Printf("File Type: %s\n", b.MajorBrand)
}

func (b *FtypBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	strtobuf(buf, b.MajorBrand, 4)
	copy(buf[4:], b.MinorVersion)
	for i, c := range b.CompatibleBrands {
		strtobuf(buf[8+i*4:], c, 4)
	}
	_, err = w.Write(buf)
	return err
}

type MoovBox struct {
	Mvhd *MvhdBox
	Iods *IodsBox
	Trak []*TrakBox
	Udta *UdtaBox
}

func DecodeMoov(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	m := &MoovBox{}
	for _, b := range l {
		switch b.Type() {
		case "mvhd":
			m.Mvhd = b.(*MvhdBox)
		case "iods":
			m.Iods = b.(*IodsBox)
		case "trak":
			m.Trak = append(m.Trak, b.(*TrakBox))
		case "udta":
			m.Udta = b.(*UdtaBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return m, err
}

func (b *MoovBox) Type() string {
	return "moov"
}

func (b *MoovBox) Size() int {
	sz := b.Mvhd.Size()
	if b.Iods != nil {
		sz += b.Iods.Size()
	}
	for _, t := range b.Trak {
		sz += t.Size()
	}
	if b.Udta != nil {
		sz += b.Udta.Size()
	}
	return sz + BoxHeaderSize
}

func (b *MoovBox) Dump() {
	b.Mvhd.Dump()
	for i, t := range b.Trak {
		fmt.Println("Track", i)
		t.Dump()
	}
}

func (b *MoovBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	err = b.Mvhd.Encode(w)
	if err != nil {
		return err
	}
	if b.Iods != nil {
		err = b.Iods.Encode(w)
		if err != nil {
			return err
		}
	}
	for _, t := range b.Trak {
		err = t.Encode(w)
		if err != nil {
			return err
		}
	}
	if b.Udta != nil {
		return b.Udta.Encode(w)
	}
	return nil
}

type MvhdBox struct {
	Version          byte
	Flags            [3]byte
	CreationTime     uint32
	ModificationTime uint32
	Timescale        uint32
	Duration         uint32
	NextTrackId      uint32
	Rate             Fixed32
	Volume           Fixed16
	notDecoded       []byte
}

func DecodeMvhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &MvhdBox{
		Version:          data[0],
		Flags:            [3]byte{data[1], data[2], data[3]},
		CreationTime:     binary.BigEndian.Uint32(data[4:8]),
		ModificationTime: binary.BigEndian.Uint32(data[8:12]),
		Timescale:        binary.BigEndian.Uint32(data[12:16]),
		Duration:         binary.BigEndian.Uint32(data[16:20]),
		Rate:             fixed32(data[20:24]),
		Volume:           fixed16(data[24:26]),
		notDecoded:       data[26:],
	}, nil
}

func (b *MvhdBox) Type() string {
	return "mvhd"
}

func (b *MvhdBox) Size() int {
	return BoxHeaderSize + 26 + len(b.notDecoded)
}

func (b *MvhdBox) Dump() {
	fmt.Printf("Movie Header:\n Timescale: %d units/sec\n Duration: %d units (%s)\n Rate: %s\n Volume: %s\n", b.Timescale, b.Duration, time.Duration(b.Duration/b.Timescale)*time.Second, b.Rate, b.Volume)
}

func (b *MvhdBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.CreationTime)
	binary.BigEndian.PutUint32(buf[8:], b.ModificationTime)
	binary.BigEndian.PutUint32(buf[12:], b.Timescale)
	binary.BigEndian.PutUint32(buf[16:], b.Duration)
	binary.BigEndian.PutUint32(buf[20:], uint32(b.Rate))
	binary.BigEndian.PutUint16(buf[24:], uint16(b.Volume))
	copy(buf[26:], b.notDecoded)
	_, err = w.Write(buf)
	return err
}

type IodsBox struct {
	notDecoded []byte
}

func DecodeIods(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &IodsBox{
		notDecoded: data,
	}, nil
}

func (b *IodsBox) Type() string {
	return "iods"
}

func (b *IodsBox) Size() int {
	return BoxHeaderSize + len(b.notDecoded)
}

func (b *IodsBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	_, err = w.Write(b.notDecoded)
	return err
}

type TrakBox struct {
	Tkhd *TkhdBox
	Mdia *MdiaBox
	Edts *EdtsBox
}

func DecodeTrak(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	t := &TrakBox{}
	for _, b := range l {
		switch b.Type() {
		case "tkhd":
			t.Tkhd = b.(*TkhdBox)
		case "mdia":
			t.Mdia = b.(*MdiaBox)
		case "edts":
			t.Edts = b.(*EdtsBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return t, nil
}

func (b *TrakBox) Type() string {
	return "trak"
}

func (b *TrakBox) Size() int {
	sz := b.Tkhd.Size()
	sz += b.Mdia.Size()
	if b.Edts != nil {
		sz += b.Edts.Size()
	}
	return sz + BoxHeaderSize
}

func (b *TrakBox) Dump() {
	b.Tkhd.Dump()
	if b.Edts != nil {
		b.Edts.Dump()
	}
	b.Mdia.Dump()
}

func (b *TrakBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	err = b.Tkhd.Encode(w)
	if err != nil {
		return err
	}
	if b.Edts != nil {
		err = b.Edts.Encode(w)
		if err != nil {
			return err
		}
	}
	return b.Mdia.Encode(w)
}

type TkhdBox struct {
	Version          byte
	Flags            [3]byte
	CreationTime     uint32
	ModificationTime uint32
	TrackId          uint32
	Duration         uint32
	Layer            uint16
	AlternateGroup   uint16 // This should really be int16 but not sure how to parse
	Volume           Fixed16
	Matrix           []byte
	Width, Height    Fixed32
}

func DecodeTkhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &TkhdBox{
		Version:          data[0],
		Flags:            [3]byte{data[1], data[2], data[3]},
		CreationTime:     binary.BigEndian.Uint32(data[4:8]),
		ModificationTime: binary.BigEndian.Uint32(data[8:12]),
		TrackId:          binary.BigEndian.Uint32(data[12:16]),
		Volume:           fixed16(data[36:38]),
		Duration:         binary.BigEndian.Uint32(data[20:24]),
		Layer:            binary.BigEndian.Uint16(data[32:34]),
		AlternateGroup:   binary.BigEndian.Uint16(data[34:36]),
		Matrix:           data[40:76],
		Width:            fixed32(data[76:80]),
		Height:           fixed32(data[80:84]),
	}, nil
}

func (b *TkhdBox) Type() string {
	return "tkhd"
}

func (b *TkhdBox) Size() int {
	return BoxHeaderSize + 84
}

func (b *TkhdBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.CreationTime)
	binary.BigEndian.PutUint32(buf[8:], b.ModificationTime)
	binary.BigEndian.PutUint32(buf[12:], b.TrackId)
	binary.BigEndian.PutUint32(buf[20:], b.Duration)
	binary.BigEndian.PutUint16(buf[32:], b.Layer)
	binary.BigEndian.PutUint16(buf[34:], b.AlternateGroup)
	putFixed16(buf[36:], b.Volume)
	copy(buf[40:], b.Matrix)
	putFixed32(buf[76:], b.Width)
	putFixed32(buf[80:], b.Height)
	_, err = w.Write(buf)
	return err
}

func (b *TkhdBox) Dump() {
	fmt.Println("Track Header:")
	fmt.Printf(" Duration: %d units\n WxH: %sx%s\n", b.Duration, b.Width, b.Height)
}

type EdtsBox struct {
	Elst *ElstBox
}

func DecodeEdts(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	e := &EdtsBox{}
	for _, b := range l {
		switch b.Type() {
		case "elst":
			e.Elst = b.(*ElstBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return e, nil
}

func (b *EdtsBox) Type() string {
	return "edts"
}

func (b *EdtsBox) Size() int {
	return BoxHeaderSize + b.Elst.Size()
}

func (b *EdtsBox) Dump() {
	b.Elst.Dump()
}

func (b *EdtsBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	return b.Elst.Encode(w)
}

type ElstBox struct {
	Version                             byte
	Flags                               [3]byte
	SegmentDuration, MediaTime          []uint32
	MediaRateInteger, MediaRateFraction []uint16 // This should really be int16 but not sure how to parse
}

func DecodeElst(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &ElstBox{
		Version:           data[0],
		Flags:             [3]byte{data[1], data[2], data[3]},
		SegmentDuration:   []uint32{},
		MediaTime:         []uint32{},
		MediaRateInteger:  []uint16{},
		MediaRateFraction: []uint16{},
	}
	ec := binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(ec); i++ {
		sd := binary.BigEndian.Uint32(data[(8 + 12*i):(12 + 12*i)])
		mt := binary.BigEndian.Uint32(data[(12 + 12*i):(16 + 12*i)])
		mri := binary.BigEndian.Uint16(data[(16 + 12*i):(18 + 12*i)])
		mrf := binary.BigEndian.Uint16(data[(18 + 12*i):(20 + 12*i)])
		b.SegmentDuration = append(b.SegmentDuration, sd)
		b.MediaTime = append(b.MediaTime, mt)
		b.MediaRateInteger = append(b.MediaRateInteger, mri)
		b.MediaRateFraction = append(b.MediaRateFraction, mrf)
	}
	return b, nil
}

func (b *ElstBox) Type() string {
	return "elst"
}

func (b *ElstBox) Size() int {
	return BoxHeaderSize + 8 + len(b.SegmentDuration)*12
}

func (b *ElstBox) Dump() {
	fmt.Println("Segment Duration:")
	for i, d := range b.SegmentDuration {
		fmt.Printf(" #%d: %d units\n", i, d)
	}
}

func (b *ElstBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := make([]byte, b.Size()-BoxHeaderSize)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], uint32(len(b.SegmentDuration)))
	for i := range b.SegmentDuration {
		binary.BigEndian.PutUint32(buf[8+12*i:], b.SegmentDuration[i])
		binary.BigEndian.PutUint32(buf[12+12*i:], b.MediaTime[i])
		binary.BigEndian.PutUint16(buf[16+12*i:], b.MediaRateInteger[i])
		binary.BigEndian.PutUint16(buf[18+12*i:], b.MediaRateFraction[i])
	}
	_, err = w.Write(buf)
	return err
}

type MdiaBox struct {
	Mdhd *MdhdBox
	Hdlr *HdlrBox
	Minf *MinfBox
}

func DecodeMdia(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	m := &MdiaBox{}
	for _, b := range l {
		switch b.Type() {
		case "mdhd":
			m.Mdhd = b.(*MdhdBox)
		case "hdlr":
			m.Hdlr = b.(*HdlrBox)
		case "minf":
			m.Minf = b.(*MinfBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return m, nil
}

func (b *MdiaBox) Type() string {
	return "mdia"
}

func (b *MdiaBox) Size() int {
	sz := b.Mdhd.Size()
	if b.Hdlr != nil {
		sz += b.Hdlr.Size()
	}
	if b.Minf != nil {
		sz += b.Minf.Size()
	}
	return sz + BoxHeaderSize
}

func (b *MdiaBox) Dump() {
	b.Mdhd.Dump()
	if b.Minf != nil {
		b.Minf.Dump()
	}
}

func (b *MdiaBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	err = b.Mdhd.Encode(w)
	if err != nil {
		return err
	}
	if b.Hdlr != nil {
		err = b.Hdlr.Encode(w)
		if err != nil {
			return err
		}
	}
	return b.Minf.Encode(w)
}

type MdhdBox struct {
	Version          byte
	Flags            [3]byte
	CreationTime     uint32
	ModificationTime uint32
	Timescale        uint32
	Duration         uint32
	Language         uint16 // Combine 1-bit padding w/ 15-bit Language data
}

func DecodeMdhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &MdhdBox{
		Version:          data[0],
		Flags:            [3]byte{data[1], data[2], data[3]},
		CreationTime:     binary.BigEndian.Uint32(data[4:8]),
		ModificationTime: binary.BigEndian.Uint32(data[8:12]),
		Timescale:        binary.BigEndian.Uint32(data[12:16]),
		Duration:         binary.BigEndian.Uint32(data[16:20]),
		Language:         binary.BigEndian.Uint16(data[20:22]),
	}, nil
}

func (b *MdhdBox) Type() string {
	return "mdhd"
}

func (b *MdhdBox) Size() int {
	return BoxHeaderSize + 24
}

func (b *MdhdBox) Dump() {
	fmt.Printf("Media Header:\n Timescale: %d units/sec\n Duration: %d units (%s)\n", b.Timescale, b.Duration, time.Duration(b.Duration/b.Timescale)*time.Second)

}

func (b *MdhdBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.CreationTime)
	binary.BigEndian.PutUint32(buf[8:], b.ModificationTime)
	binary.BigEndian.PutUint32(buf[12:], b.Timescale)
	binary.BigEndian.PutUint32(buf[16:], b.Duration)
	binary.BigEndian.PutUint16(buf[20:], b.Language)
	_, err = w.Write(buf)
	return err
}

type HdlrBox struct {
	Version     byte
	Flags       [3]byte
	PreDefined  uint32
	HandlerType string
	TrackName   string
}

func DecodeHdlr(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &HdlrBox{
		Version:     data[0],
		Flags:       [3]byte{data[1], data[2], data[3]},
		PreDefined:  binary.BigEndian.Uint32(data[4:8]),
		HandlerType: string(data[8:12]),
		TrackName:   string(data[24:]),
	}, nil
}

func (b *HdlrBox) Type() string {
	return "hdlr"
}

func (b *HdlrBox) Size() int {
	return BoxHeaderSize + 24 + len(b.TrackName)
}

func (b *HdlrBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.PreDefined)
	strtobuf(buf[8:], b.HandlerType, 4)
	strtobuf(buf[24:], b.TrackName, len(b.TrackName))
	_, err = w.Write(buf)
	return err
}

type MinfBox struct {
	Vmhd *VmhdBox
	Smhd *SmhdBox
	Stbl *StblBox
	Dinf *DinfBox
	Hdlr *HdlrBox
}

func DecodeMinf(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	m := &MinfBox{}
	for _, b := range l {
		switch b.Type() {
		case "vmhd":
			m.Vmhd = b.(*VmhdBox)
		case "smhd":
			m.Smhd = b.(*SmhdBox)
		case "stbl":
			m.Stbl = b.(*StblBox)
		case "dinf":
			m.Dinf = b.(*DinfBox)
		case "hdlr":
			m.Hdlr = b.(*HdlrBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return m, nil
}

func (b *MinfBox) Type() string {
	return "minf"
}

func (b *MinfBox) Size() int {
	sz := 0
	if b.Vmhd != nil {
		sz += b.Vmhd.Size()
	}
	if b.Smhd != nil {
		sz += b.Smhd.Size()
	}
	sz += b.Stbl.Size()
	if b.Dinf != nil {
		sz += b.Dinf.Size()
	}
	if b.Hdlr != nil {
		sz += b.Hdlr.Size()
	}
	return sz + BoxHeaderSize
}

func (b *MinfBox) Dump() {
	b.Stbl.Dump()
}

func (b *MinfBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	if b.Vmhd != nil {
		err = b.Vmhd.Encode(w)
		if err != nil {
			return err
		}
	}
	if b.Smhd != nil {
		err = b.Smhd.Encode(w)
		if err != nil {
			return err
		}
	}
	err = b.Dinf.Encode(w)
	if err != nil {
		return err
	}
	err = b.Stbl.Encode(w)
	if err != nil {
		return err
	}
	if b.Hdlr != nil {
		return b.Hdlr.Encode(w)
	}
	return nil
}

type VmhdBox struct {
	Version      byte
	Flags        [3]byte
	GraphicsMode uint16
	OpColor      [3]uint16
}

func DecodeVmhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &VmhdBox{
		Version:      data[0],
		Flags:        [3]byte{data[1], data[2], data[3]},
		GraphicsMode: binary.BigEndian.Uint16(data[4:6]),
	}
	for i := 0; i < 3; i++ {
		b.OpColor[i] = binary.BigEndian.Uint16(data[(6 + 2*i):(8 + 2*i)])
	}
	return b, nil
}

func (b *VmhdBox) Type() string {
	return "vmhd"
}

func (b *VmhdBox) Size() int {
	return BoxHeaderSize + 12
}

func (b *VmhdBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint16(buf[4:], b.GraphicsMode)
	for i := 0; i < 3; i++ {
		binary.BigEndian.PutUint16(buf[6+2*i:], b.OpColor[i])
	}
	_, err = w.Write(buf)
	return err
}

type SmhdBox struct {
	Version byte
	Flags   [3]byte
	Balance uint16 // This should really be int16 but not sure how to parse
}

func DecodeSmhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &SmhdBox{
		Version: data[0],
		Flags:   [3]byte{data[1], data[2], data[3]},
		Balance: binary.BigEndian.Uint16(data[4:6]),
	}, nil
}

func (b *SmhdBox) Type() string {
	return "smhd"
}

func (b *SmhdBox) Size() int {
	return BoxHeaderSize + 8
}

func (b *SmhdBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint16(buf[4:], b.Balance)
	_, err = w.Write(buf)
	return err
}

type StblBox struct {
	Stsd *StsdBox
	Stts *SttsBox
	Stss *StssBox
	Stsc *StscBox
	Stsz *StszBox
	Stco *StcoBox
	Ctts *CttsBox
}

func DecodeStbl(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	s := &StblBox{}
	for _, b := range l {
		switch b.Type() {
		case "stsd":
			s.Stsd = b.(*StsdBox)
		case "stts":
			s.Stts = b.(*SttsBox)
		case "stsc":
			s.Stsc = b.(*StscBox)
		case "stss":
			s.Stss = b.(*StssBox)
		case "stsz":
			s.Stsz = b.(*StszBox)
		case "stco":
			s.Stco = b.(*StcoBox)
		case "ctts":
			s.Ctts = b.(*CttsBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return s, nil
}

func (b *StblBox) Type() string {
	return "stbl"
}

func (b *StblBox) Size() int {
	sz := b.Stsd.Size()
	if b.Stts != nil {
		sz += b.Stts.Size()
	}
	if b.Stss != nil {
		sz += b.Stss.Size()
	}
	if b.Stsc != nil {
		sz += b.Stsc.Size()
	}
	if b.Stsz != nil {
		sz += b.Stsz.Size()
	}
	if b.Stco != nil {
		sz += b.Stco.Size()
	}
	if b.Ctts != nil {
		sz += b.Ctts.Size()
	}
	return sz + BoxHeaderSize
}

func (b *StblBox) Dump() {
	if b.Stsc != nil {
		b.Stsc.Dump()
	}
	if b.Stts != nil {
		b.Stts.Dump()
	}
	if b.Stsz != nil {
		b.Stsz.Dump()
	}
	if b.Stss != nil {
		b.Stss.Dump()
	}
	if b.Stco != nil {
		b.Stco.Dump()
	}
}

func (b *StblBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	err = b.Stsd.Encode(w)
	if err != nil {
		return err
	}
	err = b.Stts.Encode(w)
	if err != nil {
		return err
	}
	if b.Stss != nil {
		err = b.Stss.Encode(w)
		if err != nil {
			return err
		}
	}
	err = b.Stsc.Encode(w)
	if err != nil {
		return err
	}
	err = b.Stsz.Encode(w)
	if err != nil {
		return err
	}
	err = b.Stco.Encode(w)
	if err != nil {
		return err
	}
	if b.Ctts != nil {
		return b.Ctts.Encode(w)
	}
	return nil
}

type StsdBox struct {
	Version    byte
	Flags      [3]byte
	notDecoded []byte
}

func DecodeStsd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &StsdBox{
		Version:    data[0],
		Flags:      [3]byte{data[1], data[2], data[3]},
		notDecoded: data[4:],
	}, nil
}

func (b *StsdBox) Type() string {
	return "stsd"
}

func (b *StsdBox) Size() int {
	return BoxHeaderSize + 4 + len(b.notDecoded)
}

func (b *StsdBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	copy(buf[4:], b.notDecoded)
	_, err = w.Write(buf)
	return err
}

type SttsBox struct {
	Version         byte
	Flags           [3]byte
	SampleCount     []uint32
	SampleTimeDelta []uint32
}

func DecodeStts(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &SttsBox{
		Version:         data[0],
		Flags:           [3]byte{data[1], data[2], data[3]},
		SampleCount:     []uint32{},
		SampleTimeDelta: []uint32{},
	}
	ec := binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(ec); i++ {
		s_count := binary.BigEndian.Uint32(data[(8 + 8*i):(12 + 8*i)])
		s_delta := binary.BigEndian.Uint32(data[(12 + 8*i):(16 + 8*i)])
		b.SampleCount = append(b.SampleCount, s_count)
		b.SampleTimeDelta = append(b.SampleTimeDelta, s_delta)
	}
	return b, nil
}

func (b *SttsBox) Type() string {
	return "stts"
}

func (b *SttsBox) Size() int {
	return BoxHeaderSize + 8 + len(b.SampleCount)*8
}

func (b *SttsBox) GetTimeCode(sample, timescale uint32) time.Duration {
	sample--
	var units uint32
	i := 0
	for sample > 0 {
		if sample >= b.SampleCount[i] {
			units += b.SampleCount[i] * b.SampleTimeDelta[i]
			sample -= b.SampleCount[i]
		} else {
			units += sample * b.SampleTimeDelta[i]
			sample = 0
		}
		i++
	}
	return time.Second * time.Duration(units) / time.Duration(timescale)
}

func (b *SttsBox) Dump() {
	fmt.Println("Time to sample:")
	for i := range b.SampleCount {
		fmt.Printf(" #%d : %d samples with duration %d units\n", i, b.SampleCount[i], b.SampleTimeDelta[i])
	}
}

func (b *SttsBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], uint32(len(b.SampleCount)))
	for i := range b.SampleCount {
		binary.BigEndian.PutUint32(buf[8+8*i:], b.SampleCount[i])
		binary.BigEndian.PutUint32(buf[12+8*i:], b.SampleTimeDelta[i])
	}
	_, err = w.Write(buf)
	return err
}

type StssBox struct {
	Version      byte
	Flags        [3]byte
	SampleNumber []uint32
}

func DecodeStss(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &StssBox{
		Version:      data[0],
		Flags:        [3]byte{data[1], data[2], data[3]},
		SampleNumber: []uint32{},
	}
	ec := binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(ec); i++ {
		sample := binary.BigEndian.Uint32(data[(8 + 4*i):(12 + 4*i)])
		b.SampleNumber = append(b.SampleNumber, sample)
	}
	return b, nil
}

func (b *StssBox) Type() string {
	return "stss"
}

func (b *StssBox) Size() int {
	return BoxHeaderSize + 8 + len(b.SampleNumber)*4
}

func (b *StssBox) Dump() {
	fmt.Println("Key frames:")
	for i, n := range b.SampleNumber {
		fmt.Printf(" #%d : sample #%d\n", i, n)
	}
}

func (b *StssBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], uint32(len(b.SampleNumber)))
	for i := range b.SampleNumber {
		binary.BigEndian.PutUint32(buf[8+4*i:], b.SampleNumber[i])
	}
	_, err = w.Write(buf)
	return err
}

type StscBox struct {
	Version             byte
	Flags               [3]byte
	FirstChunk          []uint32
	SamplesPerChunk     []uint32
	SampleDescriptionID []uint32
}

func DecodeStsc(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	b := &StscBox{
		Version:             data[0],
		Flags:               [3]byte{data[1], data[2], data[3]},
		FirstChunk:          []uint32{},
		SamplesPerChunk:     []uint32{},
		SampleDescriptionID: []uint32{},
	}
	ec := binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(ec); i++ {
		fc := binary.BigEndian.Uint32(data[(8 + 12*i):(12 + 12*i)])
		spc := binary.BigEndian.Uint32(data[(12 + 12*i):(16 + 12*i)])
		sdi := binary.BigEndian.Uint32(data[(16 + 12*i):(20 + 12*i)])
		b.FirstChunk = append(b.FirstChunk, fc)
		b.SamplesPerChunk = append(b.SamplesPerChunk, spc)
		b.SampleDescriptionID = append(b.SampleDescriptionID, sdi)
	}
	return b, nil
}

func (b *StscBox) Type() string {
	return "stsc"
}

func (b *StscBox) Size() int {
	return BoxHeaderSize + 8 + len(b.FirstChunk)*12
}

func (b *StscBox) Dump() {
	fmt.Println("Sample to Chunk:")
	for i := range b.SamplesPerChunk {
		fmt.Printf(" #%d : %d samples per chunk starting @chunk #%d \n", i, b.SamplesPerChunk[i], b.FirstChunk[i])
	}
}

func (b *StscBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], uint32(len(b.FirstChunk)))
	for i := range b.FirstChunk {
		binary.BigEndian.PutUint32(buf[8+12*i:], b.FirstChunk[i])
		binary.BigEndian.PutUint32(buf[12+12*i:], b.SamplesPerChunk[i])
		binary.BigEndian.PutUint32(buf[16+12*i:], b.SampleDescriptionID[i])
	}
	_, err = w.Write(buf)
	return err
}

type StszBox struct {
	Version           byte
	Flags             [3]byte
	SampleUniformSize uint32
	SampleNumber      uint32
	SampleSize        []uint32
}

func DecodeStsz(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &StszBox{
		Version:           data[0],
		Flags:             [3]byte{data[1], data[2], data[3]},
		SampleUniformSize: binary.BigEndian.Uint32(data[4:8]),
		SampleNumber:      binary.BigEndian.Uint32(data[8:12]),
		SampleSize:        []uint32{},
	}
	if len(data) > 12 {
		for i := 0; i < int(b.SampleNumber); i++ {
			sz := binary.BigEndian.Uint32(data[(12 + 4*i):(16 + 4*i)])
			b.SampleSize = append(b.SampleSize, sz)
		}
	}
	return b, nil
}

func (b *StszBox) Type() string {
	return "stsz"
}

func (b *StszBox) Size() int {
	return BoxHeaderSize + 12 + len(b.SampleSize)*4
}

func (b *StszBox) Dump() {
	if len(b.SampleSize) == 0 {
		fmt.Printf("Samples : %d total samples\n", b.SampleNumber)
	} else {
		fmt.Printf("Samples : %d total samples\n", len(b.SampleSize))
	}
}

func (b *StszBox) GetSampleSize(i int) uint32 {
	if i > len(b.SampleSize) {
		return b.SampleUniformSize
	}
	return b.SampleSize[i-1]
}

func (b *StszBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.SampleUniformSize)
	if len(b.SampleSize) == 0 {
		binary.BigEndian.PutUint32(buf[8:], b.SampleNumber)
	} else {
		binary.BigEndian.PutUint32(buf[8:], uint32(len(b.SampleSize)))
		for i := range b.SampleSize {
			binary.BigEndian.PutUint32(buf[12+4*i:], b.SampleSize[i])
		}
	}
	_, err = w.Write(buf)
	return err
}

type StcoBox struct {
	Version     byte
	Flags       [3]byte
	ChunkOffset []uint32
}

func DecodeStco(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &StcoBox{
		Version:     data[0],
		Flags:       [3]byte{data[1], data[2], data[3]},
		ChunkOffset: []uint32{},
	}
	ec := binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(ec); i++ {
		chunk := binary.BigEndian.Uint32(data[(8 + 4*i):(12 + 4*i)])
		b.ChunkOffset = append(b.ChunkOffset, chunk)
	}
	return b, nil
}

func (b *StcoBox) Type() string {
	return "stco"
}

func (b *StcoBox) Size() int {
	return BoxHeaderSize + 8 + len(b.ChunkOffset)*4
}

func (b *StcoBox) Dump() {
	fmt.Println("Chunk byte offsets:")
	for i, o := range b.ChunkOffset {
		fmt.Printf(" #%d : starts at %d\n", i, o)
	}
}

func (b *StcoBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], uint32(len(b.ChunkOffset)))
	for i := range b.ChunkOffset {
		binary.BigEndian.PutUint32(buf[8+4*i:], b.ChunkOffset[i])
	}
	_, err = w.Write(buf)
	return err
}

type CttsBox struct {
	Version      byte
	Flags        [3]byte
	SampleCount  []uint32
	SampleOffset []uint32
}

func DecodeCtts(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &CttsBox{
		Version:      data[0],
		Flags:        [3]byte{data[1], data[2], data[3]},
		SampleCount:  []uint32{},
		SampleOffset: []uint32{},
	}
	ec := binary.BigEndian.Uint32(data[4:8])
	for i := 0; i < int(ec); i++ {
		s_count := binary.BigEndian.Uint32(data[(8 + 8*i):(12 + 8*i)])
		s_offset := binary.BigEndian.Uint32(data[(12 + 8*i):(16 + 8*i)])
		b.SampleCount = append(b.SampleCount, s_count)
		b.SampleOffset = append(b.SampleOffset, s_offset)
	}
	return b, nil
}

func (b *CttsBox) Type() string {
	return "ctts"
}

func (b *CttsBox) Size() int {
	return BoxHeaderSize + 8 + len(b.SampleCount)*8
}

func (b *CttsBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], uint32(len(b.SampleCount)))
	for i := range b.SampleCount {
		binary.BigEndian.PutUint32(buf[8+8*i:], b.SampleCount[i])
		binary.BigEndian.PutUint32(buf[12+8*i:], b.SampleOffset[i])
	}
	_, err = w.Write(buf)
	return err
}

type DinfBox struct {
	Dref *DrefBox
}

func DecodeDinf(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	d := &DinfBox{}
	for _, b := range l {
		switch b.Type() {
		case "dref":
			d.Dref = b.(*DrefBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return d, nil
}

func (b *DinfBox) Type() string {
	return "dinf"
}

func (b *DinfBox) Size() int {
	return BoxHeaderSize + b.Dref.Size()
}

func (b *DinfBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	return b.Dref.Encode(w)
}

type DrefBox struct {
	Version    byte
	Flags      [3]byte
	notDecoded []byte
}

func DecodeDref(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &DrefBox{
		Version:    data[0],
		Flags:      [3]byte{data[1], data[2], data[3]},
		notDecoded: data[4:],
	}, nil
}

func (b *DrefBox) Type() string {
	return "dref"
}

func (b *DrefBox) Size() int {
	return BoxHeaderSize + 4 + len(b.notDecoded)
}

func (b *DrefBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	copy(buf[4:], b.notDecoded)
	_, err = w.Write(buf)
	return err
}

type UdtaBox struct {
	Meta *MetaBox
}

func DecodeUdta(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	u := &UdtaBox{}
	for _, b := range l {
		switch b.Type() {
		case "meta":
			u.Meta = b.(*MetaBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return u, nil
}

func (b *UdtaBox) Type() string {
	return "udta"
}

func (b *UdtaBox) Size() int {
	return BoxHeaderSize + b.Meta.Size()
}

func (b *UdtaBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	return b.Meta.Encode(w)
}

type MetaBox struct {
	Version    byte
	Flags      [3]byte
	notDecoded []byte
}

func DecodeMeta(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &MetaBox{
		Version:    data[0],
		Flags:      [3]byte{data[1], data[2], data[3]},
		notDecoded: data[4:],
	}, nil
}

func (b *MetaBox) Type() string {
	return "meta"
}

func (b *MetaBox) Size() int {
	return BoxHeaderSize + 4 + len(b.notDecoded)
}

func (b *MetaBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	buf := makebuf(b)
	buf[0] = b.Version
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	copy(buf[4:], b.notDecoded)
	_, err = w.Write(buf)
	return err
}

type MdatBox struct {
	ContentSize uint32
	r           io.Reader
}

func DecodeMdat(r io.Reader) (Box, error) {
	return &MdatBox{r: r}, nil
}

func (b *MdatBox) Type() string {
	return "mdat"
}

func (b *MdatBox) Size() int {
	return BoxHeaderSize + int(b.ContentSize)
}

func (b *MdatBox) Encode(w io.Writer) error {
	err := EncodeHeader(b, w)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, b.r)
	return err
}

// An 8.8 fixed point number
type Fixed16 uint16

func (f Fixed16) String() string {
	return fmt.Sprintf("%d.%d", uint16(f)>>8, uint16(f)&7)
}

func fixed16(bytes []byte) Fixed16 {
	return Fixed16(binary.BigEndian.Uint16(bytes))
}

func putFixed16(bytes []byte, i Fixed16) {
	binary.BigEndian.PutUint16(bytes, uint16(i))
}

// A 16.16 fixed point number
type Fixed32 uint32

func (f Fixed32) String() string {
	return fmt.Sprintf("%d.%d", uint32(f)>>16, uint32(f)&15)
}

func fixed32(bytes []byte) Fixed32 {
	return Fixed32(binary.BigEndian.Uint32(bytes))
}

func putFixed32(bytes []byte, i Fixed32) {
	binary.BigEndian.PutUint32(bytes, uint32(i))
}

func strtobuf(out []byte, str string, l int) {
	in := []byte(str)
	if l < len(in) {
		copy(out, in)
	} else {
		copy(out, in[0:l])
	}
}

func makebuf(b Box) []byte {
	return make([]byte, b.Size()-BoxHeaderSize)
}
