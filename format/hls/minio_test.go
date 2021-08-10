package hls

import (
	"testing"
)
func TestNewMinioCtx(t *testing.T)  {
	node,err := newHLSMinioTSRecordCtx("minio://192.168.1.123:9001/test/test/void/?accessKeyID=minio&secretAccessKey=minio123456&useSSL=false")
	t.Logf("minio :%v err:%v\n",node,err)
	return
}