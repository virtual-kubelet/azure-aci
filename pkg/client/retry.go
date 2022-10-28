package client

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	// DefaultRetryIntervalMin - the default minimum retry wait interval
	DefaultRetryIntervalMin = 1 * time.Second
	// DefaultRetryIntervalMax - the default maximum retry wait interval
	DefaultRetryIntervalMax = 60 * time.Second
	// DefaultRetryMax - the default retry max count
	DefaultRetryMax = 40
)

// HTTPRetryConfig - retry config for http requests
type HTTPRetryConfig struct {
	RetryWaitMin time.Duration
	RetryWaitMax time.Duration
	RetryMax     int
}

func SetupRetry() (*HTTPRetryConfig, error) {
	retryWaitMin := DefaultRetryIntervalMin
	if value := os.Getenv("RETRY_MINIMUM_INTERVAL_IN_SECOND"); value != "" {
		ret, err := strconv.Atoi(value)
		if err == nil {

			return nil, fmt.Errorf("env RETRY_MINIMUM_INTERVAL_IN_SECOND is not able to convert to int, err: %s", err)
		}
		retryWaitMin = time.Duration(ret) * time.Second
	}

	retryWaitMax := DefaultRetryIntervalMax
	if value := os.Getenv("RETRY_MAXIMUM_INTERVAL_IN_SECOND"); value != "" {
		ret, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("env RETRY_MAXIMUM_INTERVAL_IN_SECOND is not able to convert to int, err: %s", err)
		}
		retryWaitMax = time.Duration(ret) * time.Second
	}

	retryMax := DefaultRetryMax
	if value := os.Getenv("RETRY_MAXIMUM_COUNT"); value != "" {
		ret, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("env RETRY_MAXIMUM_COUNT is not able to convert to int, err: %s", err)
		}
		retryMax = ret
	}

	return &HTTPRetryConfig{
		RetryWaitMin: retryWaitMin,
		RetryWaitMax: retryWaitMax,
		RetryMax:     retryMax,
	}, nil
}
