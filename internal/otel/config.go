package otel

import (
	"encoding/json"
	"errors"
	"strings"
)

// ConfigOverride captures a single field override loaded from persistent
// storage. The DB layer stores each user-edited field as a key/value pair
// in a settings table; a nil (zero) value means "use the env default".
//
// Field names mirror the public Config field names in lowerCamelCase
// (Endpoint, Protocol, SampleRate, ...) so the JSON the UI sends round
// trips 1:1.
type ConfigOverride struct {
	// Enabled tri-state: nil means "follow env", true/false override
	// the env default.
	Enabled *bool `json:"enabled,omitempty"`

	Endpoint          *string  `json:"endpoint,omitempty"`
	Protocol          *string  `json:"protocol,omitempty"`
	Headers           *string  `json:"headers,omitempty"`
	HeadersFile       *string  `json:"headersFile,omitempty"`
	SampleRate        *float64 `json:"sampleRate,omitempty"`
	ProductionOnly    *bool    `json:"productionOnly,omitempty"`
	IncludeNodeSpans  *bool    `json:"includeNodeSpans,omitempty"`
	InjectOutbound    *bool    `json:"injectOutbound,omitempty"`
	AgentsEnabled     *bool    `json:"agentsEnabled,omitempty"`
	AgentsRecordInputs  *bool `json:"agentsRecordInputs,omitempty"`
	AgentsRecordOutputs *bool `json:"agentsRecordOutputs,omitempty"`

	// Source describes where the override came from — "ui" or "api".
	// Filled in by the API layer; not consumed by the loader.
	Source string `json:"source,omitempty"`
}

// Validate returns an error when the override holds values that don't
// pass basic sanity checks. We don't validate Endpoint here (URL
// parsing happens lazily on first export) so the UI can stage
// half-finished changes.
func (o ConfigOverride) Validate() error {
	if o.Protocol != nil {
		p := strings.ToLower(strings.TrimSpace(*o.Protocol))
		if p != "http/protobuf" && p != "http" && p != "grpc" {
			return errors.New("otel: protocol must be http/protobuf or grpc")
		}
	}
	if o.SampleRate != nil {
		if *o.SampleRate < 0 || *o.SampleRate > 1 {
			return errors.New("otel: sampleRate must be between 0 and 1")
		}
	}
	return nil
}

// IsEmpty reports whether the override has no fields set.
func (o ConfigOverride) IsEmpty() bool {
	return o.Enabled == nil && o.Endpoint == nil && o.Protocol == nil &&
		o.Headers == nil && o.HeadersFile == nil && o.SampleRate == nil &&
		o.ProductionOnly == nil && o.IncludeNodeSpans == nil &&
		o.InjectOutbound == nil && o.AgentsEnabled == nil &&
		o.AgentsRecordInputs == nil && o.AgentsRecordOutputs == nil
}

// MarshalOverride is the canonical JSON encoding used by the DB layer
// when storing the override blob in a single TEXT column. Keeping the
// format private means we can change the storage layout later (e.g.
// migrate to one-row-per-field) without breaking on-disk payloads.
func MarshalOverride(o ConfigOverride) ([]byte, error) {
	if o.IsEmpty() {
		return []byte("{}"), nil
	}
	return json.Marshal(o)
}

// UnmarshalOverride parses the JSON form back into an override. A nil
// or empty byte slice decodes to a zero ConfigOverride — interpreted as
// "no override, fall back to env".
func UnmarshalOverride(b []byte) (ConfigOverride, error) {
	var o ConfigOverride
	if len(b) == 0 {
		return o, nil
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return o, err
	}
	return o, nil
}

// MergeConfig returns env augmented with the override. The merge order is
// env -> override, so a non-nil override field always wins. A nil pointer
// is the documented way to say "use the env default", which matches the
// n8n behaviour where empty env vars preserve the prior value.
func MergeConfig(env Config, override ConfigOverride) Config {
	out := env
	if override.Enabled != nil {
		out.Enabled = *override.Enabled
	}
	if override.Endpoint != nil {
		out.Endpoint = strings.TrimSpace(*override.Endpoint)
	}
	if override.Protocol != nil {
		out.Protocol = strings.ToLower(strings.TrimSpace(*override.Protocol))
	}
	if override.Headers != nil {
		out.Headers = strings.TrimSpace(*override.Headers)
	}
	if override.HeadersFile != nil {
		out.HeadersFile = strings.TrimSpace(*override.HeadersFile)
	}
	if override.SampleRate != nil {
		out.SampleRate = *override.SampleRate
	}
	if override.ProductionOnly != nil {
		out.ProductionOnly = *override.ProductionOnly
	}
	if override.IncludeNodeSpans != nil {
		out.IncludeNodeSpans = *override.IncludeNodeSpans
	}
	if override.InjectOutbound != nil {
		out.InjectOutbound = *override.InjectOutbound
	}
	if override.AgentsEnabled != nil {
		out.AgentsEnabled = *override.AgentsEnabled
	}
	if override.AgentsRecordInputs != nil {
		out.AgentsRecordInputs = *override.AgentsRecordInputs
	}
	if override.AgentsRecordOutputs != nil {
		out.AgentsRecordOutputs = *override.AgentsRecordOutputs
	}
	out.ApplyDefaults()
	return out
}

// EffectiveConfig returns the merged config the runtime should use. It is
// the single helper callers should reach for; LoadConfigFromEnv stays
// for tests that don't want to fake a storage layer.
func EffectiveConfig(override ConfigOverride) Config {
	return MergeConfig(LoadConfigFromEnv(), override)
}

// ConfigView is what the GET /api/v1/otel endpoint returns to the UI.
// It contains three layers: the env-derived defaults (so the UI can
// show "what would happen with no override"), the user-edited override
// (so the UI can highlight which fields diverge from defaults), and the
// effective value (for read-only display / debugging).
type ConfigView struct {
	Env       Config        `json:"env"`
	Override  ConfigOverride `json:"override"`
	Effective Config       `json:"effective"`
}
