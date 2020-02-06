package mp4

import (
	"bufio"
	"fmt"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/codec/aacparser"
	"github.com/nareix/joy4/codec/h264parser"
	"github.com/nareix/joy4/format/mp4/mp4io"
	"github.com/nareix/joy4/utils/bits/pio"
	"io"
	"math"
	"time"
)

type Muxer struct {
	w       io.WriteSeeker
	bufw    *bufio.Writer
	wpos    int64
	streams []*Stream
}

func NewMuxer(w io.WriteSeeker) *Muxer {
	return &Muxer{
		w:    w,
		bufw: bufio.NewWriterSize(w, pio.RecommendBufioSize),
	}
}

func (self *Muxer) newStream(codec av.CodecData) (err error) {
	switch codec.Type() {
	case av.H264, av.AAC:

	default:
		err = fmt.Errorf("mp4: codec type=%v is not supported", codec.Type())
		return
	}
	stream := &Stream{CodecData: codec}

	stream.sample = &mp4io.SampleTable{
		SampleDesc:   &mp4io.SampleDesc{},
		TimeToSample: &mp4io.TimeToSample{},
		SampleToChunk: &mp4io.SampleToChunk{
			Entries: []mp4io.SampleToChunkEntry{
				{
					FirstChunk:      1,
					SampleDescId:    1,
					SamplesPerChunk: 1,
				},
			},
		},
		SampleSize:  &mp4io.SampleSize{},
		ChunkOffset: &mp4io.ChunkOffset{},
	}

	stream.trackAtom = &mp4io.Track{
		Header: &mp4io.TrackHeader{
			TrackId:  int32(len(self.streams) + 1),
			Flags:    0x0003, // Track enabled | Track in movie
			Duration: 0,      // fill later
			Matrix:   [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		},
		Media: &mp4io.Media{
			Header: &mp4io.MediaHeader{
				TimeScale: 0, // fill later
				Duration:  0, // fill later
				Language:  21956,
			},
			Info: &mp4io.MediaInfo{
				Sample: stream.sample,
				Data: &mp4io.DataInfo{
					Refer: &mp4io.DataRefer{
						Url: &mp4io.DataReferUrl{
							Flags: 0x000001, // Self reference
						},
					},
				},
			},
		},
	}

	switch codec.Type() {
	case av.H264:
		stream.sample.SyncSample = &mp4io.SyncSample{}
	}

	stream.timeScale = 90000
	stream.muxer = self
	self.streams = append(self.streams, stream)

	return
}

func (self *Stream) fillTrackAtom() (err error) {
	self.trackAtom.Media.Header.TimeScale = int32(self.timeScale)
	self.trackAtom.Media.Header.Duration = int32(self.duration)

	if self.Type() == av.H264 {
		codec := self.CodecData.(h264parser.CodecData)
		width, height := codec.Width(), codec.Height()
		self.sample.SampleDesc.AVC1Desc = &mp4io.AVC1Desc{
			DataRefIdx:           1,
			HorizontalResolution: 72,
			VorizontalResolution: 72,
			Width:                int16(width),
			Height:               int16(height),
			FrameCount:           1,
			Depth:                24,
			ColorTableId:         -1,
			Conf:                 &mp4io.AVC1Conf{Data: codec.AVCDecoderConfRecordBytes()},
		}
		self.trackAtom.Media.Handler = &mp4io.HandlerRefer{
			SubType: [4]byte{'v', 'i', 'd', 'e'},
			Name:    []byte("Video Media Handler"),
		}
		self.trackAtom.Media.Info.Video = &mp4io.VideoMediaInfo{
			Flags: 0x000001,
		}
		self.trackAtom.Header.TrackWidth = float64(width)
		self.trackAtom.Header.TrackHeight = float64(height)

	} else if self.Type() == av.AAC {
		codec := self.CodecData.(aacparser.CodecData)
		self.sample.SampleDesc.MP4ADesc = &mp4io.MP4ADesc{
			DataRefIdx:       1,
			NumberOfChannels: int16(codec.ChannelLayout().Count()),
			SampleSize:       int16(codec.SampleFormat().BytesPerSample()),
			SampleRate:       float64(codec.SampleRate()),
			Conf: &mp4io.ElemStreamDesc{
				DecConfig: codec.MPEG4AudioConfigBytes(),
			},
		}
		self.trackAtom.Header.Volume = 1
		self.trackAtom.Header.AlternateGroup = 1
		self.trackAtom.Media.Handler = &mp4io.HandlerRefer{
			SubType: [4]byte{'s', 'o', 'u', 'n'},
			Name:    []byte("Sound Handler"),
		}
		self.trackAtom.Media.Info.Sound = &mp4io.SoundMediaInfo{}

	} else {
		err = fmt.Errorf("mp4: codec type=%d invalid", self.Type())
	}

	return
}

func (self *Muxer) WriteHeader(streams []av.CodecData) (err error) {
	self.streams = []*Stream{}
	for _, stream := range streams {
		if err = self.newStream(stream); err != nil {
			return
		}
	}

	taghdr := make([]byte, 16)
	pio.PutU32BE(taghdr, 1) // wide atom
	pio.PutU32BE(taghdr[4:], uint32(mp4io.MDAT))
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}
	self.wpos += 16

	for _, stream := range self.streams {
		if stream.Type().IsVideo() {
			stream.sample.CompositionOffset = &mp4io.CompositionOffset{}
		}
	}
	return
}

func (self *Muxer) WritePacket(pkt av.Packet) (err error) {
	stream := self.streams[pkt.Idx]
	if stream.lastpkt != nil {
		writePkt := *stream.lastpkt
		dts, pts := stream.calculateTs(writePkt)
		if err = stream.writePacket(writePkt, dts, pts); err != nil {
			return
		}
	}
	stream.lastpkt = &pkt
	return
}

func (self *Stream) writePacket(pkt av.Packet, dts int64, pts int64) (err error) {
	if _, err = self.muxer.bufw.Write(pkt.Data); err != nil {
		return
	}

	if pkt.IsKeyFrame && self.sample.SyncSample != nil {
		self.sample.SyncSample.Entries = append(self.sample.SyncSample.Entries, uint32(self.sampleIndex+1))
	}

	duration := uint32(dts - self.lastMuxDTS)
	if self.hasLastMuxDTS && dts == 0 && pts == 0 {
		duration = 0 // for trailer
	}
	if self.sttsEntry == nil || duration != self.sttsEntry.Duration {
		self.sample.TimeToSample.Entries = append(self.sample.TimeToSample.Entries, mp4io.TimeToSampleEntry{Duration: duration})
		self.sttsEntry = &self.sample.TimeToSample.Entries[len(self.sample.TimeToSample.Entries)-1]
	}
	self.sttsEntry.Count++

	if self.sample.CompositionOffset != nil {
		offset := uint32(pts - dts)
		if self.cttsEntry == nil || offset != self.cttsEntry.Offset {
			table := self.sample.CompositionOffset
			table.Entries = append(table.Entries, mp4io.CompositionOffsetEntry{Offset: offset})
			self.cttsEntry = &table.Entries[len(table.Entries)-1]
		}
		self.cttsEntry.Count++
	}

	self.duration += int64(duration)
	self.sampleIndex++
	self.sample.ChunkOffset.Entries = append(self.sample.ChunkOffset.Entries, self.muxer.wpos)
	self.sample.SampleSize.Entries = append(self.sample.SampleSize.Entries, uint32(len(pkt.Data)))

	self.muxer.wpos += int64(len(pkt.Data))

	self.lastMuxDTS = dts
	self.hasLastMuxDTS = true
	return
}

func (self *Stream) calculateTs(pkt av.Packet) (dts int64, pts int64) {
	dts = self.timeToTs(pkt.Time)
	pts = dts + self.timeToTs(pkt.CompositionTime)
	minDTS := self.lastMuxDTS + 1
	if dts > pts {
		min := pts
		if dts < min {
			min = dts
		}
		if minDTS < min {
			min = minDTS
		}
		max := pts
		if dts > max {
			max = dts
		}
		if minDTS > max {
			max = minDTS
		}
		newPTS := pts + dts + minDTS - min - max
		pts = newPTS
		dts = newPTS
	}
	if self.hasLastMuxDTS && dts < minDTS {
		if pts >= dts && minDTS > pts {
			pts = minDTS
		}
		dts = minDTS
	}
	return dts, pts
}

func (self *Muxer) WriteTrailer() (err error) {
	for _, stream := range self.streams {
		if stream.lastpkt != nil {
			if err = stream.writePacket(*stream.lastpkt, 0, 0); err != nil {
				return
			}
			stream.lastpkt = nil
		}
	}

	moov := &mp4io.Movie{}
	moov.Header = &mp4io.MovieHeader{
		PreferredRate:   1,
		PreferredVolume: 1,
		Matrix:          [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		NextTrackId:     2,
	}

	maxDur := time.Duration(0)
	timeScale := int64(10000)
	for _, stream := range self.streams {
		if err = stream.fillTrackAtom(); err != nil {
			return
		}
		dur := stream.tsToTime(stream.duration)
		stream.trackAtom.Header.Duration = int32(timeToTs(dur, timeScale))
		if dur > maxDur {
			maxDur = dur
		}
		moov.Tracks = append(moov.Tracks, stream.trackAtom)
	}
	moov.Header.TimeScale = int32(timeScale)
	moov.Header.Duration = int32(timeToTs(maxDur, timeScale))

	if err = self.bufw.Flush(); err != nil {
		return
	}

	var mdatsize int64
	if mdatsize, err = self.w.Seek(0, io.SeekCurrent); err != nil {
		return
	}
	if _, err = self.w.Seek(8, io.SeekStart); err != nil {
		return
	}
	taghdr := make([]byte, 8)
	pio.PutU64BE(taghdr, uint64(mdatsize))
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}

	if mdatsize > math.MaxUint32 {
		for _, stream := range self.streams {
			stream.sample.ChunkOffset.Is64bit = true
		}
	}

	if _, err = self.w.Seek(0, io.SeekEnd); err != nil {
		return
	}
	b := make([]byte, moov.Len())
	moov.Marshal(b)
	if _, err = self.w.Write(b); err != nil {
		return
	}

	return
}
