package billing_setting

import "testing"

func TestResolveBillingRulePrefersChannelModelOverride(t *testing.T) {
	origRules := billingSetting.BillingRule
	origParams := billingSetting.BillingRuleParams
	defer func() {
		billingSetting.BillingRule = origRules
		billingSetting.BillingRuleParams = origParams
	}()

	billingSetting.BillingRule = map[string]string{
		"sv-image":              "fixed_image",
		"channel:7:sv-image":    "channel_fixed_image",
		"channel:7:upstream-id": "channel_upstream",
	}
	billingSetting.BillingRuleParams = map[string]map[string]any{
		"channel:7:sv-image": {"price_per_image": 0.01},
	}

	rule, params, key, ok := ResolveBillingRule(7, "", "sv-image", "upstream-id")
	if !ok {
		t.Fatal("expected rule")
	}
	if key != "channel:7:sv-image" {
		t.Fatalf("key = %s", key)
	}
	if rule != "channel_fixed_image" {
		t.Fatalf("rule = %s", rule)
	}
	if params["price_per_image"] != 0.01 {
		t.Fatalf("params = %#v", params)
	}
}

func TestResolveBillingRuleChannelWildcard(t *testing.T) {
	origRules := billingSetting.BillingRule
	defer func() { billingSetting.BillingRule = origRules }()

	billingSetting.BillingRule = map[string]string{
		"sv-image":    "fixed_image",
		"channel:7:*": "channel_default",
	}

	rule, _, key, ok := ResolveBillingRule(7, "", "sv-image")
	if !ok {
		t.Fatal("expected rule")
	}
	if key != "channel:7:*" {
		t.Fatalf("key = %s", key)
	}
	if rule != "channel_default" {
		t.Fatalf("rule = %s", rule)
	}
}

func TestResolveBillingRuleChannelNameOverride(t *testing.T) {
	origRules := billingSetting.BillingRule
	defer func() { billingSetting.BillingRule = origRules }()

	billingSetting.BillingRule = map[string]string{
		"sv-image":                    "fixed_image",
		"channel_name:fallback:*":     "name_default",
		"channel_name:fallback:other": "name_other",
	}

	rule, _, key, ok := ResolveBillingRule(0, "fallback", "sv-image")
	if !ok {
		t.Fatal("expected rule")
	}
	if key != "channel_name:fallback:*" {
		t.Fatalf("key = %s", key)
	}
	if rule != "name_default" {
		t.Fatalf("rule = %s", rule)
	}
}

func TestResolveBillingModeAndExprPreferChannelOverride(t *testing.T) {
	origModes := billingSetting.BillingMode
	origExprs := billingSetting.BillingExpr
	defer func() {
		billingSetting.BillingMode = origModes
		billingSetting.BillingExpr = origExprs
	}()

	billingSetting.BillingMode = map[string]string{
		"gpt-test":                  BillingModeTieredExpr,
		"channel_name:provider:gpt": BillingModeTieredExpr,
	}
	billingSetting.BillingExpr = map[string]string{
		"gpt-test":                  `tier("model", p * 1 + c * 2)`,
		"channel_name:provider:gpt": `tier("provider", p * 3 + c * 4)`,
	}

	mode, modeKey := ResolveBillingMode(0, "provider", "gpt", "gpt-test")
	if mode != BillingModeTieredExpr {
		t.Fatalf("mode = %s", mode)
	}
	if modeKey != "channel_name:provider:gpt" {
		t.Fatalf("modeKey = %s", modeKey)
	}

	expr, exprKey, ok := ResolveBillingExpr(0, "provider", "gpt", "gpt-test")
	if !ok {
		t.Fatal("expected expr")
	}
	if exprKey != "channel_name:provider:gpt" {
		t.Fatalf("exprKey = %s", exprKey)
	}
	if expr != `tier("provider", p * 3 + c * 4)` {
		t.Fatalf("expr = %s", expr)
	}
}
