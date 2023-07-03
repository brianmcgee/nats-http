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

The project is currently in a state of rapid iteration. I will update this section and add more documentation as features
mature and reach a steady state.

## Client

```go
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
// it's important to use the 'http+nats' url scheme

resp, err := client.Get("http+nats://foo.bar/hello/world")
if err != nil {
    panic(err)
}

body, err := io.ReadAll(resp.Body)
if err != nil {
    panic(err)
}

println(string(body))
```

## Server

```go
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
```

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

// create a context and an error group for running processes
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
