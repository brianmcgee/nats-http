<h1 align="center"> 
  <br>
  NATS Http
  <br> 
</h1>

![Build](https://github.com/brianmcgee/nats.http/actions/workflows/ci.yaml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/brianmcgee/nats.http/badge.svg)](https://coveralls.io/github/brianmcgee/nats.http)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Status: experimental**

# Usage

The project is currently in a state of rapid iteration. I will update this section and add more documentation when it has reached a steady state.

## Proxy

```go
// connect to NATS
conn, err := nats.Connect(nats.DefaultURL)
if err != nil {
    panic(err)
}

// create a TCP listener
listener, err := net.Listen("tcp", "localhost:8080")
if err != nil {
    panic(err)
}

// create a proxy which forwards requests to 'test.service.>' subject hierarchy
proxy := Proxy{
    Subject: "test.service",
    Listener: listener,
    Transport: &Transport{
        Conn: conn,
    },
}

// create a context
ctx, cancel := context.WithCancel(context.Background())

eg := errgroup.Group{}

// start listening
eg.Go(func () error {
    return proxy.Listen(ctx)
})

// wait 10 seconds then cancel the context
eg.Go(func () error {
    <-time.After(10 * time.Second)
    cancel()
    return nil
})

// wait for the listener to complete
if err = eg.Wait(); err != nil {
    panic(err)
}

```
