package hls

import (
    "log"
	"net/url"
	"context"
	"strings"
	"fmt"
	"sync"
	"bytes"
    "github.com/minio/minio-go/v7"
    "github.com/minio/minio-go/v7/pkg/credentials"
)

type hlsMinioTSRecordCtx struct{
	client *minio.Client
	bucket string
	dir	   string
	bufPool sync.Pool

	tsbuf	bytes.Buffer
	m3u8buf bytes.Buffer

}

// minio://192.168.1.123:9001/<bucketname>/path/of/Dir?accessKeyID=minio&secretAccessKey=minio123456&useSSL=false
func newHLSMinioTSRecordCtx(Url string)(*hlsMinioTSRecordCtx,error){

	u, err := url.Parse(Url)

	if err != nil {
		log.Println("parser url failed:",err)
		return nil,err
	}

	q := u.Query()

	
	eles := strings.Split(strings.Trim(strings.Trim(u.Path," "),"/"),"/")
	if len(eles)<=0{
		return nil,fmt.Errorf("no bucket name")
	}

	buketName := eles[0]

	dir := strings.Join(eles[1:],"/")


	minioClient, err := minio.New(u.Host, &minio.Options{
        Creds:  credentials.NewStaticV4(q.Get("accessKeyID"), q.Get("secretAccessKey"), ""),
        Secure: q.Get("useSSL") == "true",
    })

	if err != nil{
		log.Println("minio.New failed:",err)
		return nil,err
	}

	found,err := minioClient.BucketExists(context.Background(),buketName)

	if err != nil{
		log.Println("BucketExists failed:",err)
		return nil,err
	}

	if !found{
		err = minioClient.MakeBucket(context.Background(),buketName,minio.MakeBucketOptions{ObjectLocking: false})
		if err != nil{
			log.Println("MakeBucket failed:",err)
			return nil,err
		}
	}
	return &hlsMinioTSRecordCtx{
		client:minioClient,
		bucket:buketName,
		dir:dir,
		bufPool:sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	},nil
}

func (ctx *hlsMinioTSRecordCtx) uploadToMinio(fileName string,data *bytes.Buffer) error{

	buf := ctx.bufPool.Get().(*bytes.Buffer)
	buf.Write(data.Bytes())
	_, err := ctx.client.PutObject(context.Background(),ctx.bucket,ctx.dir+"/"+fileName,buf,int64(buf.Len()),minio.PutObjectOptions{})
	ctx.bufPool.Put(buf)
	return err
}
func (ctx *hlsMinioTSRecordCtx) onTS(filename string,shdr,data []byte) error{
	ctx.tsbuf.Reset()

	ctx.tsbuf.Write(shdr)
	ctx.tsbuf.Write(data)

	return ctx.uploadToMinio(filename,&ctx.tsbuf)
}

func (ctx *hlsMinioTSRecordCtx) onM3U8Append(isFirst bool,append []byte) error{
	ctx.m3u8buf.Write(append)
	return ctx.uploadToMinio("hls.m3u8",&ctx.m3u8buf)
}