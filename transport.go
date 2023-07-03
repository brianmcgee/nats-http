package natshttp

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-http-utils/headers"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
)

const (
	HeaderStatus     = "X-Status"
	HeaderStatusCode = "X-Status-Code"

	UrlScheme = "httpn"

	ErrInvalidUrl = errors.ConstError("natshttp: urls must of be of the form 'httpn://a.valid.nats.subject/foo/bar?query=baz")
)

type Transport struct {
	Conn *nats.Conn

	PendingMsgsLimit  int
	PendingBytesLimit int

	maxMsgSize int
}

func IsChunkedRequest(msg *nats.Msg, msgSize int) (bool, error) {
	var err error
	var contentLength int64 = -1

	if msg.Header.Get(headers.ContentLength) != "" {
		contentLength, err = strconv.ParseInt(msg.Header.Get(headers.ContentLength), 10, 64)
		if err != nil {
			return false, err
		}
	}

	chunked := msg.Header.Get(headers.TransferEncoding) == "chunked"
	chunked = chunked || (msg.Size()-len(msg.Data)+int(contentLength)) > msgSize

	return chunked, nil
}

func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if t.maxMsgSize == 0 {
		t.maxMsgSize = int(t.Conn.MaxPayload())
	}

	if t.PendingMsgsLimit == 0 {
		t.PendingMsgsLimit = nats.DefaultSubPendingMsgsLimit
	}

	if t.PendingBytesLimit == 0 {
		t.PendingBytesLimit = nats.DefaultSubPendingBytesLimit
	}

	// create response
	resp = &http.Response{
		Request: req,
	}

	// create a new inbox and subscribe to it
	inbox := t.Conn.NewRespInbox()
	sub, err := t.Conn.SubscribeSync(inbox)

	// adjust the pending limits for slow consumer detection
	if err = sub.SetPendingLimits(t.PendingMsgsLimit, t.PendingBytesLimit); err != nil {
		return nil, err
	}

	// todo handle cleanup of subscription in case of error properly
	if err != nil {
		return nil, err
	}

	// convert the request into a stream of one or more messages
	reqMsgs, err := t.httpRequestToMsgs(req)
	if err != nil {
		return nil, err
	}

	firstMsg := <-reqMsgs
	if firstMsg.Error != nil {
		return nil, firstMsg.Error
	}

	// set reply to our inbox and publish
	firstMsg.Value.Reply = inbox
	if err = t.Conn.PublishMsg(firstMsg.Value); err != nil {
		return nil, err
	}

	// determine if the request is chunked or not
	chunked, err := IsChunkedRequest(firstMsg.Value, t.maxMsgSize)
	if err != nil {
		return nil, err
	}

	// if the request is not chunked we can start processing the responses
	if !chunked {
		err = t.processResponses(resp, sub)
		return resp, err
	}

	// otherwise we wait for the chunk handshake
	ctx := req.Context()
	var chunkSubject string

	// listen for the first response msg which will contain a private inbox for sending the remainder of the chunks
	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, err
	}

	chunkSubject = msg.Reply
	if chunkSubject == "" {
		return nil, errors.New("natshttp: invalid chunk handshake")
	}

	// send the remainder of the chunks
Loop:
	for {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case chunk, ok := <-reqMsgs:
			if !ok {
				break Loop
			}
			if chunk.Error != nil {
				return nil, chunk.Error
			}
			chunk.Value.Subject = chunkSubject
			if err = t.Conn.PublishMsg(chunk.Value); err != nil {
				return nil, err
			}
		}
	}

	// process responses
	err = t.processResponses(resp, sub)
	return resp, err
}

func (t *Transport) processResponses(resp *http.Response, sub *nats.Subscription) error {
	ctx := resp.Request.Context()

	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		return err
	}

	h := msg.Header

	statusCode, err := strconv.ParseInt(h.Get(HeaderStatusCode), 10, 64)
	if err != nil {
		return err
	}

	resp.Status = h.Get(HeaderStatus)
	resp.StatusCode = int(statusCode)

	// copy headers
	resp.Header = make(http.Header)
	for key, values := range h {
		for _, value := range values {
			resp.Header.Add(key, value)
		}
	}

	transferEncoding := resp.Header.Get("Transfer-Encoding")
	if transferEncoding != "" {
		resp.Header.Del("Content-Length")
		resp.TransferEncoding = []string{transferEncoding}
	}

	contentLength := resp.Header.Get("Content-Length")
	if contentLength != "" {
		cl, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			return err
		}
		resp.ContentLength = cl
	}

	totalBytes := msg.Size() - len(msg.Data) + int(resp.ContentLength)

	if transferEncoding != "chunked" && totalBytes < t.maxMsgSize {
		resp.Body = io.NopCloser(bytes.NewReader(msg.Data))
		return nil
	}

	bodyReader, err := NewChunkReader(msg, sub, ctx)
	if err != nil {
		return err
	}

	resp.Body = bodyReader

	return nil
}

func (t *Transport) httpRequestToMsgs(req *http.Request) (chan Result[*nats.Msg], error) {
	var err error
	msgs := make(chan Result[*nats.Msg], 8)

	if req.URL == nil {
		_ = req.Body.Close()
		return nil, errors.New("natshttp: nil Request.URL")
	}

	if req.Header == nil {
		_ = req.Body.Close()
		return nil, errors.New("natshttp: nil Request.Header")
	}

	if req.URL.Scheme != UrlScheme {
		_ = req.Body.Close()
		return nil, ErrInvalidUrl
	}

	if req.URL.Host == "" {
		_ = req.Body.Close()
		return nil, errors.New("natshttp: no Host in request URL")
	}

	msg := nats.NewMsg("")

	if err = ReqToMsg(req, msg); err != nil {
		return nil, err
	}

	h := msg.Header

	if len(req.TransferEncoding) > 0 {
		h.Set(headers.TransferEncoding, strings.Join(req.TransferEncoding, ","))
	}

	for key, values := range req.Header {
		for _, value := range values {
			msg.Header.Add(key, value)
		}
	}

	// empty body so return the msg with just headers
	if req.Body == nil {
		msgs <- Result[*nats.Msg]{Value: msg}
		return msgs, nil
	}

	// determine if this is a chunked transfer or not
	chunked := false

	te := req.TransferEncoding
	if len(te) > 0 && te[0] == "chunked" {
		chunked = true
	}

	// check if we will breach conn.MaxPayload()
	if (msg.Size() + int(req.ContentLength)) > t.maxMsgSize {
		chunked = true
	}

	// if it is not chunked, copy the body
	if !chunked {
		msg.Data, err = io.ReadAll(req.Body)
		_ = req.Body.Close()

		if err != nil {
			return nil, err
		}

		if msg.Header.Get(headers.ContentType) == "" {
			msg.Header.Set(headers.ContentType, http.DetectContentType(msg.Data))
		}

		msgs <- Result[*nats.Msg]{Value: msg}
		close(msgs)

		return msgs, nil
	}

	var n int
	readBuffer := make([]byte, t.maxMsgSize)

	go func() {
		// initialise to the first msg under construction
		nextMsg := msg

		defer func() {
			close(msgs)
			_ = req.Body.Close()
		}()

		for {
			select {
			case <-req.Context().Done():
				msgs <- Result[*nats.Msg]{Error: req.Context().Err()}
			default:

				// determine the max size for the data field
				dataSize := t.maxMsgSize - nextMsg.Size()

				// we allow some space for the subject which will be set later
				if nextMsg.Subject == "" {
					dataSize -= 256
				}

				// resize the data buffer to match the max data size
				dataBuffer := readBuffer
				if len(dataBuffer) > dataSize {
					dataBuffer = dataBuffer[:dataSize]
				}

				// read into the data buffer
				n, err = req.Body.Read(dataBuffer)

				if err != nil && err != io.EOF {
					msgs <- Result[*nats.Msg]{Error: err}
					return
				}

				// copy from the data buffer into the msg
				nextMsg.Data = make([]byte, n)
				copied := copy(nextMsg.Data, dataBuffer)

				if copied != n {
					msgs <- Result[*nats.Msg]{Error: errors.New("natshttp: failed to copy all bytes into msg.Data")}
					return
				}

				msgs <- Result[*nats.Msg]{Value: nextMsg}

				if err == io.EOF {
					// send an empty message to indicate the end of the stream of chunks
					nextMsg = nats.NewMsg("")
					msgs <- Result[*nats.Msg]{Value: nextMsg}
					return
				}

				// subsequent msgs will have their subject set to the private inbox received as part of the chunk
				// handshake
				nextMsg = nats.NewMsg("")
			}
		}
	}()

	return msgs, nil
}
