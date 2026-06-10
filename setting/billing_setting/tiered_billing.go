package billing_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/samber/lo"
)

const (
	BillingModeRatio       = "ratio"
	BillingModeTieredExpr  = "tiered_expr"
	BillingModeField       = "billing_mode"
	BillingExprField       = "billing_expr"
	BillingRuleField       = "billing_rule"
	BillingRuleParamsField = "billing_rule_params"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr
type BillingSetting struct {
	BillingMode       map[string]string         `json:"billing_mode"`
	BillingExpr       map[string]string         `json:"billing_expr"`
	BillingRule       map[string]string         `json:"billing_rule"`
	BillingRuleParams map[string]map[string]any `json:"billing_rule_params"`
}

var billingSetting = BillingSetting{
	BillingMode:       make(map[string]string),
	BillingExpr:       make(map[string]string),
	BillingRule:       make(map[string]string),
	BillingRuleParams: make(map[string]map[string]any),
}

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	if mode, ok := billingSetting.BillingMode[model]; ok {
		return mode
	}
	return BillingModeRatio
}

func GetBillingExpr(model string) (string, bool) {
	expr, ok := billingSetting.BillingExpr[model]
	return expr, ok
}

func ResolveBillingMode(channelID int, channelName string, modelNames ...string) (mode string, ruleKey string) {
	for _, key := range billingKeys(channelID, channelName, modelNames...) {
		mode, ok := billingSetting.BillingMode[key]
		if ok && mode != "" {
			return mode, key
		}
	}
	return BillingModeRatio, ""
}

func ResolveBillingExpr(channelID int, channelName string, modelNames ...string) (expr string, ruleKey string, ok bool) {
	for _, key := range billingKeys(channelID, channelName, modelNames...) {
		expr, ok := billingSetting.BillingExpr[key]
		if ok && expr != "" {
			return expr, key, true
		}
	}
	return "", "", false
}

func GetBillingRule(key string) (string, bool) {
	rule, ok := billingSetting.BillingRule[key]
	return rule, ok
}

func GetBillingRuleParams(key string) map[string]any {
	params, ok := billingSetting.BillingRuleParams[key]
	if !ok {
		return nil
	}
	return lo.Assign(params)
}

func GetBillingModeCopy() map[string]string {
	return lo.Assign(billingSetting.BillingMode)
}

func GetBillingExprCopy() map[string]string {
	return lo.Assign(billingSetting.BillingExpr)
}

func GetBillingRuleCopy() map[string]string {
	return lo.Assign(billingSetting.BillingRule)
}

func GetBillingRuleParamsCopy() map[string]map[string]any {
	out := make(map[string]map[string]any, len(billingSetting.BillingRuleParams))
	for key, params := range billingSetting.BillingRuleParams {
		out[key] = lo.Assign(params)
	}
	return out
}

func GetPricingSyncData(base map[string]any) map[string]any {
	extra := make(map[string]any, 4)
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	if rules := GetBillingRuleCopy(); len(rules) > 0 {
		extra[BillingRuleField] = rules
	}
	if params := GetBillingRuleParamsCopy(); len(params) > 0 {
		extra[BillingRuleParamsField] = params
	}
	return lo.Assign(base, extra)
}

func ResolveBillingRule(channelID int, channelName string, modelNames ...string) (ruleName string, params map[string]any, ruleKey string, ok bool) {
	for _, key := range billingKeys(channelID, channelName, modelNames...) {
		rule, exists := GetBillingRule(key)
		if !exists || rule == "" {
			continue
		}
		return rule, GetBillingRuleParams(key), key, true
	}
	return "", nil, "", false
}

func billingKeys(channelID int, channelName string, modelNames ...string) []string {
	keys := make([]string, 0, len(modelNames)*3+2)
	if channelID > 0 {
		for _, model := range modelNames {
			if model != "" {
				keys = append(keys, fmt.Sprintf("channel:%d:%s", channelID, model))
			}
		}
		keys = append(keys, fmt.Sprintf("channel:%d:*", channelID))
	}
	if channelName != "" {
		for _, model := range modelNames {
			if model != "" {
				keys = append(keys, fmt.Sprintf("channel_name:%s:%s", channelName, model))
			}
		}
		keys = append(keys, fmt.Sprintf("channel_name:%s:*", channelName))
	}
	for _, model := range modelNames {
		if model != "" {
			keys = append(keys, model)
		}
	}
	return keys
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}
