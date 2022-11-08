package tests

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"testing"
	"time"

	"github.com/hexilee/iorpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
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
	hash := md5.Sum(binary.AppendUvarint(data, key))
	return hash[:]
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

func NewClient(addr string, conns int) *iorpc.Client {
	c := iorpc.NewTCPClient(addr)
	c.Conns = conns
	c.DisableCompression = true
	c.Start()
	return c
}

func randomData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.Uint32())
	}
	return b
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

	addr := server.Listener.ListenAddr().String()

	eg := &errgroup.Group{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for i := 0; i < 8; i++ {
		client := NewClient(addr, i)
		defer client.Stop()
		for j := 0; j < 20; j++ {
			eg.Go(func() error {
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						err := func() error {
							key := rand.Uint64()
							data := randomData(1024)
							hash := sum(key, data)

							req := iorpc.Request{
								Service: ServiceEcho,
								Headers: &RequestHeaders{
									Key: key,
								},
								Body: iorpc.Body{
									Size:   uint64(len(data)),
									Reader: buffer(data),
								},
							}
							resp, err := client.Call(req)
							if err != nil {
								return errors.Wrap(err, "call")
							}
							defer resp.Body.Close()

							h, ok := resp.Headers.(*DataHash)
							if !ok {
								return errors.New("invalid header")
							}

							if DataHash(hash) != *h {
								return fmt.Errorf("invalid hash: %s != %s", hash, *h)
							}

							respData, err := io.ReadAll(resp.Body.Reader)
							if err != nil {
								return errors.Wrap(err, "read body")
							}

							if !bytes.Equal(respData, hash) {
								return fmt.Errorf("invalid data: %s != %s", hash, respData)
							}
							return nil
						}()

						if err != nil {
							return err
						}
					}
				}
			})
		}
	}

	err := eg.Wait()
	if err == context.DeadlineExceeded {
		err = nil
	}
	a.Nil(err)
}
