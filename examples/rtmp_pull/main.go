package main

import (
	"fmt"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/codec/h264parser"
	"github.com/nareix/joy4/format"
	"github.com/nareix/joy4/format/rtmp"
	"github.com/nareix/joy4/utils/bits/pio"
)

func init() {
	format.RegisterAll()
}

// as same as: ffmpeg -re -i projectindex.flv -c copy -f flv rtmp://localhost:1936/app/publish
func annexbToAVCC(pkt *av.Packet) {
	nals, typ := h264parser.SplitNALUs(pkt.Data)
	if typ == h264parser.NALU_ANNEXB {
		nalsizeByte := []byte{0, 0, 0, 0}
		pkt.Data = pkt.Data[0:0]
		for _, nal := range nals {
			pio.PutU32BE(nalsizeByte, uint32(len(nal)))
			pkt.Data = append(pkt.Data, nalsizeByte...)
			pkt.Data = append(pkt.Data, nal...)
		}
	} else {
		for _, nal := range nals {
			fmt.Printf("nal = %d size = %d \n", nal[0]&0x1f, len(nal))
		}
	}
}
func main() {
	rtmp.Debug = true
	conn, err := rtmp.Dial("rtmp://192.168.1.123:1935/live/test")
	// conn, _ := avutil.Create("rtmp://localhost:1936/app/publish")
	if err != nil {
		fmt.Printf("open err : %s\n", err.Error())
		return
	}
	s, err := conn.Streams()
	if err != nil {
		fmt.Printf("streams err : %s \n", err.Error())
		return
	}
	//fmt.Printf("streams : %v \n", s)

	var h264Idx int8 = -1

	for i, s := range s {
		switch s.Type() {
		case av.AAC:
		case av.H264:
			h264Idx = int8(i)
			break
		}
	}
	for {
		pkt, err := conn.ReadPacket()
		if err != nil {
			fmt.Printf("read err : %s\n", err.Error())
			return
		}
		if pkt.Idx == h264Idx {
			annexbToAVCC(&pkt)
		}

	}
}
