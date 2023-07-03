package natshttp

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/juju/errors"
)

const (
	HeaderPath     = "X-Path"
	HeaderQuery    = "X-Query"
	HeaderFragment = "X-Fragment"
)

type Result[T any] struct {
	Value T
	Error error
}

func ReqToMsg(req *http.Request, msg *nats.Msg) error {
	URL := req.URL
	if URL.Scheme != UrlScheme {
		return errors.Errorf("natshttp: url scheme must be '%s'", UrlScheme)
	}

	h := msg.Header

	// we can't reliably transform the subject back into the PATH as there could be paths like /foo.zst
	// we add it explicitly as a header to avoid this problem
	h.Set(HeaderPath, URL.Path)
	h.Set(HeaderQuery, URL.RawQuery)
	h.Set(HeaderFragment, URL.RawFragment)

	path := strings.ReplaceAll(URL.Path, "/", ".")
	if len(path) == 1 {
		path = ""
	}

	// <host>.<path>.<method>
	msg.Subject = fmt.Sprintf("%s%s.%s", URL.Host, path, req.Method)

	return nil
}

func MsgToRequest(prefix string, msg *nats.Msg, req *http.Request) error {
	subject := msg.Subject

	if subject[:len(prefix)] != prefix {
		return errors.Errorf("subject '%s' doesn't begin with prefix '%s'", subject, prefix)
	}
	components := strings.Split(subject[len(prefix):], ".")

	// last component of the subject is the Http Method
	method := components[len(components)-1]
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodConnect, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		req.Method = method
	default:
		return errors.Errorf("natshttp: invalid http method '%s' in subject '%s'", method, subject)
	}

	req.Proto = "HTTP/1.1"

	h := msg.Header

	// join all but the last component
	req.URL = &url.URL{
		Scheme:      UrlScheme,
		Host:        prefix,
		Path:        h.Get(HeaderPath),
		RawQuery:    h.Get(HeaderQuery),
		RawFragment: h.Get(HeaderFragment),
	}

	return nil
}
