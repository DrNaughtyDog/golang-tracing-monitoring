package monitoring

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	callsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "server_processed_calls",
		Help: "The total number of processed calls of the server",
	})
	clientCallsSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "client_sent_calls",
		Help: "The total number of sent calls via http client",
	})
	clientCallsSuccesful = promauto.NewCounter(prometheus.CounterOpts{
		Name: "caller_sent_calls_successful",
		Help: "The total number of successful calls via http client",
	})
	clientCallsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "caller_sent_calls_failed",
		Help: "The total number of failed calls via http client",
	})
	clientCallsDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "caller_duration_calls",
		Help:    "The duration of processed calls",
		Buckets: []float64{0, 1, 2, 4, 6, 10, 16, 26},
	})
)

func ServerRecordRequest() {
	go func() {
		callsProcessed.Inc()
	}()
}

func LoadRecordRequest() {
	go func() {
		clientCallsSent.Inc()
	}()
}

func LoadRecordResponse(succesful bool, durationSeconds float64) {
	go func() {
		if succesful {
			clientCallsSuccesful.Inc()
		} else {
			clientCallsFailed.Inc()
		}
		clientCallsDuration.Observe(durationSeconds)
	}()
}

func InitAsync() {
	go func() {
		Init()
	}()
}

func Init() {
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
