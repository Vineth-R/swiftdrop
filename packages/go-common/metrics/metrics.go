// Package metrics is a small, dependency-free Prometheus text-format registry.
//
// It exists so every SwiftDrop service can expose a consistent /metrics
// endpoint without pulling a metrics SDK into a student deployment. The
// registry is created explicitly by each service (no global state) and is safe
// for concurrent use.
//
// Use it for low-cardinality operational signals only: never put a user ID,
// parcel ID, tracking number, token, merchant secret or free-form error message
// in a label.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DefaultBuckets are the HTTP latency buckets in seconds.
var DefaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type kind string

const (
	kindCounter   kind = "counter"
	kindGauge     kind = "gauge"
	kindHistogram kind = "histogram"
)

type series struct {
	values  []string
	value   float64
	count   uint64
	sum     float64
	buckets []uint64
}

type family struct {
	name       string
	help       string
	kind       kind
	labelNames []string
	buckets    []float64

	mu     sync.Mutex
	series map[string]*series
}

// Registry owns a set of metric families.
type Registry struct {
	mu       sync.Mutex
	families map[string]*family
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{families: make(map[string]*family)}
}

func (r *Registry) family(name, help string, k kind, labels []string, buckets []float64) *family {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.families[name]; ok {
		return existing
	}
	f := &family{
		name:       name,
		help:       help,
		kind:       k,
		labelNames: append([]string(nil), labels...),
		buckets:    buckets,
		series:     make(map[string]*series),
	}
	r.families[name] = f
	return f
}

// Counter returns a monotonically increasing metric family.
func (r *Registry) Counter(name, help string, labels ...string) *Counter {
	return &Counter{family: r.family(name, help, kindCounter, labels, nil)}
}

// Gauge returns a metric family that can go up and down.
func (r *Registry) Gauge(name, help string, labels ...string) *Gauge {
	return &Gauge{family: r.family(name, help, kindGauge, labels, nil)}
}

// Histogram returns a histogram with DefaultBuckets.
func (r *Registry) Histogram(name, help string, labels ...string) *Histogram {
	return &Histogram{family: r.family(name, help, kindHistogram, labels, DefaultBuckets)}
}

// Counter is a handle to one counter family.
type Counter struct{ family *family }

// CounterSeries is one labelled counter stream.
type CounterSeries struct {
	family *family
	series *series
}

// With returns the stream for the supplied label values.
func (c *Counter) With(values ...string) *CounterSeries {
	return &CounterSeries{family: c.family, series: c.family.observe(values)}
}

// Inc adds one.
func (s *CounterSeries) Inc() { s.Add(1) }

// Add increases the counter. Negative values are ignored.
func (s *CounterSeries) Add(value float64) {
	if value < 0 {
		return
	}
	s.family.mu.Lock()
	s.series.value += value
	s.family.mu.Unlock()
}

// Gauge is a handle to one gauge family.
type Gauge struct{ family *family }

// GaugeSeries is one labelled gauge stream.
type GaugeSeries struct {
	family *family
	series *series
}

// With returns the stream for the supplied label values.
func (g *Gauge) With(values ...string) *GaugeSeries {
	return &GaugeSeries{family: g.family, series: g.family.observe(values)}
}

// Set replaces the gauge value.
func (s *GaugeSeries) Set(value float64) {
	s.family.mu.Lock()
	s.series.value = value
	s.family.mu.Unlock()
}

// Add changes the gauge by delta.
func (s *GaugeSeries) Add(delta float64) {
	s.family.mu.Lock()
	s.series.value += delta
	s.family.mu.Unlock()
}

// Inc adds one.
func (s *GaugeSeries) Inc() { s.Add(1) }

// Dec subtracts one.
func (s *GaugeSeries) Dec() { s.Add(-1) }

// Histogram is a handle to one histogram family.
type Histogram struct{ family *family }

// HistogramSeries is one labelled histogram stream.
type HistogramSeries struct {
	family *family
	series *series
}

// With returns the stream for the supplied label values.
func (h *Histogram) With(values ...string) *HistogramSeries {
	return &HistogramSeries{family: h.family, series: h.family.observe(values)}
}

// Observe records one value.
func (s *HistogramSeries) Observe(value float64) {
	s.family.mu.Lock()
	defer s.family.mu.Unlock()
	s.series.count++
	s.series.sum += value
	for i, upper := range s.family.buckets {
		if value <= upper {
			// Store each observation in exactly its smallest fitting bucket;
			// rendering turns these into cumulative counts.
			s.series.buckets[i]++
			break
		}
	}
}

func (f *family) observe(values []string) *series {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := encode(values)
	if existing, ok := f.series[key]; ok {
		return existing
	}
	created := &series{values: append([]string(nil), values...)}
	if f.kind == kindHistogram {
		created.buckets = make([]uint64, len(f.buckets))
	}
	f.series[key] = created
	return created
}

func encode(values []string) string {
	var builder strings.Builder
	for _, value := range values {
		builder.WriteString(strconv.Itoa(len(value)))
		builder.WriteByte(':')
		builder.WriteString(value)
	}
	return builder.String()
}

// WriteTo renders the registry in Prometheus text exposition format (0.0.4).
// Families and label streams are sorted so output is stable.
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	r.mu.Lock()
	families := make([]*family, 0, len(r.families))
	for _, f := range r.families {
		families = append(families, f)
	}
	r.mu.Unlock()
	sort.Slice(families, func(i, j int) bool { return families[i].name < families[j].name })

	var total int64
	write := func(format string, args ...any) error {
		n, err := fmt.Fprintf(w, format, args...)
		total += int64(n)
		return err
	}

	for _, f := range families {
		if err := write("# HELP %s %s\n# TYPE %s %s\n", f.name, escapeHelp(f.help), f.name, f.kind); err != nil {
			return total, err
		}
		f.mu.Lock()
		keys := make([]string, 0, len(f.series))
		for key := range f.series {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			s := f.series[key]
			switch f.kind {
			case kindHistogram:
				cumulative := uint64(0)
				for i, upper := range f.buckets {
					cumulative += s.buckets[i]
					if err := write("%s_bucket%s %d\n", f.name, labelBlock(f.labelNames, s.values, "le", strconv.FormatFloat(upper, 'g', -1, 64)), cumulative); err != nil {
						f.mu.Unlock()
						return total, err
					}
				}
				if err := write("%s_bucket%s %d\n", f.name, labelBlock(f.labelNames, s.values, "le", "+Inf"), s.count); err != nil {
					f.mu.Unlock()
					return total, err
				}
				if err := write("%s_sum%s %g\n", f.name, labelBlock(f.labelNames, s.values), s.sum); err != nil {
					f.mu.Unlock()
					return total, err
				}
				if err := write("%s_count%s %d\n", f.name, labelBlock(f.labelNames, s.values), s.count); err != nil {
					f.mu.Unlock()
					return total, err
				}
			default:
				if err := write("%s%s %g\n", f.name, labelBlock(f.labelNames, s.values), s.value); err != nil {
					f.mu.Unlock()
					return total, err
				}
			}
		}
		f.mu.Unlock()
	}
	return total, nil
}

// labelBlock renders {name="value",...}, optionally appending one extra pair.
func labelBlock(names, values []string, extra ...string) string {
	pairs := make([]string, 0, len(names)+1)
	limit := len(names)
	if len(values) < limit {
		limit = len(values)
	}
	for i := 0; i < limit; i++ {
		pairs = append(pairs, names[i]+`="`+escapeLabel(values[i])+`"`)
	}
	if len(extra) == 2 {
		pairs = append(pairs, extra[0]+`="`+escapeLabel(extra[1])+`"`)
	}
	if len(pairs) == 0 {
		return ""
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func escapeHelp(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}
