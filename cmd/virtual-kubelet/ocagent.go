package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"contrib.go.opencensus.io/exporter/ocagent"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"github.com/virtual-kubelet/virtual-kubelet/trace/opencensus"
	octrace "go.opencensus.io/trace"
)

func initOCAgent(service string) error {
	endpoint := os.Getenv("OCAGENT_ENDPOINT")
	if endpoint == "" {
		return nil
	}
	options := []ocagent.ExporterOption{
		ocagent.WithAddress(endpoint),
		ocagent.WithServiceName(service),
	}

	switch os.Getenv("OCAGENT_INSECURE") {
	case "0", "no", "n", "off", "":
	case "1", "yes", "y", "on":
		options = append(options, ocagent.WithInsecure())
	default:
		return errdefs.InvalidInput("invalid value for OCAGENT_INSECURE")
	}

	exporter, err := ocagent.NewExporter(options...)
	if err != nil {
		return err
	}

	octrace.RegisterExporter(exporter)
	return nil
}

func configureTracing(service string, rate string) error {
	var s octrace.Sampler
	switch strings.ToLower(rate) {
	case "":
	case "always":
		s = octrace.AlwaysSample()
	case "never":
		s = octrace.NeverSample()
	default:
		rate, err := strconv.Atoi(rate)
		if err != nil {
			return errdefs.AsInvalidInput(fmt.Errorf("unsupported trace sample rate: %w", err))
		}
		if rate < 0 || rate > 100 {
			return errdefs.AsInvalidInput(fmt.Errorf("trace sample rate must be between 0 and 100: %w", err))
		}
		s = octrace.ProbabilitySampler(float64(rate) / 100)
	}

	if err := initOCAgent(service); err != nil {
		return err
	}
	trace.T = opencensus.Adapter{}
	octrace.ApplyConfig(octrace.Config{
		DefaultSampler: s,
	})
	return nil
}
