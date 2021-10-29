// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/service/parserprovider"
)

// buildKey creates a key for use in the config.Map.Set function.
func buildKey(keys ...string) string {
	return strings.Join(keys, config.KeyDelimiter)
}

var _ parserprovider.MapProvider = (*mapProvider)(nil)

type mapProvider config.Map

func (p mapProvider) Get(context.Context) (*config.Map, error) {
	return (*config.Map)(&p), nil
}

func (p mapProvider) Close(context.Context) error {
	return nil
}

// defaultTracesConfig is the base traces OTLP pipeline configuration.
// This pipeline is extended through the datadog.yaml configuration values.
// It is written in YAML because it is easier to read and write than a map.
// TODO (AP-1254): Set service-level configuration when available.
const defaultTracesConfig string = `
receivers:
  otlp:

exporters:
  otlp:
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`

func newTracesMapProvider(tracePort uint) parserprovider.MapProvider {
	configMap := config.NewMap()
	configMap.Set(buildKey("exporters", "otlp", "endpoint"), fmt.Sprintf("%s:%d", "localhost", tracePort))
	return parserprovider.NewMergeMapProvider(
		parserprovider.NewInMemoryMapProvider(strings.NewReader(defaultTracesConfig)),
		mapProvider(*configMap),
	)
}

// defaultMetricsConfig is the metrics OTLP pipeline configuration.
// TODO (AP-1254): Set service-level configuration when available.
const defaultMetricsConfig string = `
receivers:
  otlp:

processors:
  batch:

exporters:
  serializer:

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [serializer]
`

func newMetricsMapProvider() parserprovider.MapProvider {
	return parserprovider.NewInMemoryMapProvider(strings.NewReader(defaultMetricsConfig))
}

func otlpConfigFromPorts(bindHost string, gRPCPort uint, httpPort uint) map[string]interface{} {
	otlpConfig := map[string]interface{}{"protocols": map[string]interface{}{}}

	if gRPCPort > 0 {
		otlpConfig["protocols"].(map[string]interface{})["grpc"] = map[string]interface{}{
			"endpoint": fmt.Sprintf("%s:%d", bindHost, gRPCPort),
		}
	}
	if httpPort > 0 {
		otlpConfig["protocols"].(map[string]interface{})["http"] = map[string]interface{}{
			"endpoint": fmt.Sprintf("%s:%d", bindHost, httpPort),
		}
	}
	return otlpConfig
}

func newReceiverProvider(otlpReceiverConfig map[string]interface{}) parserprovider.MapProvider {
	configMap := config.NewMapFromStringMap(map[string]interface{}{
		"receivers": map[string]interface{}{"otlp": otlpReceiverConfig},
	})
	return mapProvider(*configMap)
}

// newMapProvider creates a parserprovider.MapProvider with the fixed configuration.
func newMapProvider(cfg PipelineConfig) parserprovider.MapProvider {
	var providers []parserprovider.MapProvider
	if cfg.TracesEnabled {
		providers = append(providers, newTracesMapProvider(cfg.TracePort))
	}
	if cfg.MetricsEnabled {
		providers = append(providers, newMetricsMapProvider())
	}
	providers = append(providers, newReceiverProvider(cfg.OTLPReceiverConfig))
	return parserprovider.NewMergeMapProvider(providers...)
}
