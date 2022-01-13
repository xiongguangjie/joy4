package main

import (
	"fmt"
	"github.com/nareix/joy4/format"
	"github.com/nareix/joy4/format/rtmp"
)

func init() {
	format.RegisterAll()
}

// as same as: ffmpeg -re -i projectindex.flv -c copy -f flv rtmp://localhost:1936/app/publish

func main() {
	rtmp.Debug = true
	conn, err := rtmp.Dial("rtmp://192.168.1.123:1935/live/test")
	// conn, _ := avutil.Create("rtmp://localhost:1936/app/publish")
	if err != nil {
		fmt.Printf("open err : %s\n", err.Error())
		return
	}
	_, err = conn.Streams()
	if err != nil {
		fmt.Printf("streams err : %s \n", err.Error())
		return
	}

	for {
		_, err := conn.ReadPacket()
		if err != nil {
			fmt.Printf("read err : %s\n", err.Error())
			return
		}
	}
}
