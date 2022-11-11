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
	"os"
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
	ServiceFile iorpc.Service

	headerSize = binary.Size(RequestHeaders{})
)

type buffer []byte

type RequestHeaders struct {
	Key uint64
}

type DataHash [16]byte

type File struct {
	file *os.File
}

func (h *RequestHeaders) Encode(w io.Writer) (int, error) {
	return headerSize, binary.Write(w, binary.BigEndian, *h)
}

func (h *RequestHeaders) Decode(data []byte) error {
	return binary.Read(bytes.NewBuffer(data), binary.BigEndian, h)
}

func (h *DataHash) Encode(w io.Writer) (int, error) {
	n, err := w.Write((*h)[:])
	return n, err
}

func (h *DataHash) Decode(data []byte) error {
	*h = [16]byte{}
	n := copy((*h)[:], data)
	if n != 16 {
		return errors.New("invalid hash")
	}
	return nil
}

func (f *File) File() uintptr {
	return f.file.Fd()
}

func (f *File) Read(p []byte) (n int, err error) {
	return f.file.Read(p)
}

func (f *File) Close() error {
	return f.file.Close()
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
			sig := DataHash(hash)
			reader := buffer(hash[:])
			return &iorpc.Response{
				Headers: &sig,
				Body: iorpc.Body{
					Size:   uint64(len(hash)),
					Reader: &reader,
				},
			}, nil
		},
	)

	data, err := os.ReadFile("./tmp/testdata")
	if err != nil {
		log.Fatal(err)
	}

	ServiceFile, _ = Dispatcher.AddService(
		"File",
		func(clientAddr string, request iorpc.Request) (*iorpc.Response, error) {
			_ = request.Body.Close()
			header, ok := request.Headers.(*RequestHeaders)
			if !ok {
				return nil, errors.New("invalid header")
			}

			hash := sum(header.Key, data)
			sig := DataHash(hash)

			file, err := os.Open("./tmp/testdata")
			if err != nil {
				return nil, errors.Wrap(err, "open file")
			}

			return &iorpc.Response{
				Headers: &sig,
				Body: iorpc.Body{
					Offset: 0,
					Size:   uint64(len(data)),
					Reader: &File{file: file},
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

func sum(key uint64, data []byte) [16]byte {
	copied := make([]byte, len(data), len(data)+binary.Size(key))
	copy(copied, data)
	return md5.Sum(binary.AppendUvarint(copied, key))
}

func (b *buffer) Iovec() [][]byte {
	return [][]byte{*b}
}

func (b *buffer) Close() error {
	return nil
}

func (b *buffer) Read(p []byte) (n int, err error) {
	if len(*b) == 0 {
		return 0, io.EOF
	}
	n = copy(p, *b)
	if n < len(*b) {
		*b = (*b)[n:]
	} else {
		*b = nil
	}
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

	for i := 1; i < 8; i++ {
		client := NewClient(addr, i)
		defer client.Stop()
		for j := 0; j < 40; j++ {
			eg.Go(testEcho(ctx, client))
			eg.Go(testFile(ctx, client))
		}
	}

	err := eg.Wait()
	if err == context.DeadlineExceeded || err == context.Canceled {
		err = nil
	}
	a.Nil(err)
}

func testEcho(ctx context.Context, client *iorpc.Client) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				err := func() error {
					key := rand.Uint64()
					data := randomData(1024)
					hash := sum(key, data)
					reader := buffer(data)

					req := iorpc.Request{
						Service: ServiceEcho,
						Headers: &RequestHeaders{
							Key: key,
						},
						Body: iorpc.Body{
							Size:   uint64(len(data)),
							Reader: &reader,
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

					if *h != hash {
						return fmt.Errorf("invalid hash: %x != %x", hash, *h)
					}

					respData, err := io.ReadAll(resp.Body.Reader)
					if err != nil {
						return errors.Wrap(err, "read body")
					}

					if !bytes.Equal(respData, hash[:]) {
						return fmt.Errorf("invalid data: %s != %s", hash, respData)
					}
					return nil
				}()

				if err != nil {
					return errors.Wrap(err, "echo service")
				}
			}
		}
	}
}

func testFile(ctx context.Context, client *iorpc.Client) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				err := func() error {
					key := rand.Uint64()
					req := iorpc.Request{
						Service: ServiceFile,
						Headers: &RequestHeaders{
							Key: key,
						},
					}
					resp, err := client.Call(req)
					if err != nil {
						return errors.Wrap(err, "call")
					}
					defer resp.Body.Close()

					respData, err := io.ReadAll(resp.Body.Reader)
					if err != nil {
						return errors.Wrap(err, "read body")
					}

					hash := sum(key, respData)
					h, ok := resp.Headers.(*DataHash)
					if !ok {
						return errors.New("invalid header")
					}

					if *h != hash {
						return fmt.Errorf("invalid data: %x != %x", *h, hash)
					}
					return nil
				}()

				if err != nil {
					return errors.Wrap(err, "file service")
				}
			}
		}
	}
}
