package hls

import (
	"os"
	"path/filepath"
)

type hlsTSRecordCtx struct{
	Dir string
}
func (ctx *hlsTSRecordCtx) onTS(filename string,shdr,data []byte) error{
	dst := filepath.Join(ctx.Dir,filename)
	file , err := os.OpenFile(dst,os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil{
		return err
	}
	defer file.Close()
	_,err = file.Write(shdr)
	if err != nil{
		os.Remove(dst)
		return err
	}
	_,err = file.Write(data)
	if err != nil{
		os.Remove(dst)
		return err
	}
	return nil
}

func (ctx *hlsTSRecordCtx) onM3U8Append(isFirst bool,append []byte) error{
	m3u8filename := filepath.Join(ctx.Dir,"hls.m3u8")
	var err error
	var m3u8file *os.File
	if isFirst{
		m3u8file,err = os.OpenFile(m3u8filename,os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0644)
	}else{
		m3u8file,err = os.OpenFile(m3u8filename,os.O_APPEND|os.O_WRONLY,0644)
	}
	if err != nil{
		return err
	}
	m3u8file.Write(append)
	m3u8file.Close()
	return nil
}