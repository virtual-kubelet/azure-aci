package azure

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestRetryPolicy(t *testing.T) {
	type tcase struct {
		resp   http.Response
		err    error
		expect bool
	}

	cases := []tcase{
		{
			resp: http.Response{},
			err: &url.Error{
				Err: fmt.Errorf("connection refused"),
			},

			expect: true,
		},
		{
			resp: http.Response{},
			err: &url.Error{
				Err: fmt.Errorf("stopped after 3 redirects"),
			},
			expect: false,
		},
		{
			resp: http.Response{
				StatusCode: 200,
			},
			err:    nil,
			expect: false,
		},
		{
			resp: http.Response{
				StatusCode: 429,
			},
			err:    nil,
			expect: true,
		},
		{
			resp: http.Response{
				StatusCode: 500,
			},
			err:    nil,
			expect: true,
		},
		{
			resp: http.Response{
				StatusCode: 501,
			},
			err:    nil,
			expect: false,
		},
	}
	for _, tc := range cases {
		if v, _ := retryPolicy(&tc.resp, tc.err); v != tc.expect {
			t.Fatalf("bad: %#v-> %t", tc, v)
		}
	}
}

func TestBackoff(t *testing.T) {
	type tcase struct {
		min    time.Duration
		max    time.Duration
		i      int
		expect time.Duration
	}
	cases := []tcase{
		{
			time.Second,
			5 * time.Minute,
			0,
			time.Second,
		},
		{
			time.Second,
			5 * time.Minute,
			1,
			2 * time.Second,
		},
		{
			time.Second,
			5 * time.Minute,
			2,
			4 * time.Second,
		},
		{
			time.Second,
			5 * time.Minute,
			3,
			8 * time.Second,
		},
		{
			time.Second,
			5 * time.Minute,
			63,
			5 * time.Minute,
		},
		{
			time.Second,
			5 * time.Minute,
			128,
			5 * time.Minute,
		},
	}

	for _, tc := range cases {
		if v := backoff(tc.min, tc.max, tc.i, nil); v != tc.expect {
			t.Fatalf("bad: %#v -> %s", tc, v)
		}
	}
}
