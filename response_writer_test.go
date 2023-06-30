package http

import (
	"bytes"
	cryptoRand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/stretchr/testify/assert"
)

const (
	subject = "foo.bar"
)

func TestNewResponseWriter(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	writer, err := NewResponseWriter(nil, subject)
	assert.Nil(t, writer)
	assert.EqualError(t, err, "conn cannot be nil")

	writer, err = NewResponseWriter(conn, "")
	assert.Nil(t, writer)
	assert.EqualError(t, err, "subject cannot be empty")

	writer, err = NewResponseWriter(conn, subject)
	assert.Nil(t, err)
	assert.NotNil(t, writer)
}

func TestResponseWriter_WriteSmallBody(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	subjectPrefix := "test.smallBody"

	sizes := rand.Perm(SmallBodySize)

	// ensure extremes are included
	sizes = append(sizes, 0)
	sizes = append(sizes, SmallBodySize)

	for _, size := range sizes {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			body := make([]byte, size)
			n, err := cryptoRand.Read(body)
			assert.Nil(t, err)
			assert.Equal(t, size, n)

			subject := fmt.Sprintf("%s.%d", subjectPrefix, size)

			msgs := make(chan *nats.Msg, 1)
			_, err = conn.ChanSubscribe(subject, msgs)
			assert.Nil(t, err)

			w, err := NewResponseWriter(conn, subject)
			assert.Nil(t, err)

			n, err = w.Write(body)
			assert.Equal(t, len(body), n)

			assert.Nil(t, w.Close())

			msg, ok := <-msgs
			assert.True(t, ok)
			assert.NotNil(t, msg)

			// check headers
			h := msg.Header

			assert.Equal(t, h.Get(HeaderStatus), http.StatusText(http.StatusOK))
			assert.Equal(t, h.Get(HeaderStatusCode), strconv.Itoa(http.StatusOK))

			expectedContentLength := ""
			if len(body) > 0 {
				expectedContentLength = strconv.Itoa(len(body))
			}

			assert.Equal(t, expectedContentLength, msg.Header.Get("Content-Length"))

			data, err := io.ReadAll(bytes.NewReader(msg.Data))
			assert.Nil(t, err)
			assert.Equal(t, body, data)
		})
	}
}

func TestResponseWriter_WriteLargeBodyNoContentLength(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	subjectPrefix := strings.ReplaceAll(t.Name(), "_", ".")

	body := make([]byte, SmallBodySize+1)
	n, err := cryptoRand.Read(body)

	assert.Nil(t, err)
	assert.Equal(t, SmallBodySize+1, n)

	subject := fmt.Sprintf("%s.%d", subjectPrefix, len(body))

	msgs := make(chan *nats.Msg, 1)
	_, err = conn.ChanSubscribe(subject, msgs)
	assert.Nil(t, err)

	w, err := NewResponseWriter(conn, subject)
	assert.Nil(t, err)

	n, err = w.Write(body)
	assert.Equal(t, len(body), n)
	assert.Nil(t, w.Close())

	msg, ok := <-msgs
	assert.True(t, ok)
	assert.NotNil(t, msg)

	// check headers
	h := msg.Header

	assert.Equal(t, h.Get(HeaderStatus), http.StatusText(http.StatusOK))
	assert.Equal(t, h.Get(HeaderStatusCode), strconv.Itoa(http.StatusOK))
	assert.Equal(t, "chunked", h.Get("Transfer-Encoding"))
	assert.Empty(t, h.Get("Content-Length"))

	data, err := io.ReadAll(bytes.NewReader(msg.Data))
	assert.Nil(t, err)
	assert.Equal(t, body, data)
}

func TestResponseWriter_WriteLargeBodyWithContentLength(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	subjectPrefix := strings.ReplaceAll(t.Name(), "_", ".")

	body := make([]byte, SmallBodySize+1)
	n, err := cryptoRand.Read(body)

	assert.Nil(t, err)
	assert.Equal(t, SmallBodySize+1, n)

	subject := fmt.Sprintf("%s.%d", subjectPrefix, len(body))

	msgs := make(chan *nats.Msg, 2)
	_, err = conn.ChanSubscribe(subject, msgs)
	assert.Nil(t, err)

	w, err := NewResponseWriter(conn, subject)
	assert.Nil(t, err)

	w.Header().Set("Content-Length", strconv.Itoa(len(body)))

	n, err = w.Write(body)
	assert.Equal(t, len(body), n)

	assert.Nil(t, w.Close())

	msg, ok := <-msgs
	assert.True(t, ok)
	assert.NotNil(t, msg)

	// check headers
	h := msg.Header

	assert.Equal(t, h.Get(HeaderStatus), http.StatusText(http.StatusOK))
	assert.Equal(t, h.Get(HeaderStatusCode), strconv.Itoa(http.StatusOK))
	assert.Equal(t, h.Get("Content-Length"), strconv.Itoa(len(body)))
	assert.Empty(t, h.Get("Transfer-Encoding"))

	data, err := io.ReadAll(bytes.NewReader(msg.Data))
	assert.Nil(t, err)
	assert.Equal(t, body, data)
}
