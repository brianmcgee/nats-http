package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-http-utils/headers"

	"github.com/nats-io/nats.go"
)

var NoOpErrorHandler = func(_ error) {
}

type Server struct {
	Conn    *nats.Conn
	Subject string
	Group   string

	Handler      http.Handler
	ErrorHandler func(error)

	PendingMsgsLimit  int
	PendingBytesLimit int

	sub        *nats.Subscription
	maxMsgSize int
}

func (s *Server) Listen(ctx context.Context) error {
	if s.Conn == nil {
		return errors.New("natshttp: Server.Conn cannot be nil")
	}

	if s.Subject == "" {
		return errors.New("natshttp: Server.Subject cannot be empty")
	}

	if s.Handler == nil {
		return errors.New("natshttp: Server.Handler cannot be nil")
	}

	if s.ErrorHandler == nil {
		s.ErrorHandler = NoOpErrorHandler
	}

	if s.PendingMsgsLimit == 0 {
		s.PendingMsgsLimit = nats.DefaultSubPendingMsgsLimit
	}

	if s.PendingBytesLimit == 0 {
		// default to 10 Gb of pending bytes as we may be handling a lot of uploads in high load scenarios
		s.PendingBytesLimit = 1024 * 1024 * 1024
	}

	var err error
	var sub *nats.Subscription

	if s.Group == "" {
		sub, err = s.Conn.SubscribeSync(s.Subject)
	} else {
		sub, err = s.Conn.QueueSubscribeSync(s.Subject, s.Group)
	}

	// increase pending limits on the subscription to prevent slow consumer detection in high load scenarios
	if err = sub.SetPendingLimits(s.PendingMsgsLimit, s.PendingBytesLimit); err != nil {
		return err
	}

	if err != nil {
		return err
	}

	if s.maxMsgSize == 0 {
		s.maxMsgSize = int(s.Conn.MaxPayload())
	}

	for {
		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			return err
		}

		go func() {
			if err := s.onMsg(msg); err != nil {
				s.ErrorHandler(err)
			}
		}()
	}
}

func (s *Server) onMsg(msg *nats.Msg) error {
	req := http.Request{}

	if err := MsgToHttpRequest(msg, &req, s.Conn, s.maxMsgSize); err != nil {
		return err
	}

	writer, err := NewResponseWriter(s.Conn, msg.Reply)
	if err != nil {
		return err
	}

	s.Handler.ServeHTTP(writer, &req)

	return writer.Close()
}

func MsgToHttpRequest(msg *nats.Msg, req *http.Request, conn *nats.Conn, maxMsgSize int) error {
	req.Proto = "HTTP/1.1"
	req.Method = msg.Header.Get(HeaderMethod)

	URL, err := url.Parse(msg.Header.Get(HeaderUrl))
	if err != nil {
		return err
	}

	req.URL = URL

	// copy headers
	req.Header = make(http.Header)
	h := req.Header

	for key, values := range msg.Header {
		for _, value := range values {
			h.Add(key, value)
		}
	}

	// determine transfer encoding
	req.TransferEncoding = h.Values(headers.TransferEncoding)
	if h.Get(headers.TransferEncoding) == "chunked" {
		h.Del(headers.ContentLength)
	}

	// determine content length
	clHeader := msg.Header.Get(headers.ContentLength)
	if clHeader == "" {
		req.ContentLength = -1
	} else {
		contentLength, err := strconv.ParseInt(clHeader, 10, 64)
		if err != nil {
			return err
		}
		req.ContentLength = contentLength
	}

	// determine if the request is chunked or not
	chunked, err := IsChunkedRequest(msg, maxMsgSize)
	if err != nil {
		return err
	}

	// if not chunked set the body and return
	if !chunked {
		req.Body = io.NopCloser(bytes.NewReader(msg.Data))
		return nil
	}

	// otherwise we generate a unique inbox for this chunked transfer and send a message to the sender with the
	// inbox for subsequent messages
	chunkedInbox := conn.NewInbox()

	sub, err := conn.SubscribeSync(chunkedInbox)
	if err != nil {
		return err
	}

	setupMsg := nats.NewMsg(msg.Reply)
	setupMsg.Reply = chunkedInbox

	if err = conn.PublishMsg(setupMsg); err != nil {
		return err
	}

	req.Body, err = NewChunkReader(msg, sub, req.Context())
	if err != nil {
		return err
	}

	return nil
}
