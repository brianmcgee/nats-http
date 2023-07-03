package natshttp

import (
	"github.com/nats-io/nats.go"
	"io"
	"net/http"
)

func ExampleTransport_basic() {
	// connect to NATS
	conn, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}

	// create a client using the nats transport
	client := http.Client{
		Transport: &Transport{
			Conn: conn,
		},
	}

	// perform a get request against a NATS Http Server configured to listen on the 'foo.bar.>' subject hierarchy
	// it's important to use the 'httpn' url scheme

	resp, err := client.Get("httpn://foo.bar/hello/world")
	if err != nil {
		panic(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	println(string(body))
}
