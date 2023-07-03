package natshttp

import (
	"context"
	"net"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
)

func runBasicNatsServer(t *testing.T) *server.Server {
	t.Helper()
	opts := test.DefaultTestOptions
	opts.Port = -1
	return test.RunServer(&opts)
}

func shutdownNatsServer(t *testing.T, s *server.Server) {
	t.Helper()
	s.Shutdown()
	s.WaitForShutdown()
}

func client(t *testing.T, s *server.Server, opts ...nats.Option) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(s.ClientURL(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	return nc
}

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
