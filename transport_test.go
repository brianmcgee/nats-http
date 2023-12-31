package natshttp

import (
	"io"
	"net/http"

	"github.com/nats-io/nats.go"
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
	// it's important to use the 'nats+http' url scheme

	resp, err := client.Get("nats+http://foo.bar/hello/world")
	if err != nil {
		panic(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	println(string(body))
}
