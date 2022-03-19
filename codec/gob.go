package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodesc struct {
	enc  *gob.Encoder
	dec  *gob.Decoder
	buf  *bufio.Writer
	conn io.ReadWriteCloser
}

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodesc{conn: conn,
		dec: gob.NewDecoder(conn),
		buf: buf,
		enc: gob.NewEncoder(buf),
	}
}
func (this *GobCodesc) ReadHeader(h *Header) error {
	return this.dec.Decode(h)
}
func (this *GobCodesc) ReadBody(b interface{}) error {
	return this.dec.Decode(b)
}
func (this *GobCodesc) Write(h *Header, b interface{}) (err error) {
	defer func() {
		this.buf.Flush()
		if err != nil {
			this.Close()
		}
	}()
	if err = this.enc.Encode(h); err != nil {
		log.Println("rpc codec: gob error encoding header: ", err)
		return
	}
	if err = this.enc.Encode(b); err != nil {
		log.Println("rpc codec: gob error encoding body: ", err)
		return
	}
	return nil
}
func (this *GobCodesc) Close() error {
	return this.conn.Close()
}
