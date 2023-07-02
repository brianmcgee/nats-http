package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
)

type Result[T any] struct {
	Value T
	Error error
}

func ReqToSubject(req *http.Request) (string, error) {
	URL := req.URL
	if URL.Scheme != UrlScheme {
		return "", errors.Errorf("natshttp: url scheme must be '%s'", UrlScheme)
	}

	path := strings.ReplaceAll(URL.Path, "/", ".")
	if len(path) == 1 {
		path = ""
	}

	// <host>.<path>.<method>
	return fmt.Sprintf("%s%s.$%s", URL.Host, path, req.Method), nil
}
