package billingcalc

import (
	"fmt"
	"math"
)

type Stage string

const (
	StageEstimate Stage = "estimate"
	StageSettle   Stage = "settle"
)

type Context struct {
	Stage             Stage
	RuleKey           string
	ChannelID         int
	ChannelName       string
	ChannelType       int
	ModelName         string
	UpstreamModelName string
	GroupRatio        float64
	QuotaPerUnit      float64
	RequestBody       []byte
	ResponseBody      []byte
	RuleParams        map[string]any
	Snapshot          *Snapshot
}

type Line struct {
	Name    string  `json:"name"`
	Units   float64 `json:"units,omitempty"`
	RateUSD float64 `json:"rate_usd,omitempty"`
	CostUSD float64 `json:"cost_usd"`
}

type Result struct {
	RuleName  string
	RuleKey   string
	CostUSD   float64
	Quota     int
	Breakdown []Line
	Params    map[string]any
	Snapshot  *Snapshot
}

type Snapshot struct {
	RuleName          string         `json:"rule_name"`
	RuleKey           string         `json:"rule_key,omitempty"`
	ModelName         string         `json:"model_name,omitempty"`
	UpstreamModelName string         `json:"upstream_model_name,omitempty"`
	ChannelID         int            `json:"channel_id,omitempty"`
	ChannelName       string         `json:"channel_name,omitempty"`
	ChannelType       int            `json:"channel_type,omitempty"`
	GroupRatio        float64        `json:"group_ratio"`
	QuotaPerUnit      float64        `json:"quota_per_unit"`
	RuleParams        map[string]any `json:"rule_params,omitempty"`
	Params            map[string]any `json:"params,omitempty"`
	EstimatedCostUSD  float64        `json:"estimated_cost_usd"`
	EstimatedQuota    int            `json:"estimated_quota"`
	Breakdown         []Line         `json:"breakdown,omitempty"`
}

type Rule interface {
	Name() string
	Estimate(ctx Context) (Result, error)
	Settle(ctx Context) (Result, error)
}

func QuotaFromUSD(costUSD, quotaPerUnit, groupRatio float64) int {
	if costUSD <= 0 || quotaPerUnit <= 0 || groupRatio <= 0 {
		return 0
	}
	return int(math.Round(costUSD * quotaPerUnit * groupRatio))
}

func NewSnapshot(ctx Context, result Result) *Snapshot {
	return &Snapshot{
		RuleName:          result.RuleName,
		RuleKey:           result.RuleKey,
		ModelName:         ctx.ModelName,
		UpstreamModelName: ctx.UpstreamModelName,
		ChannelID:         ctx.ChannelID,
		ChannelName:       ctx.ChannelName,
		ChannelType:       ctx.ChannelType,
		GroupRatio:        ctx.GroupRatio,
		QuotaPerUnit:      ctx.QuotaPerUnit,
		RuleParams:        cloneMap(ctx.RuleParams),
		Params:            cloneMap(result.Params),
		EstimatedCostUSD:  result.CostUSD,
		EstimatedQuota:    result.Quota,
		Breakdown:         append([]Line(nil), result.Breakdown...),
	}
}

// ApplySettlement refreshes a snapshot's recorded cost/quota/breakdown/params
// with the settled result. Pre-consume stores the estimate; after async
// settlement (e.g. the upstream reports the real output duration) this lets the
// snapshot — and therefore the billing log detail — reflect the actual bill
// instead of the stale estimate.
func (s *Snapshot) ApplySettlement(result Result) {
	if s == nil {
		return
	}
	s.EstimatedCostUSD = result.CostUSD
	s.EstimatedQuota = result.Quota
	s.Params = cloneMap(result.Params)
	s.Breakdown = append([]Line(nil), result.Breakdown...)
}

func ApplyResultDefaults(ctx Context, result *Result) {
	if result.RuleKey == "" {
		result.RuleKey = ctx.RuleKey
	}
	if result.Quota == 0 && result.CostUSD > 0 {
		result.Quota = QuotaFromUSD(result.CostUSD, ctx.QuotaPerUnit, ctx.GroupRatio)
	}
	if result.Snapshot == nil {
		result.Snapshot = NewSnapshot(ctx, *result)
	}
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func ErrRuleNotFound(name string) error {
	return fmt.Errorf("billing rule %q not found", name)
}
