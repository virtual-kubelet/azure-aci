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
	// assemble
	goOmegaInstance := NewWithT(t)

	var errWithNoMessage = Error{
		StatusCode: rand.Int(),
		Code:       RandStr(),
		Body:       RandStr(),
		Header:     http.Header{"": make([]string, 1)},
		URL:        RandStr(),
	}

	// act
	err := errWithNoMessage.Error()

	// assert
	goOmegaInstance.Expect(err).To(BeAssignableToTypeOf(""))
}

func TestErrorWithMessage(t *testing.T) {
	// assemble
	goOmegaInstance := NewWithT(t)

	randomStatusCode := rand.Int()

	var errWithNoMessage = Error{
		StatusCode: randomStatusCode,
		Code:       RandStr(),
		Message:    RandStr(),
		Body:       RandStr(),
		Header:     http.Header{"": make([]string, 1)},
		URL:        RandStr(),
	}

	// act
	err := errWithNoMessage.Error()

	// assert
	goOmegaInstance.Expect(err).To(BeAssignableToTypeOf(""))
}

func TestCheckResponseWithStatusCode2XX(t *testing.T) {
	// assemble
	goOmegaInstance := NewWithT(t)
	const MinValidStatusCode = 200
	const MaxValidStatusCode = 299

	randomStatusCode2xx := RandBetween(MinValidStatusCode, MaxValidStatusCode)

	var res = &http.Response{StatusCode: randomStatusCode2xx}

	// act
	err := CheckResponse(res)

	// assert
	goOmegaInstance.Expect(err).To(BeNil())
}

func TestCheckResponseIOReaderFails(t *testing.T) {
	// assemble
	goOmegaInstance := NewWithT(t)
	const MaxValidStatusCode = 299

	randomResHeader := http.Header{}
	randomResStatusCodeNot2xx := MaxValidStatusCode + rand.Int()
	randomResStatus := RandStr()

	var res = &http.Response{
		Header:     randomResHeader,
		Status:     randomResStatus,
		StatusCode: randomResStatusCodeNot2xx,
	}
	ioReadAll = func(_ io.Reader) ([]byte, error) {
		return nil, &Error{}
	}

	// act
	err := CheckResponse(res)

	// assert
	goOmegaInstance.Expect(err).ToNot(BeNil())

	goOmegaInstance.Expect(err.(*Error).Body).To(Equal(randomResStatus))
	goOmegaInstance.Expect(err.(*Error).Header).To(Equal(randomResHeader))
	goOmegaInstance.Expect(err.(*Error).StatusCode).To(Equal(randomResStatusCodeNot2xx))
}

func TestCheckResponseJsonUnmarshallFails(t *testing.T) {
	// assemble
	goOmegaInstance := NewWithT(t)
	const MaxValidStatusCode = 299

	randomResHeader := http.Header{}
	randomResRequest := &http.Request{URL: &url.URL{}}
	randomResStatus := RandStr()
	randomResStatusCodeNot2xx := MaxValidStatusCode + rand.Int()

	var res = &http.Response{
		Header:     randomResHeader,
		Request:    randomResRequest,
		Status:     randomResStatus,
		StatusCode: randomResStatusCodeNot2xx,
	}
	ioReadAll = func(_ io.Reader) ([]byte, error) {
		return make([]byte, 1), nil
	}
	jsonUnmarshall = func(_ []byte, v interface{}) error {
		return &Error{}
	}

	// act
	err := CheckResponse(res)

	// assert
	goOmegaInstance.Expect(err).ToNot(BeNil())

	goOmegaInstance.Expect(err.(*Error).Body).To(Equal(randomResStatus))
	goOmegaInstance.Expect(err.(*Error).Header).To(Equal(randomResHeader))
	goOmegaInstance.Expect(err.(*Error).StatusCode).To(Equal(randomResStatusCodeNot2xx))
}

func TestCheckResponseJsonUnmarshallSuccessStatusCode0(t *testing.T) {
	// assemble
	goOmegaInstance := NewWithT(t)
	const MaxValidStatusCode = 299

	jsonUnMarshallStatusCode0 := 0
	randomErrorBody := RandStr()
	randomErrorCode := RandStr()
	randomErrorMessage := RandStr()
	randomErrorUrl := RandStr()
	randomResBody := make([]byte, 1)
	randomRequestUrl := url.URL{}
	randomStatusCodeNot2xx := MaxValidStatusCode + rand.Int()

	var res = &http.Response{
		StatusCode: randomStatusCodeNot2xx,
		Request:    &http.Request{URL: &randomRequestUrl},
	}
	ioReadAll = func(_ io.Reader) ([]byte, error) {
		return randomResBody, nil
	}
	jsonUnmarshall = func(_ []byte, v interface{}) error {
		*v.(*errorReply) = errorReply{
			Error: &Error{
				Body:       randomErrorBody,
				Code:       randomErrorCode,
				Header:     http.Header{},
				Message:    randomErrorMessage,
				StatusCode: jsonUnMarshallStatusCode0,
				URL:        randomErrorUrl,
			},
		}
		return nil
	}

	// act
	err := CheckResponse(res)

	// assert
	goOmegaInstance.Expect(err).ToNot(BeNil())

	goOmegaInstance.Expect(err.(*Error).Body).To(Equal(string(randomResBody)))
	goOmegaInstance.Expect(err.(*Error).StatusCode).To(Equal(randomStatusCodeNot2xx))
	goOmegaInstance.Expect(err.(*Error).URL).To(Equal(randomRequestUrl.String()))
}

func TestCheckResponseJsonUnmarshallSuccessStatusCodeNot0(t *testing.T) {
	// assemble
	goOmegaInstance := NewWithT(t)
	const MaxValidStatusCode = 299

	randomErrorBody := RandStr()
	randomErrorCode := RandStr()
	randomErrorMessage := RandStr()
	randomErrorUrl := RandStr()
	randomJsonUnMarshallStatusCodeNot0 := rand.Int()
	randomStatusCodeNot2xx := MaxValidStatusCode + rand.Int()
	randomResBody := make([]byte, 1)
	randomRequestUrl := url.URL{}

	var res = &http.Response{
		StatusCode: randomStatusCodeNot2xx,
		Request:    &http.Request{URL: &randomRequestUrl},
	}

	ioReadAll = func(_ io.Reader) ([]byte, error) {
		return randomResBody, nil
	}
	jsonUnmarshall = func(_ []byte, v interface{}) error {
		*v.(*errorReply) = errorReply{
			Error: &Error{
				Body:       randomErrorBody,
				Code:       randomErrorCode,
				Header:     http.Header{},
				Message:    randomErrorMessage,
				StatusCode: randomJsonUnMarshallStatusCodeNot0,
				URL:        randomErrorUrl,
			},
		}
		return nil
	}

	// act
	err := CheckResponse(res)

	// assert
	goOmegaInstance.Expect(err).ToNot(BeNil())

	goOmegaInstance.Expect(err.(*Error).Body).To(Equal(string(randomResBody)))
	goOmegaInstance.Expect(err.(*Error).StatusCode).To(Equal(randomJsonUnMarshallStatusCodeNot0))
	goOmegaInstance.Expect(err.(*Error).URL).To(Equal(randomRequestUrl.String()))
}
