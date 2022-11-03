package iorpc

import (
	"encoding/binary"
	"io"
	"reflect"

	"github.com/pkg/errors"
)

var (
	requestStartLineSize  = binary.Size(requestStartLine{})
	responseStartLineSize = binary.Size(responseStartLine{})

	// headersConstructors[0] should be nil
	headersConstructors = make([]func() Headers, 1)
	headersIndexes      = make(map[reflect.Type]uint32)
)

func RegisterHeaders(constructor func() Headers) {
	h := constructor()
	headersIndexes[reflect.TypeOf(h)] = uint32(len(headersConstructors))
	headersConstructors = append(headersConstructors, constructor)
}

func newHeaders(index uint32) Headers {
	constructor := headersConstructors[index]
	if constructor == nil {
		return nil
	}
	return constructor()
}

func indexHeaders(h Headers) uint32 {
	if h == nil {
		return 0
	}
	return headersIndexes[reflect.TypeOf(h)]
}

type requestStartLine struct {
	Service                uint32
	HeaderType, HeaderSize uint32
	ID, BodySize           uint64
}

type responseStartLine struct {
	ErrorSize              uint32
	HeaderType, HeaderSize uint32
	ID, BodySize           uint64
}

type wireRequest struct {
	Service uint32
	ID      uint64
	Headers Headers
	Body    Body
}

type wireResponse struct {
	ID      uint64
	Headers Headers
	Body    Body
	Error   string
}

type messageEncoder struct {
	w            io.Writer
	headerBuffer Buffer
	stat         *ConnStats
}

func (e *messageEncoder) Close() error {
	// if e.zw != nil {
	// 	return e.zw.Close()
	// }
	return e.headerBuffer.Close()
}

func (e *messageEncoder) Flush() error {
	// if e.zw != nil {
	// 	if err := e.ww.Flush(); err != nil {
	// 		return err
	// 	}
	// 	if err := e.zw.Flush(); err != nil {
	// 		return err
	// 	}
	// }
	// if err := e.bw.Flush(); err != nil {
	// 	return err
	// }
	return nil
}

func (e *messageEncoder) encode(body *Body) error {
	if len(e.headerBuffer.Bytes()) > 0 {
		n, err := e.w.Write(e.headerBuffer.Bytes())
		if err != nil {
			e.stat.incWriteErrors()
			return err
		}
		e.stat.addHeadWritten(uint64(n))
	}

	if body.Size != 0 && body.Reader != nil {
		defer body.Close()
		spliced, err := body.spliceTo(e.w)
		if err != nil {
			return errors.Wrap(err, "splice body")
		}
		if spliced {
			e.stat.addBodyWritten(body.Size)
			e.stat.incWriteCalls()
			return nil
		}

		// fallback to io.Copy when not spliced
		nc, err := io.CopyN(e.w, body.Reader, int64(body.Size))
		if err != nil {
			e.stat.incWriteErrors()
			return err
		}
		e.stat.addBodyWritten(uint64(nc))
	}

	e.stat.incWriteCalls()
	return nil
}

func (e *messageEncoder) EncodeRequest(req wireRequest) error {
	headerIndex := indexHeaders(req.Headers)
	if req.Headers != nil && headerIndex != 0 {
		e.headerBuffer.Reset()
		if err := req.Headers.Encode(e.headerBuffer); err != nil {
			e.stat.incWriteErrors()
			return err
		}
	}

	if err := binary.Write(e.w, binary.BigEndian, requestStartLine{
		ID:         req.ID,
		Service:    req.Service,
		HeaderType: headerIndex,
		HeaderSize: uint32(len(e.headerBuffer.Bytes())),
		BodySize:   req.Body.Size,
	}); err != nil {
		e.stat.incWriteErrors()
		return err
	}

	e.stat.addHeadWritten(uint64(requestStartLineSize))
	return e.encode(&req.Body)
}

func (e *messageEncoder) EncodeResponse(resp wireResponse) error {
	headerIndex := indexHeaders(resp.Headers)
	if resp.Headers != nil && headerIndex != 0 {
		e.headerBuffer.Reset()
		if err := resp.Headers.Encode(e.headerBuffer); err != nil {
			e.stat.incWriteErrors()
			return err
		}
	}

	respErr := []byte(resp.Error)

	if err := binary.Write(e.w, binary.BigEndian, responseStartLine{
		ID:         resp.ID,
		ErrorSize:  uint32(len(respErr)),
		HeaderType: headerIndex,
		HeaderSize: uint32(len(e.headerBuffer.Bytes())),
		BodySize:   resp.Body.Size,
	}); err != nil {
		e.stat.incWriteErrors()
		return err
	}

	e.stat.addHeadWritten(uint64(responseStartLineSize))

	if len(respErr) > 0 {
		n, err := e.w.Write(respErr)
		if err != nil {
			e.stat.incWriteErrors()
			return err
		}
		e.stat.addHeadWritten(uint64(n))
	}

	return e.encode(&resp.Body)
}

func newMessageEncoder(w io.Writer, s *ConnStats) *messageEncoder {
	return &messageEncoder{
		w:            w,
		headerBuffer: bufferAllocator(1 << 10),
		stat:         s,
	}
}

type messageDecoder struct {
	closeBody    bool
	r            io.Reader
	headerBuffer Buffer
	stat         *ConnStats
}

func (d *messageDecoder) Close() error {
	return d.headerBuffer.Close()
}

func (d *messageDecoder) decodeBody(size int64) (body io.ReadCloser, err error) {
	defer func() {
		if body != nil && d.closeBody {
			body.Close()
		}
	}()

	if size == 0 {
		return noopBody{}, nil
	}

	buf := bufferAllocator(int(size))
	bytes, err := io.CopyN(buf, d.r, size)
	if err != nil {
		buf.Close()
		return nil, err
	}
	d.stat.addBodyRead(uint64(bytes))
	body = buf
	return
}

func (d *messageDecoder) DecodeRequest(req *wireRequest) error {
	var startLine requestStartLine
	if err := binary.Read(d.r, binary.BigEndian, &startLine); err != nil {
		d.stat.incReadErrors()
		return err
	}
	d.stat.addHeadRead(uint64(requestStartLineSize))

	req.ID = startLine.ID
	req.Service = startLine.Service
	req.Body.Size = startLine.BodySize

	if req.Headers = newHeaders(startLine.HeaderType); req.Headers != nil && startLine.HeaderSize > 0 {
		d.headerBuffer.Reset()
		if _, err := io.CopyN(d.headerBuffer, d.r, int64(startLine.HeaderSize)); err != nil {
			d.stat.incReadErrors()
			return err
		}
		d.stat.addHeadRead(uint64(startLine.HeaderSize))
		if err := req.Headers.Decode(d.headerBuffer.Bytes()); err != nil {
			d.stat.incReadErrors()
			return err
		}
	}

	buf, err := d.decodeBody(int64(req.Body.Size))
	if err != nil {
		return err
	}
	req.Body.Reader = buf
	d.stat.incReadCalls()
	return nil
}

func (d *messageDecoder) DecodeResponse(resp *wireResponse) error {
	var startLine responseStartLine
	if err := binary.Read(d.r, binary.BigEndian, &startLine); err != nil {
		d.stat.incReadErrors()
		return err
	}
	d.stat.addHeadRead(uint64(responseStartLineSize))

	resp.ID = startLine.ID
	resp.Body.Size = startLine.BodySize

	if startLine.ErrorSize > 0 {
		respErr := make([]byte, startLine.ErrorSize)
		if _, err := io.ReadFull(d.r, respErr); err != nil {
			d.stat.incReadErrors()
			return errors.Wrapf(err, "read response error: size(%d)", startLine.ErrorSize)
		}
		d.stat.addHeadRead(uint64(startLine.ErrorSize))
		resp.Error = string(respErr)
	}

	if resp.Headers = newHeaders(startLine.HeaderType); resp.Headers != nil && startLine.HeaderSize > 0 {
		d.headerBuffer.Reset()
		if _, err := io.CopyN(d.headerBuffer, d.r, int64(startLine.HeaderSize)); err != nil {
			d.stat.incReadErrors()
			return errors.Wrapf(err, "read response headers: size(%d)", startLine.HeaderSize)
		}
		d.stat.addHeadRead(uint64(startLine.HeaderSize))
		if err := resp.Headers.Decode(d.headerBuffer.Bytes()); err != nil {
			d.stat.incReadErrors()
			return err
		}
	}

	buf, err := d.decodeBody(int64(resp.Body.Size))
	if err != nil {
		return err
	}
	resp.Body.Reader = buf
	d.stat.incReadCalls()
	return nil
}

func newMessageDecoder(r io.Reader, s *ConnStats, closeBody bool) *messageDecoder {
	headerBuffer := bufferAllocator(1 << 10)
	return &messageDecoder{
		r:            r,
		headerBuffer: headerBuffer,
		stat:         s,
		closeBody:    closeBody,
	}
}
