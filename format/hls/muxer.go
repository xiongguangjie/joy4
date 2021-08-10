package hls

import (
	"time"
	"os"
	"fmt"
	"bytes"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/ts"
)

type Muxer struct{
	buf     bytes.Buffer
	mux     *ts.Muxer
	streams []av.CodecData
	shdr 	[]byte
	vidx    int8
	aidx 	int8
	tsIdx 	uint
	segDuration time.Duration
	curSegmentFirstDTS time.Duration
	curVideoFrameNum int 
	lastDTS time.Duration

	onTS func (filename string,shdr,data []byte) error
	onM3U8Append func (isFirst bool,append []byte) error
	//tslistInfo []tsSegmentInfo
}
/*
type tsSegmentInfo struct{
	name string
	duration time.Duration
}
*/

func NewMuxer(Dir string,segDuration time.Duration) (*Muxer,error){
	s, err := os.Lstat(Dir)

	if err == nil && !s.IsDir(){
		return nil,fmt.Errorf("%s is a file",Dir)
	}

	if os.IsNotExist(err) {
		os.MkdirAll(Dir,os.ModePerm)
	}
	fileCtx := &hlsTSRecordCtx{
		Dir:Dir,
	}


	return &Muxer{
		vidx:-1,
		aidx:-1,
		segDuration:segDuration,
		onTS:fileCtx.onTS,
		onM3U8Append:fileCtx.onM3U8Append,
	},nil
}

func NewMuxerMinio(url string,segDuration time.Duration) (*Muxer,error){
	minioCtx,err := newHLSMinioTSRecordCtx(url)
	
	if err != nil{
		return nil,err
	}

	return &Muxer{
		vidx:-1,
		aidx:-1,
		segDuration:segDuration,
		onTS:minioCtx.onTS,
		onM3U8Append:minioCtx.onM3U8Append,
	},nil
}

func (self *Muxer) WriteHeader(streams []av.CodecData) (err error) {
	self.buf.Reset()
	self.mux = ts.NewMuxer(&self.buf)

	if err := self.mux.WriteHeader(streams); err != nil {
		return err
	}
	self.vidx = -1
	for i, cd := range streams {
		if cd.Type().IsVideo() {
			self.vidx = int8(i)
		}else if cd.Type().IsAudio(){
			self.aidx = int8(i)
		}
	}

	self.streams = []av.CodecData{}
	self.streams = append(self.streams,streams...)

	self.shdr = make([]byte, self.buf.Len())
	copy(self.shdr, self.buf.Bytes())
	self.buf.Reset()
	self.curSegmentFirstDTS = -1
	return nil
}

func (self *Muxer) saveTS(newStartTime time.Duration) error{
	fileName := fmt.Sprintf("%d.ts",self.tsIdx)
	
	if self.onTS != nil{
		self.onTS(fileName,self.shdr,self.buf.Bytes())
	}

	var m3u8buf  = bytes.NewBuffer([]byte{})

	if self.tsIdx == 0{
		// first ts
		fmt.Fprintf(m3u8buf,"#EXTM3U\n")
		fmt.Fprintf(m3u8buf,"#EXT-X-PLAYLIST-TYPE:EVENT\n")
		fmt.Fprintf(m3u8buf,"#EXT-X-TARGETDURATION:10\n")
		fmt.Fprintf(m3u8buf,"#EXT-X-VERSION:4\n")
		fmt.Fprintf(m3u8buf,"#EXT-X-MEDIA-SEQUENCE:0\n")

		dur := float32(newStartTime - self.curSegmentFirstDTS)/float32(time.Second)
		fmt.Fprintf(m3u8buf,"#EXTINF:%.3f,\n%s\n",dur,fileName)
	}else{
		
		dur := float32(newStartTime - self.curSegmentFirstDTS)/float32(time.Second)
		fmt.Fprintf(m3u8buf,"#EXTINF:%.3f,\n%s\n",dur,fileName)
	}

	if self.onM3U8Append != nil{
		self.onM3U8Append(self.tsIdx == 0,m3u8buf.Bytes())
	}
	m3u8buf.Reset()
	self.tsIdx++
	/*
	info := tsSegmentInfo{
		name:fileName,
		duration:pkt.Time - self.preSegmentLastDTS,
	}
	*/
	//self.curSegmentFirstDTS = newStartTime
	//self.tslistInfo = append(self.tslistInfo,info)

	return nil
}
func (self *Muxer) canFlushTs(pkt av.Packet) bool{
	if self.aidx>=0 || self.vidx>=0{
		if self.vidx>=0{
			// has video 
			dur := pkt.Time - self.curSegmentFirstDTS
			if pkt.Idx == self.vidx && pkt.IsKeyFrame && self.buf.Len()>0 && self.curVideoFrameNum>0 && dur>=self.segDuration{
				return true
			}
		}else{
			// only audio
			dur := pkt.Time - self.curSegmentFirstDTS
			if pkt.Idx == self.aidx && pkt.IsKeyFrame && self.buf.Len()>0 && dur>=self.segDuration{
				return true
			}
		}
	}else{
		fmt.Printf("hls muxer not ocur \n")
	}
	return false
}
func (self *Muxer) WritePacket(pkt av.Packet) (err error){

	if self.canFlushTs(pkt){
		//can flush ts
		err := self.saveTS(pkt.Time)
		if err != nil{
			fmt.Printf("hls muxer not ocur \n")
		}else{
			self.buf.Reset()
		}
		self.curVideoFrameNum = 0
		self.curSegmentFirstDTS = pkt.Time
	}
	if pkt.Idx == self.vidx{
		self.curVideoFrameNum++
		self.lastDTS = pkt.Time
	}else{
		self.lastDTS = pkt.Time
	}
	return self.mux.WritePacket(pkt)
}

func (self *Muxer) WriteTrailer() (err error) {
	self.mux.WriteTrailer()
	self.saveTS(self.lastDTS)
	self.buf.Reset()

	m3u8buf := bytes.NewBuffer([]byte{})
	if err != nil{
		fmt.Printf("hls muxer not ocur \n")
	} 
	fmt.Fprintf(m3u8buf,"#EXT-X-ENDLIST\n")

	if self.onM3U8Append != nil{
		self.onM3U8Append(self.tsIdx == 0,m3u8buf.Bytes())
	}

	m3u8buf.Reset()
	return err
}