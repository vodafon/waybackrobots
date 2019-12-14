package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
)

// RoundTripFunc .
type roundTripFunc func(req *http.Request) *http.Response

// RoundTrip .
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestList(t *testing.T) {
	tests := []struct {
		p string
		l int
	}{
		{"./testdata/list2.json", 2},
		{"./testdata/list0.json", 0},
	}
	for i, tt := range tests {
		cl := client{newTestClient(readFile(tt.p))}

		list := listSnapshots(cl)
		if len(list) != tt.l {
			t.Errorf("%d: Incorrect result. Expected %q, got %q\n", i, tt.l, len(list))
		}
		if tt.l == 0 {
			continue
		}
		exp := [][2]string{
			{"20070702231826", "http://example.com/robots.txt"},
			{"20070706061934", "http://www.example.com/robots.txt"},
		}
		if fmt.Sprint(exp) != fmt.Sprint(list) {
			t.Errorf("%d: Incorrect result. Expected %q, got %q\n", i, exp, list)
		}
	}
}

func TestWorkerDo(t *testing.T) {
	tests := []struct {
		name string
		p    string
		exp  string
	}{
		{"Only unique", "./testdata/robots1.txt", "/a\n/b/c\n/e\n"},
		{"Lower case", "./testdata/robots2.txt", "/a\n/b/c\n/e\n"},
		{"Comments and missing colon", "./testdata/robots2.txt", "/a\n/b/c\n/e\n"},
	}
	for _, tt := range tests {
		cl := client{newTestClient(readFile(tt.p))}

		wg := &sync.WaitGroup{}
		rowC := make(chan [2]string, 1)

		w := bytes.NewBuffer([]byte{})
		uniq := &Uniq{
			mp: make(map[string]struct{}),
			w:  w,
		}

		go Worker{
			wg:   wg,
			rowC: rowC,
			um:   uniq,
			cl:   cl,
		}.Do()

		wg.Add(1)

		rowC <- [2]string{"20070702231826", "http://example.com/robots.txt"}
		close(rowC)
		wg.Wait()
		res := w.String()
		if tt.exp != res {
			t.Errorf("%s: Incorrect result. Expected %q, got %q\n", tt.name, tt.exp, res)
		}
	}
}

func readFile(p string) []byte {
	data, err := ioutil.ReadFile(p)
	if err != nil {
		panic(err)
	}
	return data
}

func newTestClient(data []byte) *http.Client {
	return &http.Client{
		Transport: newRoundTrip(data),
	}
}

func newRoundTrip(data []byte) roundTripFunc {
	return func(req *http.Request) *http.Response {
		// Test request parameters
		return &http.Response{
			StatusCode: 200,
			// Send response to be tested
			Body: ioutil.NopCloser(bytes.NewBuffer(data)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	}
}
