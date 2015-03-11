package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
)

const (
	BOX_HEADER_SIZE = 8
)

var (
	ErrUnknownBoxType = errors.New("unknown box type")
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
	}
}

type BoxHeader struct {
	Type string
	Size int64
}

func (h BoxHeader) Encode(w io.Writer) error {
	buf := make([]byte, BOX_HEADER_SIZE)
	binary.BigEndian.PutUint32(buf, uint32(h.Size))
	strtobuf(buf[4:], h.Type, 4)
	_, err := w.Write(buf)
	return err
}

type Box interface {
	SetHeader(BoxHeader)
	Header() BoxHeader
}

type BoxDecoder func(r io.Reader) (Box, error)

func DecodeBox(h BoxHeader, r io.Reader) (Box, error) {
	fmt.Printf("Found %s with size %d\n", h.Type, h.Size)
	d := decoders[h.Type]
	if d == nil {
		log.Printf("Error : %s unknown", h.Type)
		return nil, ErrUnknownBoxType
	}
	b, err := d(io.LimitReader(r, h.Size-int64(BOX_HEADER_SIZE)))
	if err != nil {
		return nil, err
	}
	b.SetHeader(h)
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

type BaseBox struct {
	h BoxHeader
}

func (b *BaseBox) SetHeader(h BoxHeader) {
	b.h = h
}

func (b *BaseBox) Header() BoxHeader {
	return b.h
}

type FtypBox struct {
	BaseBox
	MajorBrand       string
	MinorVersion     string
	CompatibleBrands []string
}

func DecodeFtyp(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &FtypBox{
		MajorBrand:       string(data[0:4]),
		MinorVersion:     string(data[4:8]),
		CompatibleBrands: []string{},
	}
	if len(data) > 8 {
		for i := 8; i < len(data); i += 4 {
			b.CompatibleBrands = append(b.CompatibleBrands, string(data[i:i+4]))
		}
	}
	return b, nil
}

func (b *FtypBox) Dump() {
	fmt.Printf("ftyp:\n MajorBrand: %s\n MinorVersion: %s\n CompatibleBrands: %v\n", b.MajorBrand, b.MinorVersion, b.CompatibleBrands)
}

func (b *FtypBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+4*len(b.CompatibleBrands))
	strtobuf(buf, b.MajorBrand, 4)
	strtobuf(buf[4:], b.MinorVersion, 4)
	for i, c := range b.CompatibleBrands {
		strtobuf(buf[8+i*4:], c, 4)
	}
	_, err = w.Write(buf)
	return err
}

type MoovBox struct {
	BaseBox
	Mvhd *MvhdBox
	Iods *IodsBox
	Trak []*TrakBox
	Udta *UdtaBox
}

func DecodeMoov(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	m := &MoovBox{}
	for _, b := range l {
		switch b.Header().Type {
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

func (b *MoovBox) Dump() {
	fmt.Println("moov:")
	b.Mvhd.Dump()
}

func (b *MoovBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	err = b.Mvhd.Encode(w)
	if err != nil {
		return err
	}
	err = b.Iods.Encode(w)
	if err != nil {
		return err
	}
	for _, t := range b.Trak {
		err = t.Encode(w)
		if err != nil {
			return err
		}
	}
	return b.Udta.Encode(w)
}

type MvhdBox struct {
	BaseBox
	Version          uint8
	Flags            [3]byte
	CreationTime     uint32
	ModificationTime uint32
	Timescale        uint32
	Duration         uint32
	NextTrackId      uint32
	Rate             Fixed32
	Volume           Fixed16
	otherData        []byte
}

func DecodeMvhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &MvhdBox{
		Version:          data[0],
		Flags:            [3]byte{data[1], data[2], data[3]},
		CreationTime:     binary.BigEndian.Uint32(data[4:8]),
		ModificationTime: binary.BigEndian.Uint32(data[8:12]),
		Timescale:        binary.BigEndian.Uint32(data[12:16]),
		Duration:         binary.BigEndian.Uint32(data[16:20]),
		otherData:        data[26:],
	}
	b.Rate, err = MakeFixed32(data[20:24])
	if err != nil {
		return nil, err
	}
	b.Volume, err = MakeFixed16(data[24:26])
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (b *MvhdBox) Dump() {
	fmt.Printf("mvhd:\n Timescale: %d\n Duration: %d\n Rate: %s\n Volume: %s\n", b.Timescale, b.Duration, b.Rate, b.Volume)
}

func (b *MvhdBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 26)
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.CreationTime)
	binary.BigEndian.PutUint32(buf[8:], b.ModificationTime)
	binary.BigEndian.PutUint32(buf[12:], b.Timescale)
	binary.BigEndian.PutUint32(buf[16:], b.Duration)
	binary.BigEndian.PutUint32(buf[20:], uint32(b.Rate))
	binary.BigEndian.PutUint16(buf[24:], uint16(b.Volume))
	buf = append(buf, b.otherData...)
	_, err = w.Write(buf)
	return err
}

type IodsBox struct {
	BaseBox
	data []byte
}

func DecodeIods(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &IodsBox{
		data: data,
	}
	return b, nil
}

func (b *IodsBox) Dump() {
	fmt.Printf("iods:\n")
}

func (b *IodsBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	_, err = w.Write(b.data)
	return err
}

type TrakBox struct {
	BaseBox
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
		switch b.Header().Type {
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

func (b *TrakBox) Dump() {
	fmt.Println("trak:")
}

func (b *TrakBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
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
	BaseBox
	Version          uint8
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
	b := &TkhdBox{
		Version:          data[0],
		Flags:            [3]byte{data[1], data[2], data[3]},
		CreationTime:     binary.BigEndian.Uint32(data[4:8]),
		ModificationTime: binary.BigEndian.Uint32(data[8:12]),
		TrackId:          binary.BigEndian.Uint32(data[12:16]),
		Duration:         binary.BigEndian.Uint32(data[20:24]),
		Layer:            binary.BigEndian.Uint16(data[32:34]),
		AlternateGroup:   binary.BigEndian.Uint16(data[34:36]),
		Matrix:           data[40:76],
	}
	b.Volume, err = MakeFixed16(data[36:38])
	if err != nil {
		return nil, err
	}
	b.Width, err = MakeFixed32(data[76:80])
	if err != nil {
		return nil, err
	}
	b.Height, err = MakeFixed32(data[80:84])
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (b *TkhdBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 84)
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.CreationTime)
	binary.BigEndian.PutUint32(buf[8:], b.ModificationTime)
	binary.BigEndian.PutUint32(buf[12:], b.TrackId)
	binary.BigEndian.PutUint32(buf[20:], b.Duration)
	binary.BigEndian.PutUint16(buf[32:], b.Layer)
	binary.BigEndian.PutUint16(buf[34:], b.AlternateGroup)
	PutFixed16(buf[36:], b.Volume)
	copy(buf[40:], b.Matrix)
	PutFixed32(buf[76:], b.Width)
	PutFixed32(buf[80:], b.Height)
	_, err = w.Write(buf)
	return err
}

type EdtsBox struct {
	BaseBox
	Elst *ElstBox
}

func DecodeEdts(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	t := &EdtsBox{}
	for _, b := range l {
		switch b.Header().Type {
		case "elst":
			t.Elst = b.(*ElstBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return t, nil
}

func (b *EdtsBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	return b.Elst.Encode(w)
}

type ElstBox struct {
	BaseBox
	Version                             uint8
	Flags                               [3]byte
	EntryCount                          uint32
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
		EntryCount:        binary.BigEndian.Uint32(data[4:8]),
		SegmentDuration:   []uint32{},
		MediaTime:         []uint32{},
		MediaRateInteger:  []uint16{},
		MediaRateFraction: []uint16{},
	}
	for i := 0; i < int(b.EntryCount); i++ {
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

func (b *ElstBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+int(b.EntryCount*12))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	for i := 0; i < int(b.EntryCount); i++ {
		binary.BigEndian.PutUint32(buf[8+12*i:], b.SegmentDuration[i])
		binary.BigEndian.PutUint32(buf[12+12*i:], b.MediaTime[i])
		binary.BigEndian.PutUint16(buf[16+12*i:], b.MediaRateInteger[i])
		binary.BigEndian.PutUint16(buf[18+12*i:], b.MediaRateFraction[i])
	}
	_, err = w.Write(buf)
	return err
}

type MdiaBox struct {
	BaseBox
	Mdhd *MdhdBox
	Hdlr *HdlrBox
	Minf *MinfBox
}

func DecodeMdia(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	t := &MdiaBox{}
	for _, b := range l {
		switch b.Header().Type {
		case "mdhd":
			t.Mdhd = b.(*MdhdBox)
		case "hdlr":
			t.Hdlr = b.(*HdlrBox)
		case "minf":
			t.Minf = b.(*MinfBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return t, nil
}

func (b *MdiaBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
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
	BaseBox
	Version          uint8
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
	b := &MdhdBox{
		Version:          data[0],
		Flags:            [3]byte{data[1], data[2], data[3]},
		CreationTime:     binary.BigEndian.Uint32(data[4:8]),
		ModificationTime: binary.BigEndian.Uint32(data[8:12]),
		Timescale:        binary.BigEndian.Uint32(data[12:16]),
		Duration:         binary.BigEndian.Uint32(data[16:20]),
		Language:         binary.BigEndian.Uint16(data[20:22]),
	}
	return b, nil
}

func (b *MdhdBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 24)
	buf[0] = byte(b.Version)
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
	BaseBox
	Version     uint8
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
	b := &HdlrBox{
		Version:     data[0],
		Flags:       [3]byte{data[1], data[2], data[3]},
		PreDefined:  binary.BigEndian.Uint32(data[4:8]),
		HandlerType: string(data[8:12]),
		TrackName:   string(data[24:]),
	}
	return b, nil
}

func (b *HdlrBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 24+len(b.TrackName))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.PreDefined)
	strtobuf(buf[8:], b.HandlerType, 4)
	strtobuf(buf[24:], b.TrackName, len(b.TrackName))
	_, err = w.Write(buf)
	return err
}

type MinfBox struct {
	BaseBox
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
		switch b.Header().Type {
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

func (b *MinfBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
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
	BaseBox
	Version      uint8
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

func (b *VmhdBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 12)
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint16(buf[4:], b.GraphicsMode)
	for i := 0; i < 3; i++ {
		binary.BigEndian.PutUint16(buf[6+2*i:], b.OpColor[i])
	}
	_, err = w.Write(buf)
	return err
}

type SmhdBox struct {
	BaseBox
	Version uint8
	Flags   [3]byte
	Balance uint16 // This should really be int16 but not sure how to parse
}

func DecodeSmhd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &SmhdBox{
		Version: data[0],
		Flags:   [3]byte{data[1], data[2], data[3]},
		Balance: binary.BigEndian.Uint16(data[4:6]),
	}
	return b, nil
}

func (b *SmhdBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8)
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint16(buf[4:], b.Balance)
	_, err = w.Write(buf)
	return err
}

type StblBox struct {
	BaseBox
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
		switch b.Header().Type {
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

func (b *StblBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
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
	BaseBox
	Version    uint8
	Flags      [3]byte
	EntryCount uint32
	otherData  []byte
}

func DecodeStsd(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &StsdBox{
		Version:    data[0],
		Flags:      [3]byte{data[1], data[2], data[3]},
		EntryCount: binary.BigEndian.Uint32(data[4:8]),
		otherData:  data[8:],
	}
	return b, nil
}

func (b *StsdBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+len(b.otherData))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	copy(buf[8:], b.otherData)
	_, err = w.Write(buf)
	return err
}

type SttsBox struct {
	BaseBox
	Version     uint8
	Flags       [3]byte
	EntryCount  uint32
	SampleCount []uint32
	SampleDelta []uint32
}

func DecodeStts(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &SttsBox{
		Version:     data[0],
		Flags:       [3]byte{data[1], data[2], data[3]},
		EntryCount:  binary.BigEndian.Uint32(data[4:8]),
		SampleCount: []uint32{},
		SampleDelta: []uint32{},
	}
	for i := 0; i < int(b.EntryCount); i++ {
		s_count := binary.BigEndian.Uint32(data[(8 + 8*i):(12 + 8*i)])
		s_delta := binary.BigEndian.Uint32(data[(12 + 8*i):(16 + 8*i)])
		b.SampleCount = append(b.SampleCount, s_count)
		b.SampleDelta = append(b.SampleDelta, s_delta)
	}
	return b, nil
}

func (b *SttsBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+int(b.EntryCount*8))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	for i := 0; i < int(b.EntryCount); i++ {
		binary.BigEndian.PutUint32(buf[8+8*i:], b.SampleCount[i])
		binary.BigEndian.PutUint32(buf[12+8*i:], b.SampleDelta[i])
	}
	_, err = w.Write(buf)
	return err
}

type StssBox struct {
	BaseBox
	Version      uint8
	Flags        [3]byte
	EntryCount   uint32
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
		EntryCount:   binary.BigEndian.Uint32(data[4:8]),
		SampleNumber: []uint32{},
	}
	for i := 0; i < int(b.EntryCount); i++ {
		sample := binary.BigEndian.Uint32(data[(8 + 4*i):(12 + 4*i)])
		b.SampleNumber = append(b.SampleNumber, sample)
	}
	return b, nil
}

func (b *StssBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+int(b.EntryCount*4))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	for i := 0; i < int(b.EntryCount); i++ {
		binary.BigEndian.PutUint32(buf[8+4*i:], b.SampleNumber[i])
	}
	_, err = w.Write(buf)
	return err
}

type StscBox struct {
	BaseBox
	Version                uint8
	Flags                  [3]byte
	EntryCount             uint32
	FirstChunk             []uint32
	SamplesPerChunk        []uint32
	SampleDescriptionIndex []uint32
}

func DecodeStsc(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	b := &StscBox{
		Version:                data[0],
		Flags:                  [3]byte{data[1], data[2], data[3]},
		EntryCount:             binary.BigEndian.Uint32(data[4:8]),
		FirstChunk:             []uint32{},
		SamplesPerChunk:        []uint32{},
		SampleDescriptionIndex: []uint32{},
	}
	for i := 0; i < int(b.EntryCount); i++ {
		fc := binary.BigEndian.Uint32(data[(8 + 12*i):(12 + 12*i)])
		spc := binary.BigEndian.Uint32(data[(12 + 12*i):(16 + 12*i)])
		sdi := binary.BigEndian.Uint32(data[(16 + 12*i):(20 + 12*i)])
		b.FirstChunk = append(b.FirstChunk, fc)
		b.SamplesPerChunk = append(b.SamplesPerChunk, spc)
		b.SampleDescriptionIndex = append(b.SampleDescriptionIndex, sdi)
	}
	return b, nil
}

func (b *StscBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+int(b.EntryCount*12))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	for i := 0; i < int(b.EntryCount); i++ {
		binary.BigEndian.PutUint32(buf[8+12*i:], b.FirstChunk[i])
		binary.BigEndian.PutUint32(buf[12+12*i:], b.SamplesPerChunk[i])
		binary.BigEndian.PutUint32(buf[16+12*i:], b.SampleDescriptionIndex[i])
	}
	_, err = w.Write(buf)
	return err
}

type StszBox struct {
	BaseBox
	Version     uint8
	Flags       [3]byte
	SampleSize  uint32
	SampleCount uint32
	EntrySize   []uint32
}

func DecodeStsz(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &StszBox{
		Version:     data[0],
		Flags:       [3]byte{data[1], data[2], data[3]},
		SampleSize:  binary.BigEndian.Uint32(data[4:8]),
		SampleCount: binary.BigEndian.Uint32(data[8:12]),
		EntrySize:   []uint32{},
	}
	for i := 0; i < int(b.SampleCount); i++ {
		entry := binary.BigEndian.Uint32(data[(12 + 4*i):(16 + 4*i)])
		b.EntrySize = append(b.EntrySize, entry)
	}
	return b, nil
}

func (b *StszBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 12+int(b.SampleCount*4))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.SampleSize)
	binary.BigEndian.PutUint32(buf[8:], b.SampleCount)
	for i := 0; i < int(b.SampleCount); i++ {
		binary.BigEndian.PutUint32(buf[12+4*i:], b.EntrySize[i])
	}
	_, err = w.Write(buf)
	return err
}

type StcoBox struct {
	BaseBox
	Version     uint8
	Flags       [3]byte
	EntryCount  uint32
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
		EntryCount:  binary.BigEndian.Uint32(data[4:8]),
		ChunkOffset: []uint32{},
	}
	for i := 0; i < int(b.EntryCount); i++ {
		chunk := binary.BigEndian.Uint32(data[(8 + 4*i):(12 + 4*i)])
		b.ChunkOffset = append(b.ChunkOffset, chunk)
	}
	return b, nil
}

func (b *StcoBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+int(b.EntryCount*4))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	for i := 0; i < int(b.EntryCount); i++ {
		binary.BigEndian.PutUint32(buf[8+4*i:], b.ChunkOffset[i])
	}
	_, err = w.Write(buf)
	return err
}

type CttsBox struct {
	BaseBox
	Version      uint8
	Flags        [3]byte
	EntryCount   uint32
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
		EntryCount:   binary.BigEndian.Uint32(data[4:8]),
		SampleCount:  []uint32{},
		SampleOffset: []uint32{},
	}
	for i := 0; i < int(b.EntryCount); i++ {
		s_count := binary.BigEndian.Uint32(data[(8 + 8*i):(12 + 8*i)])
		s_offset := binary.BigEndian.Uint32(data[(12 + 8*i):(16 + 8*i)])
		b.SampleCount = append(b.SampleCount, s_count)
		b.SampleOffset = append(b.SampleOffset, s_offset)
	}
	return b, nil
}

func (b *CttsBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+int(b.EntryCount*8))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	for i := 0; i < int(b.EntryCount); i++ {
		binary.BigEndian.PutUint32(buf[8+8*i:], b.SampleCount[i])
		binary.BigEndian.PutUint32(buf[12+8*i:], b.SampleOffset[i])
	}
	_, err = w.Write(buf)
	return err
}

type DinfBox struct {
	BaseBox
	Dref *DrefBox
}

func DecodeDinf(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	d := &DinfBox{}
	for _, b := range l {
		switch b.Header().Type {
		case "dref":
			d.Dref = b.(*DrefBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return d, nil
}

func (b *DinfBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	return b.Dref.Encode(w)
}

type DrefBox struct {
	BaseBox
	Version    uint8
	Flags      [3]byte
	EntryCount uint32
	otherData  []byte
}

func DecodeDref(r io.Reader) (Box, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b := &DrefBox{
		Version:    data[0],
		Flags:      [3]byte{data[1], data[2], data[3]},
		EntryCount: binary.BigEndian.Uint32(data[4:8]),
		otherData:  data[8:],
	}
	return b, nil
}

func (b *DrefBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+len(b.otherData))
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	binary.BigEndian.PutUint32(buf[4:], b.EntryCount)
	copy(buf[8:], b.otherData)
	_, err = w.Write(buf)
	return err
}

type UdtaBox struct {
	BaseBox
	Meta *MetaBox
}

func DecodeUdta(r io.Reader) (Box, error) {
	l, err := DecodeContainer(r)
	if err != nil {
		return nil, err
	}
	u := &UdtaBox{}
	for _, b := range l {
		switch b.Header().Type {
		case "meta":
			u.Meta = b.(*MetaBox)
		default:
			return nil, ErrBadFormat
		}
	}
	return u, nil
}

func (b *UdtaBox) Dump() {
	fmt.Printf("udta:\n")
}

func (b *UdtaBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	return err
}

type MetaBox struct {
	BaseBox
	Version uint8
	Flags   [3]byte
	Hdlr    *HdlrBox
}

func DecodeMeta(r io.Reader) (Box, error) {
	ioutil.ReadAll(r)
	return &MetaBox{}, nil
}

/*
func (b *MetaBox) parse() (err error) {
	data := b.ReadBoxData()
	b.Version = data[0]
	b.Flags = [3]byte{data[1], data[2], data[3]}
	boxes := readSubBoxes(b.File(), b.Start()+4, b.Size()-4)
	for subBox := range boxes {
		switch subBox.Name() {
		case "hdlr":
			b.hdlr = &HdlrBox{Box: subBox}
			err = b.hdlr.parse()
		default:
			fmt.Printf("Unhandled Meta Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}
*/
func (b *MetaBox) Encode(w io.Writer) error {
	err := b.Header().Encode(w)
	if err != nil {
		return err
	}
	buf := make([]byte, 4)
	buf[0] = byte(b.Version)
	buf[1], buf[2], buf[3] = b.Flags[0], b.Flags[1], b.Flags[2]
	_, err = w.Write(buf)
	b.Hdlr.Encode(w)
	return err
}

// An 8.8 Fixed Point Decimal notation
type Fixed16 uint16

func (f Fixed16) String() string {
	return fmt.Sprintf("%v", uint16(f)>>8)
}

func MakeFixed16(bytes []byte) (Fixed16, error) {
	if len(bytes) != 2 {
		return Fixed16(0), errors.New("Invalid number of bytes for Fixed16. Need 2, got " + string(len(bytes)))
	}
	return Fixed16(binary.BigEndian.Uint16(bytes)), nil
}

func PutFixed16(bytes []byte, i Fixed16) {
	binary.BigEndian.PutUint16(bytes, uint16(i))
}

// A 16.16 Fixed Point Decimal notation
type Fixed32 uint32

func (f Fixed32) String() string {
	return fmt.Sprintf("%v", uint32(f)>>16)
}

func MakeFixed32(bytes []byte) (Fixed32, error) {
	if len(bytes) != 4 {
		return Fixed32(0), errors.New("Invalid number of bytes for Fixed32. Need 4, got " + string(len(bytes)))
	}
	return Fixed32(binary.BigEndian.Uint32(bytes)), nil
}

func PutFixed32(bytes []byte, i Fixed32) {
	binary.BigEndian.PutUint32(bytes, uint32(i))
}

/*
type Chunk struct {
	SampleDescriptionIndex, start_sample, SampleCount, offset uint32
}

type Sample struct {
	size, offset, start_time, Duration, cto uint32
}
*/
func strtobuf(out []byte, str string, l int) {
	in := []byte(str)
	if l < len(in) {
		copy(out, in)
	} else {
		copy(out, in[0:l])
	}
}
