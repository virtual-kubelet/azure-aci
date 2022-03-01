package api

import (
	. "github.com/onsi/gomega"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"testing"
)

func TestErrorWithNoMessage(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var errWithNoMessage = Error{
		StatusCode: rand.Int(),
		Code:       "",
		Message:    "",
		Body:       "",
		Header:     http.Header{"": make([]string, 1)},
		URL:        "",
	}

	err := errWithNoMessage.Error()
	goOmegaInstance.Expect(err).To(BeAssignableToTypeOf(""))
}

func TestErrorWithMessage(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var errWithNoMessage = Error{
		StatusCode: rand.Int(),
		Code:       "",
		Message:    "rand_msg",
		Body:       "",
		Header:     http.Header{"": make([]string, 1)},
		URL:        "",
	}

	err := errWithNoMessage.Error()
	goOmegaInstance.Expect(err).To(BeAssignableToTypeOf(""))
}

func TestCheckResponseWithStatusCodeIs2XX(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var res = &http.Response{StatusCode: 290}

	err := CheckResponse(res, NewIoReaderUtils(), NewJsonUtils())
	goOmegaInstance.Expect(err).To(BeNil())
}

func TestCheckResponseWithStatusCodeIsNot2xxHappyPath(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var res = &http.Response{
		StatusCode: 100,
	}
	var fakeIOReader = IoReaderUtils{
		func(_ io.Reader) ([]byte, error) {
			return make([]byte, 1), nil
		},
	}
	var fakeJsonUtils = JsonUtils{
		unMarshall: func(_ []byte, _ interface{}) error {
			return nil
		}}

	err := CheckResponse(res, fakeIOReader, fakeJsonUtils)
	goOmegaInstance.Expect(err).ToNot(BeNil())
}

func TestCheckResponseWithStatusCodeIsNot2xxIOReaderFails(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var res = &http.Response{
		StatusCode: 100,
	}
	var fakeIOReader = IoReaderUtils{
		func(_ io.Reader) ([]byte, error) {
			return nil, &Error{}
		},
	}
	var fakeJsonUtils = JsonUtils{
		unMarshall: func(_ []byte, _ interface{}) error {
			return nil
		}}

	err := CheckResponse(res, fakeIOReader, fakeJsonUtils)
	goOmegaInstance.Expect(err).ToNot(BeNil())
}

func TestCheckResponseWithStatusCodeIsNot2xxErrorInJsonUnmarshallFails(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var res = &http.Response{
		StatusCode: 100,
		Request:    &http.Request{URL: &url.URL{}},
	}
	var fakeIOReader = IoReaderUtils{
		func(_ io.Reader) ([]byte, error) {
			return make([]byte, 1), nil
		},
	}
	var fakeJsonUtils = JsonUtils{
		unMarshall: func(_ []byte, v interface{}) error {
			return &Error{}
		}}

	err := CheckResponse(res, fakeIOReader, fakeJsonUtils)
	goOmegaInstance.Expect(err).ToNot(BeNil())
}

func TestCheckResponseWithStatusCodeIsNot2xxErrorInJsonUnmarshallParsing(t *testing.T) {
	goOmegaInstance := NewWithT(t)
	var res = &http.Response{
		StatusCode: 100,
		Request:    &http.Request{URL: &url.URL{}},
	}
	var fakeIOReader = IoReaderUtils{
		func(_ io.Reader) ([]byte, error) {
			return make([]byte, 1), nil
		},
	}
	var fakeJsonUtils = JsonUtils{
		unMarshall: func(_ []byte, v interface{}) error {
			*v.(*errorReply) = errorReply{
				Error: &Error{
					StatusCode: rand.Int(),
					Code:       "",
					Message:    "",
					Body:       "",
					Header:     http.Header{},
					URL:        "",
				},
			}
			return nil
		}}

	err := CheckResponse(res, fakeIOReader, fakeJsonUtils)
	goOmegaInstance.Expect(err).ToNot(BeNil())
}
