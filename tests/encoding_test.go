package tests

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/hexilee/iorpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var (
	Dispatcher  = iorpc.NewDispatcher()
	ServiceEcho iorpc.Service

	headerSize = binary.Size(RequestHeaders{})
)

type buffer []byte
type RequestHeaders struct {
	Key uint64
}
type DataHash string

func (h *RequestHeaders) Encode(w io.Writer) (int, error) {
	return headerSize, binary.Write(w, binary.BigEndian, *h)
}

func (h *RequestHeaders) Decode(data []byte) error {
	return binary.Read(bytes.NewBuffer(data), binary.BigEndian, h)
}

func (h *DataHash) Encode(w io.Writer) (int, error) {
	data := []byte(*h)
	n, err := w.Write(data)
	return n, err
}

func (h *DataHash) Decode(data []byte) error {
	*h = DataHash(data)
	return nil
}

func init() {
	iorpc.RegisterHeaders(func() iorpc.Headers {
		return new(RequestHeaders)
	})
	iorpc.RegisterHeaders(func() iorpc.Headers {
		return new(DataHash)
	})

	ServiceEcho, _ = Dispatcher.AddService(
		"Echo",
		func(clientAddr string, request iorpc.Request) (*iorpc.Response, error) {
			defer request.Body.Close()
			header, ok := request.Headers.(*RequestHeaders)
			if !ok {
				return nil, errors.New("invalid header")
			}

			data, err := io.ReadAll(request.Body.Reader)
			if err != nil {
				return nil, errors.Wrap(err, "read body")
			}

			hash := sum(header.Key, data)
			str := DataHash(hash)
			return &iorpc.Response{
				Headers: &str,
				Body: iorpc.Body{
					Size:   uint64(len(hash)),
					Reader: buffer(hash),
				},
			}, nil
		},
	)
}

func NewServer() *iorpc.Server {
	addr := ":0"
	server := iorpc.NewTCPServer(addr, Dispatcher.HandlerFunc())
	server.Listener.Init(addr)
	return server
}

func sum(key uint64, data []byte) []byte {
	hasher := md5.New()
	binary.Write(hasher, binary.BigEndian, key)
	return hasher.Sum(data)
}

func (b buffer) Iovec() [][]byte {
	return [][]byte{b}
}

func (b buffer) Close() error {
	return nil
}

func (b buffer) Read(p []byte) (n int, err error) {
	n = copy(p, b)
	return
}

func NewClient(addr string) *iorpc.Client {
	c := iorpc.NewTCPClient(addr)
	c.Conns = 1
	c.DisableCompression = true
	c.Start()
	return c
}

func TestEncoding(t *testing.T) {
	a := assert.New(t)

	server := NewServer()
	go func() {
		if err := server.Serve(); err != nil {
			log.Fatalln(err.Error())
		}
	}()
	defer server.Stop()

	addr := server.Listener.ListenAddr()
	fmt.Println("server listen on", addr)
	client := NewClient(addr.String())
	defer client.Stop()

	key := uint64(0x1234567890abcdef)
	data := []byte("hello world")
	hash := sum(key, data)

	header := RequestHeaders{Key: key}
	body := iorpc.Body{
		Size:   uint64(len(data)),
		Reader: buffer(data),
	}

	resp, err := client.CallTimeout(iorpc.Request{
		Service: ServiceEcho,
		Headers: &header,
		Body:    body,
	}, time.Hour)
	a.Nil(err)
	a.NotNil(resp.Headers)
	defer resp.Body.Close()

	h, ok := resp.Headers.(*DataHash)
	a.True(ok)
	a.Equal(DataHash(hash), *h)

	respData, err := io.ReadAll(resp.Body.Reader)
	a.Nil(err)
	a.Equal(hash, respData)
}
