package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCounterGaugeAndHistogramRender(t *testing.T) {
	registry := NewRegistry()
	counter := registry.Counter("test_requests_total", "Requests.", "method", "status")
	counter.With("GET", "200").Inc()
	counter.With("GET", "200").Add(2)
	counter.With("POST", "500").Inc()

	gauge := registry.Gauge("test_connections", "Connections.")
	gauge.With().Set(3)
	gauge.With().Inc()

	histogram := registry.Histogram("test_duration_seconds", "Duration.", "route")
	histogram.With("/x").Observe(0.01)
	histogram.With("/x").Observe(2)

	var output bytes.Buffer
	if _, err := registry.WriteTo(&output); err != nil {
		t.Fatalf("WriteTo() = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		`# TYPE test_requests_total counter`,
		`test_requests_total{method="GET",status="200"} 3`,
		`test_requests_total{method="POST",status="500"} 1`,
		`# TYPE test_connections gauge`,
		`test_connections 4`,
		`# TYPE test_duration_seconds histogram`,
		`test_duration_seconds_bucket{route="/x",le="0.005"} 0`,
		`test_duration_seconds_bucket{route="/x",le="0.01"} 1`,
		`test_duration_seconds_bucket{route="/x",le="2.5"} 2`,
		`test_duration_seconds_bucket{route="/x",le="+Inf"} 2`,
		`test_duration_seconds_count{route="/x"} 2`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, text)
		}
	}
}

func TestConcurrentObservations(t *testing.T) {
	registry := NewRegistry()
	counter := registry.Counter("test_total", "Total.", "label")
	histogram := registry.Histogram("test_seconds", "Seconds.", "label")

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.With("v").Inc()
			histogram.With("v").Observe(0.1)
		}()
	}
	wg.Wait()

	var output bytes.Buffer
	if _, err := registry.WriteTo(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `test_total{label="v"} 64`) {
		t.Fatalf("counter was not updated atomically:\n%s", output.String())
	}
	if !strings.Contains(output.String(), `test_seconds_count{label="v"} 64`) {
		t.Fatalf("histogram was not updated atomically:\n%s", output.String())
	}
}

func TestEscapesLabelValues(t *testing.T) {
	registry := NewRegistry()
	registry.Counter("test_total", "Total.", "path").With(`a"b\c`).Inc()
	var output bytes.Buffer
	if _, err := registry.WriteTo(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `path="a\"b\\c"`) {
		t.Fatalf("label was not escaped:\n%s", output.String())
	}
}
