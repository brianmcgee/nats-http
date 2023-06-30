package http

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/nats-io/nats.go"
)

type ChunkReader struct {
	ctx context.Context
	sub *nats.Subscription

	firstMsg      *nats.Msg
	remainingMsgs <-chan *nats.Msg

	idx    int
	reader io.Reader
}

func NewChunkReader(
	firstMsg *nats.Msg,
	sub *nats.Subscription,
	ctx context.Context,
) (*ChunkReader, error) {
	if sub == nil {
		return nil, errors.New("sub cannot be nil")
	}

	if firstMsg == nil {
		return nil, errors.New("firstMsg cannot be nil")
	}

	if ctx == nil {
		// default
		ctx = context.Background()
	}

	return &ChunkReader{
		ctx:      ctx,
		sub:      sub,
		firstMsg: firstMsg,
	}, nil
}

func (c *ChunkReader) Read(p []byte) (n int, err error) {
	if c.reader == nil {

		// determine next msg
		var msg *nats.Msg
		if c.idx == 0 {
			msg = c.firstMsg
		} else {
			msg, err = c.sub.NextMsgWithContext(c.ctx)
			if err != nil {
				return
			}
		}

		// empty data indicates the end of the chunk stream
		if len(msg.Data) == 0 {
			return 0, io.EOF
		}

		// otherwise create a new reader for the next chunk
		c.reader = bytes.NewReader(msg.Data)
		c.idx += 1
	}

	// read from the current chunk
	n, err = c.reader.Read(p)

	if err == io.EOF {
		// we have exhausted the current chunk
		// clear state for next read
		err = nil
		c.reader = nil
	}

	return
}

func (c *ChunkReader) Close() error {
	return c.sub.Unsubscribe()
}
