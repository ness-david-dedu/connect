// Copyright 2026 Redpanda Data, Inc.
//
// Licensed as a Redpanda Enterprise file under the Redpanda Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// https://github.com/redpanda-data/connect/blob/main/licenses/rcl.md

// Package salesforce provides a Benthos salesforceProcessor that integrates with the Salesforce APIs
// to fetch data based on input messages. It allows querying Salesforce resources
// such as .... TODO

package salesforce

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"

	"github.com/redpanda-data/connect/v4/internal/impl/salesforce/salesforcegrpc"
	"github.com/redpanda-data/connect/v4/internal/impl/salesforce/salesforcehttp"
)

// Config field name constants — prefixed sfp (salesforce processor) to avoid
// duplication between the spec definition and the parsing code.
const (
	sfpFieldOrgURL                   = "org_url"
	sfpFieldClientID                 = "client_id"
	sfpFieldClientSecret             = "client_secret"
	sfpFieldRESTAPIVersion           = "restapi_version"
	sfpFieldRequestTimeout           = "request_timeout"
	sfpFieldMaxRetries               = "max_retries"
	sfpFieldQueryType                = "query_type"
	sfpFieldQuery                    = "query"
	sfpFieldCDCEnabled               = "cdc_enabled"
	sfpFieldCDCObjects               = "cdc_objects"
	sfpFieldCDCBatchSize             = "cdc_batch_size"
	sfpFieldCDCBufferSize            = "cdc_buffer_size"
	sfpFieldCDCReplayPreset          = "cdc_replay_preset"
	sfpFieldPubSubTopic              = "pubsub_topic"
	sfpFieldGRPCReconnectBaseDelay   = "grpc_reconnect_base_delay"
	sfpFieldGRPCReconnectMaxDelay    = "grpc_reconnect_max_delay"
	sfpFieldGRPCReconnectMaxAttempts = "grpc_reconnect_max_attempts"
	sfpFieldGRPCShutdownTimeout      = "grpc_shutdown_timeout"
	sfpFieldCacheResource            = "cache_resource"
)

// SalesforceProcessorConfig holds all parsed configuration for the Salesforce processor.
type SalesforceProcessorConfig struct {
	OrgURL                   string
	ClientID                 string
	ClientSecret             string
	APIVersion               string
	RequestTimeout           time.Duration
	MaxRetries               int
	QueryType                string
	Query                    string
	CDCEnabled               bool
	CDCObjects               []string
	CDCBatchSize             int
	CDCBufferSize            int
	CDCReplayPreset          string
	PubSubTopic              string
	GRPCReconnectBaseDelay   time.Duration
	GRPCReconnectMaxDelay    time.Duration
	GRPCReconnectMaxAttempts int
	GRPCShutdownTimeout      time.Duration
	CacheResource            string
}

// salesforceProcessorConfigFromParsed extracts and validates all configuration
// fields from a parsed Benthos config into a SalesforceProcessorConfig.
func salesforceProcessorConfigFromParsed(conf *service.ParsedConfig) (SalesforceProcessorConfig, error) {
	var cfg SalesforceProcessorConfig
	var err error

	if cfg.OrgURL, err = conf.FieldString(sfpFieldOrgURL); err != nil {
		return cfg, err
	}
	if _, err = url.ParseRequestURI(cfg.OrgURL); err != nil {
		return cfg, errors.New("org_url is not a valid URL")
	}

	if cfg.ClientID, err = conf.FieldString(sfpFieldClientID); err != nil {
		return cfg, err
	}
	if cfg.ClientSecret, err = conf.FieldString(sfpFieldClientSecret); err != nil {
		return cfg, err
	}
	if cfg.APIVersion, err = conf.FieldString(sfpFieldRESTAPIVersion); err != nil {
		return cfg, err
	}
	if cfg.RequestTimeout, err = conf.FieldDuration(sfpFieldRequestTimeout); err != nil {
		return cfg, err
	}
	if cfg.MaxRetries, err = conf.FieldInt(sfpFieldMaxRetries); err != nil {
		return cfg, err
	}
	if cfg.QueryType, err = conf.FieldString(sfpFieldQueryType); err != nil {
		return cfg, err
	}
	if cfg.Query, err = conf.FieldString(sfpFieldQuery); err != nil {
		return cfg, err
	}
	if cfg.CDCEnabled, err = conf.FieldBool(sfpFieldCDCEnabled); err != nil {
		return cfg, err
	}
	if cfg.CDCObjects, err = conf.FieldStringList(sfpFieldCDCObjects); err != nil {
		return cfg, err
	}
	if cfg.CDCBatchSize, err = conf.FieldInt(sfpFieldCDCBatchSize); err != nil {
		return cfg, err
	}
	if cfg.CDCBufferSize, err = conf.FieldInt(sfpFieldCDCBufferSize); err != nil {
		return cfg, err
	}
	if cfg.CDCReplayPreset, err = conf.FieldString(sfpFieldCDCReplayPreset); err != nil {
		return cfg, err
	}
	if cfg.PubSubTopic, err = conf.FieldString(sfpFieldPubSubTopic); err != nil {
		return cfg, err
	}
	if cfg.GRPCReconnectBaseDelay, err = conf.FieldDuration(sfpFieldGRPCReconnectBaseDelay); err != nil {
		return cfg, err
	}
	if cfg.GRPCReconnectMaxDelay, err = conf.FieldDuration(sfpFieldGRPCReconnectMaxDelay); err != nil {
		return cfg, err
	}
	if cfg.GRPCReconnectMaxAttempts, err = conf.FieldInt(sfpFieldGRPCReconnectMaxAttempts); err != nil {
		return cfg, err
	}
	if cfg.GRPCShutdownTimeout, err = conf.FieldDuration(sfpFieldGRPCShutdownTimeout); err != nil {
		return cfg, err
	}
	if cfg.CacheResource, err = conf.FieldString(sfpFieldCacheResource); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// salesforceProcessor is the Benthos salesforceProcessor implementation for Salesforce queries.
// It holds the client state and orchestrates calls into the salesforcehttp package.
type salesforceProcessor struct {
	log         *service.Logger
	client      *salesforcehttp.Client
	res         *service.Resources
	binLogCache string
	req         Request

	// CDC configuration
	cdcEnabled      bool
	cdcObjects      []string
	cdcTopicName    string
	cdcBatchSize    int32
	cdcBufferSize   int32
	cdcReplayPreset string

	// Pub/Sub topic override (for Platform Events or arbitrary topics)
	pubsubTopic string

	// gRPC reconnection backoff settings
	grpcReconnectBaseDelay   time.Duration
	grpcReconnectMaxDelay    time.Duration
	grpcReconnectMaxAttempts int

	// gRPC shutdown timeout
	grpcShutdownTimeout time.Duration

	// gRPC client for CDC/Pub/Sub streaming (lazy-initialized)
	grpcClient *salesforcegrpc.Client
}

func init() {
	if err := service.RegisterProcessor(
		"salesforce", newSalesforceProcessorConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			return newSalesforceProcessor(conf, mgr)
		},
	); err != nil {
		panic(err)
	}
}

// newSalesforceProcessorConfigSpec creates a new Configuration specification for the Salesforce processor
func newSalesforceProcessorConfigSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Fetches data from Salesforce based on input messages").
		Description(`This salesforceProcessor takes input messages containing Salesforce queries and returns Salesforce data.

Supports the following Salesforce resources:
- todo

Configuration examples:

` + "```configYAML" + `
# Minimal configuration
pipeline:
  processors:
    - salesforce:
        org_url: "https://your-domain.salesforce.com"
        client_id: "${SALESFORCE_CLIENT_ID}"
        client_secret: "${SALESFORCE_CLIENT_SECRET}"

# Full configuration with CDC
pipeline:
  processors:
    - salesforce:
        org_url: "https://your-domain.salesforce.com"
        client_id: "${SALESFORCE_CLIENT_ID}"
        client_secret: "${SALESFORCE_CLIENT_SECRET}"
        restapi_version: "v64.0"
        request_timeout: "30s"
        max_retries: 50
        cdc_enabled: true
        cdc_objects:
          - Account
          - Contact
        cdc_batch_size: 100
        cdc_buffer_size: 1000
        cdc_replay_preset: "latest"

# Platform Events (standalone, no REST snapshot)
pipeline:
  processors:
    - salesforce:
        org_url: "https://your-domain.salesforce.com"
        client_id: "${SALESFORCE_CLIENT_ID}"
        client_secret: "${SALESFORCE_CLIENT_SECRET}"
        pubsub_topic: "/event/MyEvent__e"
` + "```").
		Field(service.NewStringField(sfpFieldOrgURL).
			Description("Salesforce instance base URL (e.g., https://your-domain.salesforce.com)")).
		Field(service.NewStringField(sfpFieldClientID).
			Description("Client ID for the Salesforce Connected App")).
		Field(service.NewStringField(sfpFieldClientSecret).
			Description("Client Secret for the Salesforce Connected App").
			Secret()).
		Field(service.NewStringField(sfpFieldRESTAPIVersion).
			Description("Salesforce REST API version to use (example: v64.0). Default: v65.0").
			Default("v65.0")).
		Field(service.NewDurationField(sfpFieldRequestTimeout).
			Description("HTTP request timeout").
			Default("30s")).
		Field(service.NewIntField(sfpFieldMaxRetries).
			Description("Maximum number of retries in case of 429 HTTP Status Code").
			Default(10)).
		// sfpFieldQueryType selects which Salesforce API to use:
		//   "rest"    – REST API (default). Without a query, fetches all SObjects with checkpointing.
		//   "graphql" – GraphQL API. Requires a query to be provided.
		Field(service.NewStringField(sfpFieldQueryType).
			Description("API mode: \"rest\" (default) or \"graphql\"").
			Default("rest")).
		// sfpFieldQuery is an optional SOQL or GraphQL query string.
		//   For rest:    a SOQL expression, e.g. "SELECT Id, Name FROM Account"
		//   For graphql: a GraphQL query string
		// When omitted the processor defaults to fetching all SObjects via REST with checkpointing.
		Field(service.NewStringField(sfpFieldQuery).
			Description("Optional SOQL (rest) or GraphQL query. When empty, all SObjects are fetched via REST.").
			Default("")).
		// CDC configuration fields
		Field(service.NewBoolField(sfpFieldCDCEnabled).
			Description("Enable Change Data Capture streaming after REST snapshot completes").
			Default(false)).
		Field(service.NewStringListField(sfpFieldCDCObjects).
			Description("SObject types to capture changes for (e.g., [\"Account\", \"Contact\"]). When empty, subscribes to /data/ChangeEvents for all objects.").
			Default([]any{})).
		Field(service.NewIntField(sfpFieldCDCBatchSize).
			Description("Number of CDC events to request per gRPC fetch").
			Default(100)).
		Field(service.NewIntField(sfpFieldCDCBufferSize).
			Description("Size of the internal CDC event buffer").
			Default(1000)).
		Field(service.NewStringField(sfpFieldCDCReplayPreset).
			Description("CDC replay preset when no checkpoint exists: \"latest\" (default) or \"earliest\"").
			Default("latest")).
		// Pub/Sub topic override (for Platform Events or arbitrary topics)
		Field(service.NewStringField(sfpFieldPubSubTopic).
			Description("Arbitrary Pub/Sub API topic (e.g., \"/event/MyEvent__e\"). When set, overrides cdc_objects for topic selection.").
			Default("")).
		// gRPC reconnection backoff settings
		Field(service.NewDurationField(sfpFieldGRPCReconnectBaseDelay).
			Description("Base delay for gRPC reconnection backoff").
			Default("500ms")).
		Field(service.NewDurationField(sfpFieldGRPCReconnectMaxDelay).
			Description("Maximum delay for gRPC reconnection backoff").
			Default("30s")).
		Field(service.NewIntField(sfpFieldGRPCReconnectMaxAttempts).
			Description("Maximum number of gRPC reconnection attempts (0 = unlimited)").
			Default(0)).
		// gRPC shutdown timeout
		Field(service.NewDurationField(sfpFieldGRPCShutdownTimeout).
			Description("Timeout for graceful gRPC client shutdown").
			Default("10s")).
		Field(service.NewStringField(sfpFieldCacheResource).
			Description("Name of the Benthos cache resource used for checkpointing state (must be defined in cache_resources).").
			Default("salesforce_checkpoint"))
}

func newSalesforceProcessor(conf *service.ParsedConfig, mgr *service.Resources) (*salesforceProcessor, error) {
	//if err := license.CheckRunningEnterprise(mgr); err != nil {
	//	return nil, err
	//}

	cfg, err := salesforceProcessorConfigFromParsed(conf)
	if err != nil {
		return nil, err
	}

	req, err := buildRequest(cfg.QueryType, cfg.Query)
	if err != nil {
		return nil, err
	}

	// Build the CDC topic name: pubsub_topic takes priority, otherwise derive from cdc_objects
	cdcTopicName := cfg.PubSubTopic
	if cdcTopicName == "" {
		cdcTopicName = buildCDCTopicName(cfg.CDCObjects)
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}

	salesforceHttp, err := salesforcehttp.NewClient(cfg.OrgURL, cfg.ClientID, cfg.ClientSecret, cfg.APIVersion, cfg.MaxRetries, httpClient, mgr.Logger(), mgr.Metrics())
	if err != nil {
		return nil, err
	}

	return &salesforceProcessor{
		client:                   salesforceHttp,
		log:                      mgr.Logger(),
		res:                      mgr,
		req:                      req,
		binLogCache:              cfg.CacheResource,
		cdcEnabled:               cfg.CDCEnabled,
		cdcObjects:               cfg.CDCObjects,
		cdcTopicName:             cdcTopicName,
		cdcBatchSize:             int32(cfg.CDCBatchSize),
		cdcBufferSize:            int32(cfg.CDCBufferSize),
		cdcReplayPreset:          cfg.CDCReplayPreset,
		pubsubTopic:              cfg.PubSubTopic,
		grpcReconnectBaseDelay:   cfg.GRPCReconnectBaseDelay,
		grpcReconnectMaxDelay:    cfg.GRPCReconnectMaxDelay,
		grpcReconnectMaxAttempts: cfg.GRPCReconnectMaxAttempts,
		grpcShutdownTimeout:      cfg.GRPCShutdownTimeout,
	}, nil
}

// buildCDCTopicName constructs the Pub/Sub API topic name for CDC.
// For specific objects: /data/<Object>ChangeEvent (single object)
// For all objects: /data/ChangeEvents
func buildCDCTopicName(objects []string) string {
	if len(objects) == 0 {
		return "/data/ChangeEvents"
	}
	if len(objects) == 1 {
		return "/data/" + objects[0] + "ChangeEvent"
	}
	// Multiple specific objects: use the combined channel
	// Salesforce requires subscribing to individual channels or the catch-all
	// For simplicity, use catch-all when multiple objects are specified
	return "/data/ChangeEvents"
}

func buildRequest(queryType, query string) (Request, error) {
	var req Request

	switch queryType {
	case "rest":
		req.QueryType = QueryREST
	case "graphql":
		req.QueryType = QueryGraphQL
	default:
		return Request{}, fmt.Errorf("invalid query_type %q: must be \"rest\" or \"graphql\"", queryType)
	}

	if query != "" {
		req.Filter = FilterConfig{Enabled: true, Value: query}
	}

	return req, nil
}

func (s *salesforceProcessor) Process(ctx context.Context, _ *service.Message) (service.MessageBatch, error) {
	return s.Dispatch(ctx, s.req)
}

func (s *salesforceProcessor) Close(ctx context.Context) error {
	if s.grpcClient == nil {
		return nil
	}

	// Drain remaining events for final checkpoint
	remaining := s.grpcClient.DrainBuffer()
	if len(remaining) > 0 {
		var latestReplayID []byte
		for _, evt := range remaining {
			if len(evt.ReplayID) > 0 {
				latestReplayID = evt.ReplayID
			}
		}
		if len(latestReplayID) > 0 {
			state, err := s.loadState(ctx)
			if err == nil {
				if s.pubsubTopic != "" {
					state.PubSubReplayID = latestReplayID
					state.PubSubTopic = s.pubsubTopic
				} else {
					state.CDCReplayID = latestReplayID
				}
				if err := s.saveState(ctx, state); err != nil {
					s.log.Errorf("Failed to save final checkpoint: %v", err)
				} else {
					s.log.Infof("Final checkpoint saved (%d drained events)", len(remaining))
				}
			}
		}
	}

	return s.grpcClient.CloseWithTimeout(s.grpcShutdownTimeout)
}

// filterCDCEntity checks if a CDC event entity matches the configured objects.
// Used when subscribing to the catch-all /data/ChangeEvents topic to filter events.
func (s *salesforceProcessor) filterCDCEntity(entityName string) bool {
	if len(s.cdcObjects) == 0 {
		return true // No filter, accept all
	}
	for _, obj := range s.cdcObjects {
		if strings.EqualFold(obj, entityName) {
			return true
		}
	}
	return false
}
