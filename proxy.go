package http

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/juju/errors"
	"golang.org/x/sync/errgroup"
)

type Proxy struct {
	Subject   string
	Transport *Transport
	Listener  net.Listener
}

func (p *Proxy) Listen(ctx context.Context) error {
	if p.Subject == "" {
		return errors.New("natshttp: Proxy.Subject cannot be empty")
	}

	if p.Transport == nil {
		return errors.New("natshttp: Proxy.Transport cannot be empty")
	}

	if p.Listener == nil {
		return errors.New("natshttp: Proxy.Listener cannot be empty")
	}

	r := chi.NewRouter()
	r.Handle("/*", p)

	srv := http.Server{
		Handler: r,
	}

	eg := errgroup.Group{}
	eg.Go(func() error {
		<-ctx.Done()
		_ = srv.Close()
		_ = p.Listener.Close()
		return nil
	})

	eg.Go(func() error {
		err := srv.Serve(p.Listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		return err
	})

	return eg.Wait()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// todo better error handling

	proxyReq := &http.Request{
		URL: &url.URL{
			Host:     p.Subject,
			Scheme:   UrlScheme,
			Path:     req.URL.Path,
			RawQuery: req.URL.RawQuery,
		},
		Method: req.Method,
		Header: req.Header,
		Body:   req.Body,
	}

	proxyReq.ContentLength = req.ContentLength
	proxyReq.TransferEncoding = req.TransferEncoding

	resp, err := p.Transport.RoundTrip(proxyReq)
	if err != nil {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, err.Error())
		return
	}

	defer func() { _ = resp.Body.Close() }()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		panic(err)
	}
}
