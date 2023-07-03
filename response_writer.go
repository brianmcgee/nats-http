package natshttp

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-http-utils/headers"

	"github.com/nats-io/nats.go"
)

const (
	SmallBodySize     = 4 * 1024  // 4 Kb
	DefaultHeaderSize = 10 * 1024 // 10 Kb
)

type ResponseWriter struct {
	conn       *nats.Conn
	subject    string
	maxMsgSize int

	buf            *bytes.Buffer
	headers        http.Header
	headersWritten bool

	chunked       bool
	contentLength int64

	flushCount  int
	flushBuffer []byte
}

func NewResponseWriter(conn *nats.Conn, subject string) (*ResponseWriter, error) {
	if conn == nil {
		return nil, errors.New("conn cannot be nil")
	}
	if subject == "" {
		return nil, errors.New("subject cannot be empty")
	}

	return &ResponseWriter{
		conn:          conn,
		subject:       subject,
		maxMsgSize:    int(conn.MaxPayload()),
		headers:       make(http.Header),
		buf:           bytes.NewBuffer(nil),
		contentLength: -1,
	}, nil
}

func (r *ResponseWriter) Header() http.Header {
	return r.headers
}

func (r *ResponseWriter) WriteHeader(statusCode int) {
	if r.headersWritten {
		return
	}

	h := r.headers

	// set status code and message
	h.Set(HeaderStatus, http.StatusText(statusCode))
	h.Set(HeaderStatusCode, strconv.FormatInt(int64(statusCode), 10))

	// attempt to determine content length
	if r.flushCount == 0 && h.Get(headers.ContentLength) == "" {
		buffered := r.buf.Len()

		// if the total size of all written data is under a few KB and there are no flush calls, the
		// Content-Length header is added automatically, otherwise we assume a chunked transfer
		if buffered > 0 && buffered <= SmallBodySize {
			h.Set(headers.ContentLength, strconv.Itoa(buffered))
		} else if buffered > SmallBodySize {
			h.Set(headers.TransferEncoding, "chunked")
		}
	}

	if h.Get(headers.TransferEncoding) == "chunked" {
		// transfer encoding takes precedence, remove content length if present
		h.Del(headers.ContentLength)
	}

	// capture resultant content length
	if h.Get(headers.ContentLength) != "" {
		var err error
		r.contentLength, err = strconv.ParseInt(h.Get(headers.ContentLength), 10, 64)
		if err != nil {
			// todo log this error
		}
	}

	// create a sample msg for accurate sizing
	// todo replace this with a lighter calculation
	msg := nats.NewMsg(r.subject)
	msg.Header = nats.Header(h)

	// determine if this will be a single message response or multiple
	totalBytes := msg.Size() + int(r.contentLength)
	r.chunked = (totalBytes > r.maxMsgSize) || h.Get(headers.TransferEncoding) == "chunked"

	r.headersWritten = true
}

func (r *ResponseWriter) Write(b []byte) (n int, err error) {
	n, err = r.buf.Write(b)
	if err != nil {
		return
	}

	if !r.headersWritten {
		r.WriteHeader(http.StatusOK)

		// try to detect content type
		if r.headers.Get(headers.ContentType) == "" {
			r.headers.Set(headers.ContentType, http.DetectContentType(b))
		}
	}

	if r.buf.Len() >= r.maxMsgSize {
		err = r.flush()
	}

	return
}

func (r *ResponseWriter) flush() (err error) {
	// initialise the byte arrays used for reading from the write buffer
	if r.flushBuffer == nil {
		r.flushBuffer = make([]byte, r.maxMsgSize)
	}

	var n int

	for {
		msg := nats.NewMsg(r.subject)

		// add headers to first msg
		if r.flushCount == 0 {
			msg.Header = nats.Header(r.headers)
		}

		// determine max size of the data field
		dataSize := r.maxMsgSize - msg.Size()

		// resize flush buffer if required
		flushBuffer := r.flushBuffer
		if dataSize < len(flushBuffer) {
			flushBuffer = flushBuffer[:dataSize]
		}

		// read into the flush buffer
		n, err = r.buf.Read(flushBuffer)

		// initialise data to match the number of bytes that were read and copy from the flush buffer
		msg.Data = make([]byte, n)
		copied := copy(msg.Data, flushBuffer)

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		if copied != n {
			return errors.New("natshttp: failed to copy all bytes into msg.Data")
		}

		if err = r.conn.PublishMsg(msg); err != nil {
			return err
		}

		r.flushCount += 1
	}
}

func (r *ResponseWriter) Close() error {
	// flush any pending chunks
	if err := r.flush(); err != nil {
		return err
	}

	// if status hasn't been set, yet we assume a status of OK
	if !r.headersWritten {
		r.WriteHeader(http.StatusOK)
	}

	// if no msgs have been sent yet, we generate and send a single message with the headers
	// this happens in the case of HEAD responses for example
	if r.flushCount == 0 {
		msg := nats.NewMsg(r.subject)
		msg.Header = nats.Header(r.headers)
		return r.conn.PublishMsg(msg)
	}

	if r.chunked {
		// send empty message to indicate end of chunk stream
		msg := nats.NewMsg(r.subject)
		return r.conn.PublishMsg(msg)
	}

	return nil
}
