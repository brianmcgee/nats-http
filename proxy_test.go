package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-http-utils/headers"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
)

func runProxy(t *testing.T, router chi.Router, listener net.Listener, conn *nats.Conn, group string, ctx context.Context) {
	t.Helper()

	srv := Server{
		Conn:    conn,
		Subject: subject,
		Group:   group,
		Handler: router,
	}

	go func() {
		_ = srv.Listen(ctx)
	}()

	proxy := Proxy{
		Subject: subject,
		Transport: &Transport{
			Conn: conn,
		},
		Listener: listener,
	}

	go func() {
		_ = proxy.Listen(ctx)
	}()
}

func TestProxy_Head(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	routes := chi.NewRouter()
	routes.Head("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.Nil(t, err)

	runProxy(t, routes, listener, conn, "", ctx)

	baseUrl := fmt.Sprintf("http://%s/", listener.Addr().String())
	resp, err := http.Head(baseUrl)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, fmt.Sprintf("%d %s", http.StatusNoContent, http.StatusText(http.StatusNoContent)), resp.Status)

	b, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(b))
}

func TestProxy_Simple(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body := "Hello World"

	routes := chi.NewRouter()
	handler := func(w http.ResponseWriter, req *http.Request) {
		var err error

		switch req.Method {
		case http.MethodGet:
			_, err = io.WriteString(w, body)
		case http.MethodPost, http.MethodPut:
			_, err = io.Copy(w, req.Body)
		default:
			// do nothing
		}

		assert.Nil(t, err, "echo handler failed to copy request body into response writer")
	}

	routes.Get("/", handler)
	routes.Post("/", handler)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.Nil(t, err)

	runProxy(t, routes, listener, conn, "", ctx)

	baseUrl := fmt.Sprintf("http://%s/", listener.Addr().String())

	t.Run("GET", func(t *testing.T) {
		// Get
		resp, err := http.Get(baseUrl)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)), resp.Status)
		assert.Equal(t, len(body), int(resp.ContentLength))

		b, err := io.ReadAll(io.NopCloser(resp.Body))
		assert.Nil(t, err)
		assert.Equal(t, body, string(b))
	})

	t.Run("POST", func(t *testing.T) {
		// Get
		body = "Ping Pong"

		bodyReader := io.NopCloser(bytes.NewReader([]byte(body)))
		resp, err := http.Post(baseUrl, "text/plain", bodyReader)

		assert.Nil(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)), resp.Status)
		assert.Equal(t, len(body), int(resp.ContentLength))

		b, err := io.ReadAll(io.NopCloser(resp.Body))
		assert.Nil(t, err)
		assert.Equal(t, body, string(b))
	})
}

func TestProxy_Chunked(t *testing.T) {
	s := runBasicNatsServer(t)
	defer shutdownNatsServer(t, s)
	conn := client(t, s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	routes := chi.NewRouter()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.Nil(t, err)

	runProxy(t, routes, listener, conn, "", ctx)
	baseUrl := fmt.Sprintf("http://%s", listener.Addr().String())

	t.Run("GET", func(t *testing.T) {
		body := make([]byte, conn.MaxPayload()*2)

		n, err := rand.Read(body)
		assert.Nil(t, err)
		assert.Equal(t, len(body), n)

		routes.Get("/auto", func(w http.ResponseWriter, r *http.Request) {
			_, err = io.Copy(w, bytes.NewReader(body))
			assert.Nil(t, err)
		})

		routes.Get("/chunked", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Transfer-Encoding", "chunked")
			_, err = io.Copy(w, bytes.NewReader(body))
			assert.Nil(t, err)
		})

		routes.Get("/content-length", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(headers.ContentLength, strconv.Itoa(len(body)))
			_, err = io.Copy(w, bytes.NewReader(body))
			assert.Nil(t, err)
		})

		paths := []string{"auto", "chunked", "content-length"}

		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				resp, err := http.Get(baseUrl + "/" + path)
				assert.Nil(t, err)

				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)), resp.Status)

				switch path {
				case "auto":
					assert.Equal(t, int64(-1), resp.ContentLength)
					assert.Equal(t, []string{"chunked"}, resp.TransferEncoding)
				case "chunked":
					assert.Equal(t, int64(-1), resp.ContentLength)
					assert.Equal(t, []string{"chunked"}, resp.TransferEncoding)
				case "set-length":
					assert.Equal(t, len(body), resp.ContentLength)
					assert.Nil(t, resp.TransferEncoding)
				}

				b, err := io.ReadAll(resp.Body)
				assert.Nil(t, err)
				assert.Equal(t, len(body), len(b))
				assert.Equal(t, body, b)
			})
		}
	})

	t.Run("POST", func(t *testing.T) {
		body := make([]byte, conn.MaxPayload()*10)
		n, err := rand.Read(body)

		routes.Post("/", func(w http.ResponseWriter, r *http.Request) {
			reqBody, err := io.ReadAll(r.Body)
			assert.Nil(t, err)

			for idx := 0; idx < len(reqBody); idx++ {
				if body[idx] != reqBody[idx] {
					println()
				}
			}

			assert.Equal(t, len(reqBody), len(body))
			assert.Equal(t, reqBody, body)

			// echo back what we received
			n, err := w.Write(reqBody)
			assert.Nil(t, err)
			assert.Equal(t, len(reqBody), n)
		})

		assert.Nil(t, err)
		assert.Equal(t, len(body), n)

		bodyReader := io.NopCloser(bytes.NewReader(body))
		resp, err := http.Post(baseUrl, "", bodyReader)
		assert.Nil(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)), resp.Status)

		b, err := io.ReadAll(resp.Body)
		assert.Nil(t, err)
		assert.Equal(t, len(body), len(b))
		assert.Equal(t, body, b)
	})
}
