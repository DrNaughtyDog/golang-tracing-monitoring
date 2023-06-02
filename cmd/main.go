package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DrNaughtyDog/golang-tracing-monitoring/cmd/monitoring"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	Name                  string                   `mapstructure:"name"`
	Port                  int32                    `mapstructure:"port"`
	MaxSleepDuration      int16                    `mapstructure:"sleep-max-seconds"`
	RequestTimeoutSeconds int                      `mapstructure:"request-timeout-seconds"`
	ForwardUrls           []map[string]interface{} `mapstructure:"forward-urls"`
}
type responseData struct {
	statusCode   int
	responseBody string
}

var tracer trace.Tracer

var Configuration = Config{}

func main() {
	debug := flag.Bool("debug", false, "sets log level to debug")
	jaegerEndpoint := flag.String("jaeger-collector-endpoint", "http://localhost:14268", "set the endpoint for the jaeger collector")
	flag.Parse()
	// Default level for this example is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// Use human-friendly, colorized output on console
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	readConfig()

	// Init tracing
	tracer = otel.GetTracerProvider().Tracer(Configuration.Name)
	tp, err := tracerProvider(fmt.Sprintf("%s/api/traces", *jaegerEndpoint))
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Fatal().Msg(err.Error())
		}
	}()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	rand.Seed(time.Now().UnixNano())
	monitoring.InitAsync()

	// Create a new custom ServeMux
	mux := http.NewServeMux()

	// Register handlers for specific paths
	mux.Handle("/leak", NewHandler(http.HandlerFunc(handleReqMemLeak), "handleRequestWithLeak"))
	mux.HandleFunc("/healthz", handleHealthCheck)
	mux.Handle("/app", NewHandler(http.HandlerFunc(handleReq), "handleRequest"))

	// Register a catch-all handler for unmatched paths
	mux.Handle("/", NewHandler(http.HandlerFunc(handleRequestNotFound), "handleRequestNotFound"))
	// Use the custom ServeMux
	server := &http.Server{Addr: fmt.Sprintf(":%v", Configuration.Port), Handler: mux}
	log.Info().Msgf("Starting server on port %d", Configuration.Port)

	err = server.ListenAndServe()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}

func NewHandler(h http.Handler, operation string) http.Handler {
	httpOptions := []otelhttp.Option{
		otelhttp.WithTracerProvider(otel.GetTracerProvider()),
		otelhttp.WithPropagators(propagation.TraceContext{}),
	}
	return otelhttp.NewHandler(h, operation, httpOptions...)
}

func handleReq(w http.ResponseWriter, r *http.Request) {
	defer monitoring.ServerRecordRequest()
	ctx := r.Context()
	otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
	log.Debug().Msgf("Passed headers in request: %s", r.Header)
	sleep(ctx)
	if Configuration.ForwardUrls != nil {
		var responseDatas []responseData
		for _, urlMaps := range Configuration.ForwardUrls {
			var wg sync.WaitGroup
			responses := make(chan responseData, len(Configuration.ForwardUrls[0]))
			for key := range urlMaps {
				wg.Add(1)
				go fireRequestAsync(ctx, &wg, responses, key)
			}
			wg.Wait()
			close(responses)

			for responseData := range responses {
				responseDatas = append(responseDatas, responseData)
			}
		}
		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("%v", responseDatas)))
	} else {
		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("Hello from %s", Configuration.Name)))
	}
}

func handleRequestNotFound(w http.ResponseWriter, r *http.Request) {
	defer monitoring.ServerRecordRequest()
	ctx := r.Context()
	otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
	log.Debug().Msgf("Passed headers in request: %s", r.Header)
	w.WriteHeader(404)
	w.Write([]byte(fmt.Sprint("404 - Path not found")))
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

func handleReqMemLeak(w http.ResponseWriter, r *http.Request) {
	defer monitoring.ServerRecordRequest()
	ctx := r.Context()
	otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
	log.Debug().Msgf("Passed headers in request: %s", r.Header)
	causeMemLeak()
}

func sleep(ctx context.Context) {
	_, childSpan := tracer.Start(ctx, "sleep")
	defer childSpan.End()

	log.Debug().Msgf("Max sleep duration in seconds is %v", Configuration.MaxSleepDuration)
	n := 1 + rand.Intn(int(Configuration.MaxSleepDuration))
	log.Info().Msgf("A sleeping pokemon blocks the way for %v seconds", time.Duration(n)*time.Second)
	time.Sleep(time.Duration(n) * time.Second)
	log.Info().Msg("Snorlax woke up!")
}

func fireRequestAsync(ctx context.Context, wg *sync.WaitGroup, receiver chan<- responseData, forwardUrl string) {
	log.Debug().Msgf("Timeout for the Request in seconds is: %v", Configuration.RequestTimeoutSeconds)
	client := &http.Client{
		Timeout:   time.Duration(Configuration.RequestTimeoutSeconds) * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/app", forwardUrl), nil)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	log.Debug().Msgf("Forward to url: http://%s/\n", forwardUrl)
	start := time.Now()
	monitoring.LoadRecordRequest()
	response, err := client.Do(req)
	elapsedSeconds := time.Since(start).Seconds()
	if err != nil {
		if os.IsTimeout(err) {
			log.Debug().Msgf("Timeout while calling %s. Returning Status 408", forwardUrl)
			receiver <- responseData{
				statusCode:   408,
				responseBody: fmt.Sprintf("Request Timeout when calling %s", forwardUrl),
			}
			monitoring.LoadRecordResponse(false, elapsedSeconds)
			wg.Done()
			return
		} else {
			receiver <- responseData{
				statusCode:   404,
				responseBody: fmt.Sprintf("Empty response from %s", forwardUrl),
			}
			monitoring.LoadRecordResponse(false, elapsedSeconds)
			wg.Done()
			return
		}
	}
	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Info().Msg(err.Error())
		monitoring.LoadRecordResponse(false, elapsedSeconds)
		receiver <- responseData{
			statusCode:   500,
			responseBody: err.Error(),
		}
	} else {
		response.Body.Close()
		monitoring.LoadRecordResponse(true, elapsedSeconds)
		receiver <- responseData{
			statusCode:   200,
			responseBody: string(responseBytes),
		}
	}
	wg.Done()
}

func readConfig() {
	viper.SetConfigFile("config.yaml")
	log.Debug().Msg("Reading config file")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
	err = viper.Unmarshal(&Configuration)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
	log.Info().Msgf("Using config with: %+v", Configuration)
}
func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(Configuration.Name),
			attribute.String("environment", "dev"),
		)),
	)
	return tp, nil
}

func causeMemLeak() {
	var slice []byte

	for i := 0; i < 1000000; i++ {
		b := make([]byte, 1024*1024) // Allocate 1 MB of memory.
		slice = append(slice, b...)
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		memUsage := float64(memStats.Alloc) / 1024 / 1024
		log.Debug().Msgf("Allocated %v MB of memory\n", memUsage)

		// Sleep for a short time to slow down memory allocation.
		time.Sleep(300 * time.Millisecond)
	}
}
