package mp4

import (
	"errors"
	"io"
	"log"
	"sort"
	"time"
)

var (
	ErrInvalidDuration = errors.New("invalid duration")
	ErrClipOutside     = errors.New("clip zone is outside video")
	ErrTruncatedChunk  = errors.New("chunk was truncted")
)

type chunk struct {
	track                   int
	index                   int
	firstTC, lastTC         time.Duration
	descriptionID           uint32
	oldOffset               uint32
	samples                 []uint32
	firstSample, lastSample uint32
	skip                    bool
}

func (c *chunk) size() uint32 {
	var sz uint32
	for _, ssz := range c.samples {
		sz += ssz
	}
	return sz
}

type mdat []*chunk

func (m mdat) Len() int {
	return len(m)
}

func (m mdat) Less(i, j int) bool {
	return m[i].oldOffset < m[j].oldOffset
}

func (m mdat) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m mdat) firstSample(tnum int, timecode time.Duration) uint32 {
	for _, c := range m {
		if c.track != tnum {
			continue
		}
		if timecode >= c.firstTC && timecode <= c.lastTC {
			return c.firstSample
		}
	}
	return 0
}

func (m mdat) lastSample(tnum int, timecode time.Duration) uint32 {
	for _, c := range m {
		if c.track != tnum {
			continue
		}
		if timecode >= c.firstTC && timecode <= c.lastTC {
			return c.lastSample
		}
	}
	return 0
}

type clipFilter struct {
	err        error
	begin, end time.Duration
	mdatSize   uint32
	chunks     mdat
}

// Clip a video between begin and begin + duration (in seconds, starting at 0)
func Clip(begin, duration int) *clipFilter {
	f := &clipFilter{begin: time.Duration(begin) * time.Second, end: time.Duration(begin+duration) * time.Second}
	if begin < 0 {
		f.err = ErrClipOutside
	}
	return f
}

func (f *clipFilter) FilterMoov(m *MoovBox) error {
	if f.err != nil {
		return f.err
	}
	if f.begin > time.Second*time.Duration(m.Mvhd.Duration)/time.Duration(m.Mvhd.Timescale) {
		return ErrClipOutside
	}
	if f.end > time.Second*time.Duration(m.Mvhd.Duration)/time.Duration(m.Mvhd.Timescale) || f.end == f.begin {
		f.end = time.Second * time.Duration(m.Mvhd.Duration) / time.Duration(m.Mvhd.Timescale)
	}
	oldSize := m.Size()
	f.chunks = []*chunk{}
	for tnum, t := range m.Trak {
		f.buildChunkList(tnum, t)
		// update stts, find first/last sample
		f.updateSamples(tnum, t)
		f.updateChunks(tnum, t)
		// co64 ?
	}
	f.updateDurations(m)
	sort.Sort(f.chunks)
	for _, c := range f.chunks {
		sz := 0
		for _, ssz := range c.samples {
			sz += int(ssz)
		}
		log.Printf("chunk %d/track %d (offset %d, size %d)\n", c.index, c.track, c.oldOffset, sz)
	}
	deltaOffset := m.Size() - oldSize
	f.mdatSize = f.updateChunkOffsets(m, deltaOffset)
	return nil
}

func (f *clipFilter) buildChunkList(tnum int, t *TrakBox) {
	stsz := t.Mdia.Minf.Stbl.Stsz
	stsc := t.Mdia.Minf.Stbl.Stsc
	stco := t.Mdia.Minf.Stbl.Stco
	stts := t.Mdia.Minf.Stbl.Stts
	timescale := t.Mdia.Mdhd.Timescale
	sci, ssi := 0, 0
	for i, off := range stco.ChunkOffset {
		c := &chunk{
			track:       tnum,
			index:       i + 1,
			oldOffset:   uint32(off),
			samples:     []uint32{},
			firstSample: uint32(ssi + 1),
			firstTC:     stts.GetTimeCode(uint32(ssi+1), timescale),
		}
		if sci < len(stsc.FirstChunk)-1 && c.index >= int(stsc.FirstChunk[sci+1]) {
			sci++
		}
		c.descriptionID = stsc.SampleDescriptionID[sci]
		samples := stsc.SamplesPerChunk[sci]
		for samples > 0 {
			c.samples = append(c.samples, stsz.GetSampleSize(ssi))
			ssi++
			samples--
		}
		c.lastSample = uint32(ssi + 1)
		c.lastTC = stts.GetTimeCode(c.lastSample, timescale)
		f.chunks = append(f.chunks, c)
	}
}

func (f *clipFilter) updateSamples(tnum int, t *TrakBox) {
	// stts - sample duration
	stts := t.Mdia.Minf.Stbl.Stts
	oldCount, oldDelta := stts.SampleCount, stts.SampleTimeDelta
	stts.SampleCount, stts.SampleTimeDelta = []uint32{}, []uint32{}

	firstSample := f.chunks.firstSample(tnum, f.begin)
	lastSample := f.chunks.lastSample(tnum, f.end)

	//FIXME : compute real duration
	var sample uint32
	for i := 0; i < len(oldCount) && sample < lastSample; i++ {
		if sample+oldCount[i] >= firstSample {
			current := oldCount[i]
			if sample < firstSample && sample+oldCount[i] > firstSample {
				current += sample - firstSample
			}
			if sample+oldCount[i] > lastSample {
				current += lastSample - sample - oldCount[i]
			}
			stts.SampleCount = append(stts.SampleCount, current)
			stts.SampleTimeDelta = append(stts.SampleTimeDelta, oldDelta[i])
		}
		sample += oldCount[i]
	}
	log.Printf("first : %d, last : %d (trak %d)", firstSample, lastSample, tnum)

	// stss (key frames)
	stss := t.Mdia.Minf.Stbl.Stss
	if stss != nil {
		oldNumber := stss.SampleNumber
		stss.SampleNumber = []uint32{}
		for _, n := range oldNumber {
			if n >= firstSample && n <= lastSample {
				stss.SampleNumber = append(stss.SampleNumber, n-uint32(firstSample))
			}
		}
	}

	// stsz (sample sizes)
	stsz := t.Mdia.Minf.Stbl.Stsz
	oldSize := stsz.SampleSize
	stsz.SampleSize = []uint32{}
	for n, sz := range oldSize {
		if uint32(n) >= firstSample-1 && uint32(n) <= lastSample-1 {
			stsz.SampleSize = append(stsz.SampleSize, sz)
		}
	}

	// ctts - time offsets
	ctts := t.Mdia.Minf.Stbl.Ctts
	if ctts != nil {
		oldCount, oldOffset := ctts.SampleCount, ctts.SampleOffset
		ctts.SampleCount, ctts.SampleOffset = []uint32{}, []uint32{}
		var sample uint32
		for i := 0; i < len(oldCount) && sample < lastSample; i++ {
			if sample+oldCount[i] >= firstSample {
				current := oldCount[i]
				if sample < firstSample && sample+oldCount[i] > firstSample {
					current += sample - firstSample
				}
				if sample+oldCount[i] > lastSample {
					current += lastSample - sample - oldCount[i]
				}

				ctts.SampleCount = append(ctts.SampleCount, current)
				ctts.SampleOffset = append(ctts.SampleOffset, oldOffset[i])
			}
			sample += oldCount[i]
		}
	}

}

func (f *clipFilter) updateChunks(tnum int, t *TrakBox) {
	// stsc (sample to chunk) - full rebuild
	stsc := t.Mdia.Minf.Stbl.Stsc
	stsc.FirstChunk, stsc.SamplesPerChunk, stsc.SampleDescriptionID = []uint32{}, []uint32{}, []uint32{}
	var firstChunk *chunk
	var index uint32
	for _, c := range f.chunks {
		if c.track != tnum {
			continue
		}
		if c.firstTC > f.end || c.lastTC < f.begin {
			c.skip = true
			continue
		}
		index++
		if firstChunk == nil {
			firstChunk = c
		}
		if index == 1 || len(c.samples) != len(firstChunk.samples) || c.descriptionID != firstChunk.descriptionID {
			stsc.FirstChunk = append(stsc.FirstChunk, index)
			stsc.SamplesPerChunk = append(stsc.SamplesPerChunk, uint32(len(firstChunk.samples)))
			stsc.SampleDescriptionID = append(stsc.SampleDescriptionID, firstChunk.descriptionID)
			firstChunk = c
		}
	}

	// stco (chunk offsets) - build empty table to compute moov box size
	stco := t.Mdia.Minf.Stbl.Stco
	stco.ChunkOffset = make([]uint32, index)
}

func (f *clipFilter) updateChunkOffsets(m *MoovBox, deltaOff int) uint32 {
	stco, i := make([]*StcoBox, len(m.Trak)), make([]int, len(m.Trak))
	for tnum, t := range m.Trak {
		stco[tnum] = t.Mdia.Minf.Stbl.Stco
	}
	var offset, sz uint32
	for _, c := range f.chunks {
		if offset == 0 {
			offset = uint32(int(c.oldOffset) + deltaOff)
		}
		if !c.skip {
			stco[c.track].ChunkOffset[i[c.track]] = offset + sz
			i[c.track]++
			sz += c.size()
		}
	}
	return sz
}

func (f *clipFilter) updateDurations(m *MoovBox) {
	timescale := m.Mvhd.Timescale
	m.Mvhd.Duration = 0
	for tnum, t := range m.Trak {
		var start, end time.Duration
		for _, c := range f.chunks {
			if c.track != tnum || c.skip {
				continue
			}
			if start == 0 || c.firstTC < start {
				start = c.firstTC
			}
			if end == 0 || c.lastTC > end {
				end = c.lastTC
			}
		}
		t.Mdia.Mdhd.Duration = uint32((end - start) * time.Duration(t.Mdia.Mdhd.Timescale) / time.Second)
		t.Tkhd.Duration = uint32((end - start) * time.Duration(timescale) / time.Second)
		if t.Tkhd.Duration > m.Mvhd.Duration {
			m.Mvhd.Duration = t.Tkhd.Duration
		}
	}
}

func (f *clipFilter) FilterMdat(w io.Writer, m *MdatBox) error {
	if f.err != nil {
		return f.err
	}
	m.ContentSize = f.mdatSize
	err := EncodeHeader(m, w)
	if err != nil {
		return err
	}
	var bufSize uint32
	for _, c := range f.chunks {
		if c.size() > bufSize {
			bufSize = c.size()
		}
	}
	buffer := make([]byte, bufSize)
	for _, c := range f.chunks {
		s := c.size()
		n, err := m.r.Read(buffer[:s])
		if err != nil {
			return err
		}
		if n != int(s) {
			return ErrTruncatedChunk
		}
		if !c.skip {
			n, err = w.Write(buffer[:s])
			if err != nil {
				return err
			}
			if n != int(s) {
				return ErrTruncatedChunk
			}
		} else {
			log.Printf("skipping chunk %d (track %d)\n", c.index, c.track)
		}
	}
	return nil
}
