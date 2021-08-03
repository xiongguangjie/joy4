package hls

import (
	"time"
	"os"
	"fmt"
	"bytes"
	"path/filepath"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/ts"
)

type Muxer struct{
	buf     bytes.Buffer
	mux     *ts.Muxer
	streams []av.CodecData
	shdr 	[]byte
	dir 	string
	vidx    int8
	aidx 	int8
	tsIdx 	uint
	segDuration time.Duration
	curSegmentFirstDTS time.Duration
	curVideoFrameNum int 
	lastDTS time.Duration
	m3u8filename string
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

	return &Muxer{
		dir:Dir,
		vidx:-1,
		aidx:-1,
		m3u8filename:filepath.Join(Dir,"hls.m3u8"),
		segDuration:segDuration,
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
	dst := filepath.Join(self.dir,fileName)
	file , err := os.OpenFile(dst,os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil{
		return err
	}
	defer file.Close()

	_,err = file.Write(self.shdr)
	if err != nil{
		os.Remove(dst)
		return err
	}
	_,err = file.Write(self.buf.Bytes())
	if err != nil{
		os.Remove(dst)
		return err
	}

	// flush m3u8

	if self.tsIdx == 0{
		// first ts
		m3u8file,err := os.OpenFile(self.m3u8filename,os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0644)
		if err != nil{
			os.Remove(dst)
			return err
		}
		fmt.Fprintf(m3u8file,"#EXTM3U\n")
		fmt.Fprintf(m3u8file,"#EXT-X-PLAYLIST-TYPE:EVENT\n")
		fmt.Fprintf(m3u8file,"#EXT-X-TARGETDURATION:10\n")
		fmt.Fprintf(m3u8file,"#EXT-X-VERSION:4\n")
		fmt.Fprintf(m3u8file,"#EXT-X-MEDIA-SEQUENCE:0\n")

		dur := float32(newStartTime - self.curSegmentFirstDTS)/float32(time.Second)
		fmt.Fprintf(m3u8file,"#EXTINF:%.3f,\n%s\n",dur,fileName)

		m3u8file.Close()
	}else{
		m3u8file,err := os.OpenFile(self.m3u8filename,os.O_APPEND|os.O_WRONLY,0644)
		if err != nil{
			os.Remove(dst)
			return err
		}
		dur := float32(newStartTime - self.curSegmentFirstDTS)/float32(time.Second)
		fmt.Fprintf(m3u8file,"#EXTINF:%.3f,\n%s\n",dur,fileName)
		m3u8file.Close()
	}
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

	m3u8file,err := os.OpenFile(self.m3u8filename,os.O_APPEND|os.O_WRONLY,0644)
	if err != nil{
		fmt.Printf("hls muxer not ocur \n")
	} 
	fmt.Fprintf(m3u8file,"#EXT-X-ENDLIST\n")
	m3u8file.Close()
	return err
}