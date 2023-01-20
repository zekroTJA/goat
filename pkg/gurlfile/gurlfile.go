package gurlfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	sectionNameSetup        = "setup"
	sectionNameSetupEach    = "setup-each"
	sectionnameTests        = "tests"
	sectionNameTeardown     = "teardown"
	sectionNameTeardownEach = "teardown-each"
)

const (
	optionNameQueryParams = "queryparams"
)

// Gurlfile holds all sections and
// their requests.
type Gurlfile struct {
	Imports []string

	Setup        []Request
	SetupEach    []Request
	Tests        []Request
	Teardown     []Request
	TeardownEach []Request
}

// Merge appends all requests in all sections of with
// to the current Gurlfile.
func (t *Gurlfile) Merge(with Gurlfile) {
	t.Setup = append(t.Setup, with.Setup...)
	t.SetupEach = append(t.SetupEach, with.SetupEach...)
	t.Tests = append(t.Tests, with.Tests...)
	t.Teardown = append(t.Teardown, with.Teardown...)
	t.TeardownEach = append(t.TeardownEach, with.TeardownEach...)
}

func (t Gurlfile) String() string {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Sprintf("error: %s", err.Error())
	}
	return string(b)
}

// Options holds the specific request
// options.
type Options struct {
	QueryParams map[string]any
}

// Request holds the specifications
// for a HTTP request with options
// and script commands.
type Request struct {
	Options

	Method string
	URI    string
	Header http.Header
	Body   []byte
	Script string

	parsed bool
}

func newRequest() (r Request) {
	r.Header = http.Header{}
	return r
}

// ParseWithParams takes the given parameters
// and replaces placeholders within the request
// with values from the given params.
//
// Returns the new parsed request.
func (t Request) ParseWithParams(params any) (Request, error) {
	if t.parsed {
		return Request{}, ErrAlreadyParsed
	}

	var err error

	t.URI, err = applyTemplate(t.URI, params)
	if err != nil {
		return Request{}, err
	}

	for _, vals := range t.Header {
		for i, v := range vals {
			vals[i], err = applyTemplate(v, params)
			if err != nil {
				return Request{}, err
			}
		}
	}

	bodyStr, err := applyTemplate(string(t.Body), params)
	if err != nil {
		return Request{}, err
	}
	t.Body = []byte(bodyStr)

	t.Script, err = applyTemplate(t.Script, params)
	if err != nil {
		return Request{}, err
	}

	for k, v := range t.QueryParams {
		switch vt := v.(type) {
		case string:
			t.QueryParams[k], err = applyTemplate(vt, params)
		case []any:
			err = applyTemplateToArray(vt, params)
		default:
			continue
		}

		if err != nil {
			return Request{}, err
		}
	}

	return t, nil
}

func (t Request) ToHttpRequest() (*http.Request, error) {
	uri, err := url.Parse(t.URI)
	if err != nil {
		return nil, fmt.Errorf("failed parsing URI: %s", err.Error())
	}

	for key, val := range t.Options.QueryParams {
		if arr, ok := val.([]any); ok {
			for _, v := range arr {
				uri.Query().Add(key, toString(v))
			}
		} else {
			uri.Query().Add(key, toString(val))
		}
	}

	var body io.Reader

	if len(t.Body) > 0 {
		body = bytes.NewBuffer(t.Body)
	}

	req, err := http.NewRequest(t.Method, uri.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header = t.Header

	return req, nil
}

func (t Request) String() string {
	return fmt.Sprintf("%s %s", t.Method, t.URI)
}

func toString(v any) string {
	return fmt.Sprintf("%v", v)
}
