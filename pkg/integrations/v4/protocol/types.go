// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/newrelic/infrastructure-agent/pkg/entity"
	"time"
)

type MetricType string

// Metric type values
const (
	MetricTypeCount   MetricType = "count"
	MetricTypeSummary MetricType = "summary"
	MetricTypeGauge   MetricType = "gauge"
	MetricTypeRate    MetricType = "rate"

	MetricTypePrometheusSummary   MetricType = "prometheus-summary"
	MetricTypePrometheusHistogram MetricType = "prometheus-histogram"
)

const millisSinceJanuaryFirst1978 = 252489600000

type DataV4 struct {
	PluginProtocolVersion
	Integration IntegrationMetadata `json:"integration"`
	DataSets    []Dataset           `json:"data"`
}

type IntegrationMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Dataset struct {
	Common    Common                   `json:"common"`
	Metrics   []Metric                 `json:"metrics"`
	Entity    entity.Fields            `json:"entity"`
	Inventory map[string]InventoryData `json:"inventory"`
	Events    []EventData              `json:"events"`
}

type Common struct {
	Timestamp  *int64                 `json:"timestamp"`
	Interval   *int64                 `json:"interval.ms"`
	Attributes map[string]interface{} `json:"attributes"`
}

type Metric struct {
	Name       string                 `json:"name"`
	Type       MetricType             `json:"type"`
	Timestamp  *int64                 `json:"timestamp"`
	Interval   *int64                 `json:"interval.ms"`
	Attributes map[string]interface{} `json:"attributes"`
	Value      json.RawMessage        `json:"value"`
}

type SummaryValue struct {
	Count float64 `json:"count"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Sum   float64 `json:"sum"`
}

// PrometheusHistogram represents a Prometheus histogram
type PrometheusHistogramValue struct {
	SampleCount *uint64  `json:"sample_count,omitempty"`
	SampleSum   *float64 `json:"sample_sum,omitempty"`
	// Buckets defines the buckets into which observations are counted. Each
	// element in the slice is the upper inclusive bound of a bucket. The
	// values must are sorted in strictly increasing order.
	Buckets []*bucket `json:"buckets,omitempty"`
}

type bucket struct {
	CumulativeCount *float64 `json:"cumulative_count,omitempty"`
	UpperBound      *float64 `json:"upper_bound,omitempty"`
}

// PrometheusSummary represents a Prometheus summary
type PrometheusSummaryValue struct {
	SampleCount float64    `json:"sample_count,omitempty"`
	SampleSum   float64    `json:"sample_sum,omitempty"`
	Quantiles   []quantile `json:"quantiles,omitempty"`
}

type quantile struct {
	Quantile float64 `json:"quantile,omitempty"`
	Value    float64 `json:"value,omitempty"`
}

// PluginDataV1 supports a single data set for a single entity
type PluginDataV1 struct {
	PluginOutputIdentifier
	PluginDataSet
}

// convertToV3 converts the data structure of an external plugin v1 to the
// structure of a plugin v3. The only difference between v1 and v3 is that v3
// consists of an array of data sets & it has cluster and service fields.
func (pv1 *PluginDataV1) convertToV3(dest *PluginDataV3) {
	dest.PluginOutputIdentifier = pv1.PluginOutputIdentifier
	dest.DataSets = []PluginDataSetV3{
		{
			PluginDataSet: pv1.PluginDataSet,
		},
	}
}

// PluginDataV3 supports an array of data sets, each for a different entity.
// It's also valid for protocol V2, protocol v3 only adds service & cluster
type PluginDataV3 struct {
	PluginOutputIdentifier
	DataSets []PluginDataSetV3 `json:"data"`
}

type PluginDataSetV3 struct {
	PluginDataSet
	Cluster string `json:"cluster"`
	Service string `json:"service"`
}

// A collection of data generated by a plugin for a single entity.
// V2 plugins can produce multiple of these, where V1 produces one per execution.
type PluginDataSet struct {
	Entity    entity.Fields            `json:"entity"`
	Metrics   []MetricData             `json:"metrics"`
	Inventory map[string]InventoryData `json:"inventory"`
	Events    []EventData              `json:"events"`
	// this is here for backcompat with the SDK, but is ignored
	AddHostname bool `json:"add_hostname"`
}

// Basic fields which identify the plugin and the version of its output
type PluginOutputIdentifier struct {
	Name               string      `json:"name"`
	RawProtocolVersion interface{} `json:"protocol_version"` // Left open-ended for validation purposes
	IntegrationVersion string      `json:"integration_version"`
	Status             string      `json:"integration_status"`
}

// InventoryData is the data type for inventory data produced by a plugin data source and emitted to the agent's inventory data store
type InventoryData map[string]interface{}

// MetricData is the data type for events produced by a plugin data source and emitted to the agent's metrics data store
type MetricData map[string]interface{}

// EventData is the data type for single shot events
type EventData map[string]interface{}

// NewEventData create a new event data from builder func
func NewEventData(options ...func(EventData)) (EventData, error) {
	e := EventData{
		"eventType": "InfrastructureEvent",
		"category":  "notifications",
	}

	for _, opt := range options {
		opt(e)
	}

	// Validate required field
	if _, ok := e["summary"]; !ok {
		return nil, errors.New("invalid event format: missing required 'summary' field")
	}

	// there are integrations that add the hostname
	// and since backed has a attribute limit
	// we remove it to avoid potential conflict when submitting events
	delete(e, "hostname")

	return e, nil
}

// Builder for NewEventData copying all event fields.
func WithEvents(original EventData) func(EventData) {
	return func(copy EventData) {
		for k, v := range original {
			copy[k] = v
		}
	}
}

// Builder for NewEventData constructor will add 'integrationUser' key
func WithIntegrationUser(value string) func(EventData) {
	return func(copy EventData) {
		copy["integrationUser"] = value
	}
}

// Builder for NewEventData constructor will add 'entityKey' and 'entityID' keys
func WithEntity(e entity.Entity) func(EventData) {
	return func(copy EventData) {
		copy["entityKey"] = e.Key.String()
		copy["entityID"] = e.ID.String()
	}
}

// Builder for NewEventData constructor will add labels with prefix 'label.'
func WithLabels(l map[string]string) func(EventData) {
	return func(copy EventData) {
		for key, value := range l {
			copy[fmt.Sprintf("label.%s", key)] = value
		}
	}
}

// Builder for NewEventData constructor will add attributes
// if already exist in the eventData will add it with prefix 'attr.'
func WithAttributes(a map[string]interface{}) func(EventData) {
	return func(copy EventData) {
		for key, value := range a {
			if _, ok := copy[key]; ok {
				copy[fmt.Sprintf("attr.%s", key)] = value
			} else {
				copy[key] = value
			}
		}
	}
}

// Minimum information to determine plugin protocol
type PluginProtocolVersion struct {
	RawProtocolVersion interface{} `json:"protocol_version"` // Left open-ended for validation purposes
}

func (i InventoryData) SortKey() string {
	if i, ok := i["id"]; ok {
		return i.(string)
	}
	return ""
}

// HasInterval does metric type support interval.
func (t MetricType) HasInterval() bool {
	return t == MetricTypeCount || t == MetricTypeSummary
}

// Converts timestamp to a Time object, accepting timestamps in both
// seconds and milliseconds.
func (m *Metric) Time() time.Time {
	if m.Timestamp == nil {
		return time.Now()
	}

	metricTimestamp := *m.Timestamp

	// We assume that timestamp is in seconds if it's less than
	// January 1st 1978 in milliseconds.
	// We will assume that timestamp is in seconds otherwise.
	// See: https://github.com/timhudson/date-from-num
	if metricTimestamp < millisSinceJanuaryFirst1978 {
		return time.Unix(int64(metricTimestamp), 0)
	} else {
		return time.Unix(0, metricTimestamp*int64(time.Millisecond))
	}
}

func (m *Metric) IntervalDuration() time.Duration {
	if m.Interval == nil {
		return time.Duration(0)
	}

	return time.Duration(*m.Interval * int64(time.Millisecond))
}

func (m *Metric) NumericValue() (float64, error) {
	if m.Type == "gauge" || m.Type == "count" || m.Type == "rate" || m.Type == "cumulative-rate" || m.Type == "cumulative-count" {
		var value float64
		err := json.Unmarshal(m.Value, &value)

		return value, err
	}

	return 0, fmt.Errorf("metric type %v is not gauge or count", m.Type)
}

func (m *Metric) SummaryValue() (SummaryValue, error) {
	if m.Type == "summary" {
		var value SummaryValue
		err := json.Unmarshal(m.Value, &value)

		return value, err
	}

	return SummaryValue{}, fmt.Errorf("metric type %v is not summary", m.Type)
}

func (m *Metric) GetPrometheusSummaryValue() (PrometheusSummaryValue, error) {
	if m.Type == MetricTypePrometheusSummary {
		var value PrometheusSummaryValue
		err := json.Unmarshal(m.Value, &value)

		return value, err
	}

	return PrometheusSummaryValue{}, fmt.Errorf("metric type %v is not prometheus-summary", m.Type)
}

func (m *Metric) GetPrometheusHistogramValue() (PrometheusHistogramValue, error) {
	if m.Type == MetricTypePrometheusHistogram {
		var value PrometheusHistogramValue
		err := json.Unmarshal(m.Value, &value)

		return value, err
	}

	return PrometheusHistogramValue{}, fmt.Errorf("metric type %v is not prometheus-histogram", m.Type)
}

// CopyAttrs returns a (shallow) copy of the passed attrs.
func (m *Metric) CopyAttrs() map[string]interface{} {
	duplicate := make(map[string]interface{}, len(m.Attributes))
	for k, v := range m.Attributes {
		duplicate[k] = v
	}
	return duplicate
}
