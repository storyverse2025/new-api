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
