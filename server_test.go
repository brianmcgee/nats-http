package natshttp

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"time"
)

func ExampleServer_basic() {
	// connect to NATS
	conn, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}

	// create a router
	router := chi.NewRouter()
	router.Head("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "world")
	})

	// create a server
	srv := Server{
		Conn:    conn,
		Subject: "foo.bar",   // it will listen for requests on the 'foo.bar.>' subject hierarchy
		Group:   "my-server", // name of the queue group when subscribing, used for load balancing
		Handler: router,
	}

	// create a context and an error group for running processes
	ctx, cancel := context.WithCancel(context.Background())
	eg := errgroup.Group{}

	// start listening
	eg.Go(func() error {
		return srv.Listen(ctx)
	})

	// wait 10 seconds then cancel the context
	eg.Go(func() error {
		<-time.After(10 * time.Second)
		cancel()
		return nil
	})

	// wait for the listener to complete
	if err = eg.Wait(); err != nil {
		panic(err)
	}
}
