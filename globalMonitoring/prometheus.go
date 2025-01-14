package globalMonitoring

import (
	"context"
	"fmt"
	"github.com/micro/go-micro/v2/server"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"sync"
)

// metricsHandler is defines and creates metrics to be exported
type metricsHandler struct {
	options Options
	//callFunc client.CallFunc
	//client.Client
}

// metricsDef holds the definition of the metrics to be exported
type metricsDef struct {
	opsCounter           *prometheus.CounterVec
	timeCounterSummary   *prometheus.SummaryVec
	timeCounterHistogram *prometheus.HistogramVec
}

// Options related to metrics scraping and export
type Options struct {
	Name               string
	Version            string
	ID                 string
	MetricsPrefix      string
	MetricsLabelPrefix string
}
type Option func(*Options)

// metrics values to be exported
var metrics metricsDef

// mu mutex to setup metrics
var mu sync.Mutex

// ServiceName set ups name of the service
func ServiceName(name string) Option {
	return func(opts *Options) {
		opts.Name = name
	}
}

// ServiceVersion set ups the current version of the service
func ServiceVersion(version string) Option {
	return func(opts *Options) {
		opts.Version = version
	}
}

// ServiceID set ups the Id of the service
func ServiceID(id string) Option {
	return func(opts *Options) {
		opts.ID = id
	}
}

// ServiceMetricsPrefix is the prefix used for all the metrics exported
func ServiceMetricsPrefix(metricsPrefix string) Option {
	return func(opts *Options) {
		opts.MetricsPrefix = metricsPrefix
	}
}

// ServiceMetricsLabelPrefix is the prefix used for all the labels of the metrics exported
func ServiceMetricsLabelPrefix(metricsLabelPrefix string) Option {
	return func(opts *Options) {
		opts.MetricsLabelPrefix = metricsLabelPrefix
	}
}

// setupCounterVec returns a new prometheus counter vector with all the necessary labels
func setupCounterVec(name, help, metricPrefix, labelPrefix string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: fmt.Sprintf("%s%s", metricPrefix, name),
			Help: help,
		},
		[]string{
			fmt.Sprintf("%s%s", labelPrefix, "name"),
			fmt.Sprintf("%s%s", labelPrefix, "version"),
			fmt.Sprintf("%s%s", labelPrefix, "id"),
			fmt.Sprintf("%s%s", labelPrefix, "endpoint"),
			fmt.Sprintf("%s%s", labelPrefix, "status"),
		},
	)
}

// setupSummaryVec returns a new prometheus summary vector with all the necessary labels
func setupSummaryVec(name, help, metricPrefix, labelPrefix string) *prometheus.SummaryVec {
	return prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: fmt.Sprintf("%s%s", metricPrefix, name),
			Help: help,
		},
		[]string{
			fmt.Sprintf("%s%s", labelPrefix, "name"),
			fmt.Sprintf("%s%s", labelPrefix, "version"),
			fmt.Sprintf("%s%s", labelPrefix, "id"),
			fmt.Sprintf("%s%s", labelPrefix, "endpoint"),
		},
	)
}

// setupHistogramVec returns a new prometheus histogram vector with all the necessary labels
func setupHistogramVec(name, help, metricPrefix, labelPrefix string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: fmt.Sprintf("%s%s", metricPrefix, name),
			Help: help,
		},
		[]string{
			fmt.Sprintf("%s%s", labelPrefix, "name"),
			fmt.Sprintf("%s%s", labelPrefix, "version"),
			fmt.Sprintf("%s%s", labelPrefix, "id"),
			fmt.Sprintf("%s%s", labelPrefix, "endpoint"),
		},
	)
}

// registerMetrics creates and registers the metrics to be exported from the system
func (w *metricsHandler) registerMetrics(metricPrefix, labelPrefix string) {
	mu.Lock()
	defer mu.Unlock()

	if metrics.opsCounter == nil {
		metrics.opsCounter = setupCounterVec("request_total",
			"Requests processed, partitioned by endpoint and status",
			metricPrefix, labelPrefix)
	}

	if metrics.timeCounterSummary == nil {
		metrics.timeCounterSummary = setupSummaryVec("latency_microseconds",
			"Request latencies in microseconds, partitioned by endpoint",
			metricPrefix, labelPrefix)

	}

	if metrics.timeCounterHistogram == nil {
		metrics.timeCounterHistogram = setupHistogramVec("request_duration_seconds",
			"Request time in seconds, partitioned by endpoint",
			metricPrefix, labelPrefix)
	}

	for _, collector := range []prometheus.Collector{metrics.opsCounter, metrics.timeCounterSummary, metrics.timeCounterHistogram} {
		if err := prometheus.DefaultRegisterer.Register(collector); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				log.Printf("Promethus collector already registered: %s", err)
			}
		}
	}

}

// NewMetricsWrapper creates a new metrics handler and  call function to create and register the metrics to be exported
func NewMetricsWrapper(opts ...Option) server.HandlerWrapper {

	options := Options{}
	for _, opt := range opts {
		opt(&options)
	}

	handler := &metricsHandler{
		options: options,
	}
	handler.registerMetrics(handler.options.MetricsPrefix, handler.options.MetricsLabelPrefix)

	return handler.metricsWrapper
}

//metricsWrapper runs everytime one of the end points of the service is called and updates the metrics to be exported
func (w *metricsHandler) metricsWrapper(fn server.HandlerFunc) server.HandlerFunc {
	return func(ctx context.Context, req server.Request, resp interface{}) error {
		endpoint := req.Endpoint()

		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			us := v * 1000000 // make microseconds
			metrics.timeCounterSummary.WithLabelValues(w.options.Name, w.options.Version, w.options.ID, endpoint).Observe(us)
			metrics.timeCounterHistogram.WithLabelValues(w.options.Name, w.options.Version, w.options.ID, endpoint).Observe(v)
		}))
		defer timer.ObserveDuration()

		err := fn(ctx, req, resp)

		if err == nil {
			metrics.opsCounter.WithLabelValues(w.options.Name, w.options.Version, w.options.ID, endpoint, "success").Inc()
		} else {
			metrics.opsCounter.WithLabelValues(w.options.Name, w.options.Version, w.options.ID, endpoint, "failure").Inc()
		}

		return err
	}
}
