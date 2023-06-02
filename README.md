# Jaeger Tracing & Prometheus Monitoring Demo

A showcase app to demonstrate tracing in a microservice pattern. Also, a metrics endpoint with additional basic custom metrics is exposed.

## Development
run
```bash
docker-compose build && docker-compose up
```

you can also run native go by changing

```go
	viper.SetConfigFile("config.yaml")
```
to a valid config file in the cfg folder and then execute

```bash
go run cmd/main.go
```

## Usage

/app: sleep a random amount of seconds (set the max number of seconds in the config.yaml of each service in the cfg folder) and call all services listed in the forward-urls set. If the set is empty, return a string and response 200.

/leak: Start allocating heap memory until the pod explodes (showcase k8s behavour when a pod reaches its limit and is shut down by k8s).

### Metrics
are exposed on a seperate port 2112 on /metrics
kube-prometheus stack deployment needs to be configured to scrape this target
(https://artifacthub.io/packages/helm/prometheus-community/kube-prometheus-stack)

### Traces
are exported but need a configured jaeger-all-in-one deployment (https://artifacthub.io/packages/helm/jaeger-all-in-one/jaeger-all-in-one)

# Deployment on k8s

Deploy on k8s, you can use minikube (https://minikube.sigs.k8s.io/docs/start/)

### Preliminaries in chart/values.yaml
Configure the repo and tag for your container image
Configure the namespace for the endpoint of your jaeger-all-in-one agent.

## Installation
run
```bash
helm upgrade -i -n <NAMESPACE> jaeger-tracing-app chart/jaeger-tracing
```

## Usage

Portforward the gateway (or any service) with kubectl

/app: sleep a random amount of seconds (set the max number of seconds in the config.yaml of each service in the cfg folder) and call all services listed in the forward-urls set. If the set is empty, return a string and response 200.

/leak: Start allocating heap memory until the pod explodes (is shut down by k8s due to limits defined in the values.yaml).
