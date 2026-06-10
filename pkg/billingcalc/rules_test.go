package billingcalc

import (
	"math"
	"testing"
)

func TestFixedImageRule(t *testing.T) {
	result, err := Estimate("fixed_image", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"n":3}`),
		RuleParams: map[string]any{
			"price_per_image": 0.035,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.CostUSD-0.105) > 1e-12 {
		t.Fatalf("cost = %v, want 0.105", result.CostUSD)
	}
	if result.Quota != 52500 {
		t.Fatalf("quota = %d, want 52500", result.Quota)
	}
}

func TestFixedPriceRule(t *testing.T) {
	result, err := Estimate("fixed_price", Context{
		GroupRatio:   1.5,
		QuotaPerUnit: 500000,
		RuleParams: map[string]any{
			"price": 0.02,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.CostUSD-0.02) > 1e-12 {
		t.Fatalf("cost = %v, want 0.02", result.CostUSD)
	}
	if result.Quota != 15000 {
		t.Fatalf("quota = %d, want 15000", result.Quota)
	}
}

func TestPerSecondRuleAudioOn(t *testing.T) {
	result, err := Estimate("per_second", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"duration":5,"metadata":{"generate_audio":true}}`),
		RuleParams: map[string]any{
			"audio_off_price_per_second": 0.112,
			"audio_on_price_per_second":  0.14,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.CostUSD-0.7) > 1e-12 {
		t.Fatalf("cost = %v, want 0.7", result.CostUSD)
	}
	if result.Quota != 350000 {
		t.Fatalf("quota = %d, want 350000", result.Quota)
	}
}

func TestPerSecondRuleResolutionSpecificRate(t *testing.T) {
	result, err := Estimate("per_second", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"duration":5,"resolution":"4k","generate_audio":false}`),
		RuleParams: map[string]any{
			"audio_off_price_per_second":    0.2,
			"audio_off_4k_price_per_second": 0.4,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.CostUSD-2.0) > 1e-12 {
		t.Fatalf("cost = %v, want 2.0", result.CostUSD)
	}
}

func TestPerSecondSettleUsesResponseDuration(t *testing.T) {
	estimated, err := Estimate("per_second", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"duration":5,"resolution":"4k","generate_audio":false}`),
		RuleParams: map[string]any{
			"audio_off_4k_price_per_second": 0.4,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := SettleSnapshot(estimated.Snapshot, Context{
		ResponseBody: []byte(`{"duration":8,"resolution":"4k"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(settled.CostUSD-3.2) > 1e-12 {
		t.Fatalf("cost = %v, want 3.2", settled.CostUSD)
	}
}

func TestPerSecondRuleDurationSeconds(t *testing.T) {
	result, err := Estimate("per_second", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"text":"cinematic whoosh","duration_seconds":2.5}`),
		RuleParams: map[string]any{
			"price_per_second": 0.002,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.CostUSD-0.005) > 1e-12 {
		t.Fatalf("cost = %v, want 0.005", result.CostUSD)
	}
}

func TestPerCharacterRule(t *testing.T) {
	result, err := Estimate("per_character", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"input":"hello 世界"}`),
		RuleParams: map[string]any{
			"price_per_1k_characters": 0.1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.CostUSD-0.0008) > 1e-12 {
		t.Fatalf("cost = %v, want 0.0008", result.CostUSD)
	}
}

func TestBytePlusSeedance2Rule720pNoVideo(t *testing.T) {
	result, err := Estimate("byteplus_seedance2", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"duration":5,"metadata":{"resolution":"720p"}}`),
		RuleParams: map[string]any{
			"fps": 24,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCost := (5.0 * 1280 * 720 * 24 / 1024) / 1_000_000 * 7.0
	if math.Abs(result.CostUSD-wantCost) > 1e-12 {
		t.Fatalf("cost = %v, want %v", result.CostUSD, wantCost)
	}
	if result.Quota != QuotaFromUSD(wantCost, 500000, 1) {
		t.Fatalf("quota = %d, want %d", result.Quota, QuotaFromUSD(wantCost, 500000, 1))
	}
}

func TestBytePlusSeedance2SettleUsesResponseDuration(t *testing.T) {
	estimated, err := Estimate("byteplus_seedance2", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"duration":-1,"metadata":{"resolution":"720p"}}`),
		RuleParams: map[string]any{
			"default_seconds": 5,
			"fps":             24,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	settled, err := SettleSnapshot(estimated.Snapshot, Context{
		ResponseBody: []byte(`{"status":"succeeded","duration":8,"resolution":"720p","framespersecond":24}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCost := (8.0 * 1280 * 720 * 24 / 1024) / 1_000_000 * 7.0
	if math.Abs(settled.CostUSD-wantCost) > 1e-12 {
		t.Fatalf("cost = %v, want %v", settled.CostUSD, wantCost)
	}
	if settled.Quota <= estimated.Quota {
		t.Fatalf("settled quota = %d, estimated quota = %d", settled.Quota, estimated.Quota)
	}
}

func TestSnapshotApplySettlementRefreshesEstimate(t *testing.T) {
	// Pre-consume at the default 5s estimate.
	estimated, err := Estimate("byteplus_seedance2", Context{
		GroupRatio:   1,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"duration":-1,"metadata":{"resolution":"720p"}}`),
		RuleParams: map[string]any{
			"default_seconds": 5,
			"fps":             24,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := estimated.Snapshot

	// Upstream actually produced 8s — settle against the real response.
	settled, err := SettleSnapshot(snap, Context{
		ResponseBody: []byte(`{"status":"succeeded","duration":8,"resolution":"720p","framespersecond":24}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Before refresh the snapshot still holds the 5s estimate.
	if snap.EstimatedQuota != estimated.Quota {
		t.Fatalf("pre-refresh snapshot quota = %d, want estimate %d", snap.EstimatedQuota, estimated.Quota)
	}

	snap.ApplySettlement(settled)

	if snap.EstimatedCostUSD != settled.CostUSD {
		t.Fatalf("snapshot cost = %v, want settled %v", snap.EstimatedCostUSD, settled.CostUSD)
	}
	if snap.EstimatedQuota != settled.Quota {
		t.Fatalf("snapshot quota = %d, want settled %d", snap.EstimatedQuota, settled.Quota)
	}
	if got := snap.Params["output_seconds"]; got != settled.Params["output_seconds"] {
		t.Fatalf("snapshot params output_seconds = %v, want %v", got, settled.Params["output_seconds"])
	}
	if len(snap.Breakdown) == 0 || snap.Breakdown[0].CostUSD != settled.CostUSD {
		t.Fatalf("snapshot breakdown not refreshed: %+v", snap.Breakdown)
	}
}

func TestSettleSnapshotUsesStoredParams(t *testing.T) {
	estimated, err := Estimate("fixed_image", Context{
		GroupRatio:   2,
		QuotaPerUnit: 500000,
		RequestBody:  []byte(`{"n":2}`),
		RuleParams: map[string]any{
			"price_per_image": 0.035,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := SettleSnapshot(estimated.Snapshot, Context{})
	if err != nil {
		t.Fatal(err)
	}
	if settled.Quota != estimated.Quota {
		t.Fatalf("settled quota = %d, want %d", settled.Quota, estimated.Quota)
	}
}
