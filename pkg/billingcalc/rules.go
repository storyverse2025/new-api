package billingcalc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func init() {
	Register(FixedPriceRule{})
	Register(FixedImageRule{})
	Register(PerSecondRule{})
	Register(BytePlusSeedance2Rule{})
}

type FixedPriceRule struct{}

func (FixedPriceRule) Name() string { return "fixed_price" }

func (r FixedPriceRule) Estimate(ctx Context) (Result, error) {
	price := paramFloat(ctx.RuleParams, "price", paramFloat(ctx.RuleParams, "price_per_call", 0))
	if price <= 0 {
		return Result{}, fmt.Errorf("%s requires price or price_per_call > 0", r.Name())
	}
	return Result{
		RuleName:  r.Name(),
		CostUSD:   price,
		Params:    map[string]any{"units": 1.0},
		Breakdown: []Line{{Name: "call", Units: 1, RateUSD: price, CostUSD: price}},
	}, nil
}

func (r FixedPriceRule) Settle(ctx Context) (Result, error) {
	return r.Estimate(ctx)
}

type FixedImageRule struct{}

func (FixedImageRule) Name() string { return "fixed_image" }

func (r FixedImageRule) Estimate(ctx Context) (Result, error) {
	price := paramFloat(ctx.RuleParams, "price_per_image", 0)
	if price <= 0 {
		return Result{}, fmt.Errorf("%s requires price_per_image > 0", r.Name())
	}
	n := jsonFloat(ctx.RequestBody, 1, "n")
	if n <= 0 {
		n = 1
	}
	cost := price * n
	return Result{
		RuleName: r.Name(),
		CostUSD:  cost,
		Params: map[string]any{
			"n": n,
		},
		Breakdown: []Line{{Name: "image", Units: n, RateUSD: price, CostUSD: cost}},
	}, nil
}

func (r FixedImageRule) Settle(ctx Context) (Result, error) {
	if ctx.Snapshot != nil && len(ctx.Snapshot.Params) > 0 {
		ctx.RequestBody = nil
		ctx.RuleParams = ctx.Snapshot.RuleParams
		n := paramFloat(ctx.Snapshot.Params, "n", 1)
		price := paramFloat(ctx.RuleParams, "price_per_image", 0)
		cost := price * n
		return Result{
			RuleName:  r.Name(),
			CostUSD:   cost,
			Params:    cloneMap(ctx.Snapshot.Params),
			Breakdown: []Line{{Name: "image", Units: n, RateUSD: price, CostUSD: cost}},
		}, nil
	}
	return r.Estimate(ctx)
}

type PerSecondRule struct{}

func (PerSecondRule) Name() string { return "per_second" }

func (r PerSecondRule) Estimate(ctx Context) (Result, error) {
	defaultSeconds := paramFloat(ctx.RuleParams, "default_seconds", 1)
	seconds := jsonFloat(ctx.RequestBody, defaultSeconds, "duration", "seconds", "metadata.duration", "metadata.durationSeconds")
	if seconds <= 0 {
		seconds = defaultSeconds
	}

	audio := jsonBool(ctx.RequestBody, paramBool(ctx.RuleParams, "default_generate_audio", false), "generate_audio", "metadata.generate_audio")
	rate := paramFloat(ctx.RuleParams, "price_per_second", 0)
	if audio {
		rate = paramFloat(ctx.RuleParams, "audio_on_price_per_second", rate)
	} else {
		rate = paramFloat(ctx.RuleParams, "audio_off_price_per_second", rate)
	}
	if rate <= 0 {
		return Result{}, fmt.Errorf("%s requires price_per_second or audio_*_price_per_second > 0", r.Name())
	}

	cost := seconds * rate
	return Result{
		RuleName: r.Name(),
		CostUSD:  cost,
		Params: map[string]any{
			"seconds":        seconds,
			"generate_audio": audio,
		},
		Breakdown: []Line{{Name: "seconds", Units: seconds, RateUSD: rate, CostUSD: cost}},
	}, nil
}

func (r PerSecondRule) Settle(ctx Context) (Result, error) {
	if ctx.Snapshot != nil && len(ctx.Snapshot.Params) > 0 {
		seconds := paramFloat(ctx.Snapshot.Params, "seconds", paramFloat(ctx.RuleParams, "default_seconds", 1))
		audio := paramBool(ctx.Snapshot.Params, "generate_audio", false)
		rate := paramFloat(ctx.RuleParams, "price_per_second", 0)
		if audio {
			rate = paramFloat(ctx.RuleParams, "audio_on_price_per_second", rate)
		} else {
			rate = paramFloat(ctx.RuleParams, "audio_off_price_per_second", rate)
		}
		cost := seconds * rate
		return Result{
			RuleName:  r.Name(),
			CostUSD:   cost,
			Params:    cloneMap(ctx.Snapshot.Params),
			Breakdown: []Line{{Name: "seconds", Units: seconds, RateUSD: rate, CostUSD: cost}},
		}, nil
	}
	return r.Estimate(ctx)
}

type BytePlusSeedance2Rule struct{}

func (BytePlusSeedance2Rule) Name() string { return "byteplus_seedance2" }

func (r BytePlusSeedance2Rule) Estimate(ctx Context) (Result, error) {
	params := seedanceParams(ctx)
	rate := seedanceRatePerMillion(ctx.RuleParams, params.Resolution, params.HasVideoInput, params.Fast)
	if rate <= 0 {
		return Result{}, fmt.Errorf("%s cannot resolve token rate", r.Name())
	}

	tokens := (params.InputVideoSeconds + params.OutputSeconds) * params.Width * params.Height * params.FPS / 1024
	if minTokens := seedanceMinTokens(ctx.RuleParams, params.Resolution, params.HasVideoInput, params.Fast); minTokens > 0 && tokens < minTokens {
		tokens = minTokens
	}
	cost := tokens / 1_000_000 * rate
	lineName := params.Resolution
	if params.Fast {
		lineName += "_fast"
	}
	if params.HasVideoInput {
		lineName += "_video_input"
	} else {
		lineName += "_text_image_input"
	}

	return Result{
		RuleName: r.Name(),
		CostUSD:  cost,
		Params: map[string]any{
			"resolution":          params.Resolution,
			"width":               params.Width,
			"height":              params.Height,
			"fps":                 params.FPS,
			"output_seconds":      params.OutputSeconds,
			"input_video_seconds": params.InputVideoSeconds,
			"has_video_input":     params.HasVideoInput,
			"fast":                params.Fast,
			"tokens":              tokens,
		},
		Breakdown: []Line{{Name: lineName, Units: tokens, RateUSD: rate, CostUSD: cost}},
	}, nil
}

func (r BytePlusSeedance2Rule) Settle(ctx Context) (Result, error) {
	if ctx.Snapshot != nil && len(ctx.Snapshot.Params) > 0 {
		rate := seedanceRatePerMillion(ctx.RuleParams,
			paramString(ctx.Snapshot.Params, "resolution", "720p"),
			paramBool(ctx.Snapshot.Params, "has_video_input", false),
			paramBool(ctx.Snapshot.Params, "fast", false),
		)
		tokens := paramFloat(ctx.Snapshot.Params, "tokens", 0)
		cost := tokens / 1_000_000 * rate
		return Result{
			RuleName:  r.Name(),
			CostUSD:   cost,
			Params:    cloneMap(ctx.Snapshot.Params),
			Breakdown: []Line{{Name: "seedance2", Units: tokens, RateUSD: rate, CostUSD: cost}},
		}, nil
	}
	return r.Estimate(ctx)
}

type seedanceBillingParams struct {
	Resolution        string
	Width             float64
	Height            float64
	FPS               float64
	OutputSeconds     float64
	InputVideoSeconds float64
	HasVideoInput     bool
	Fast              bool
}

func seedanceParams(ctx Context) seedanceBillingParams {
	fps := paramFloat(ctx.RuleParams, "fps", 24)
	outputSeconds := jsonFloat(ctx.RequestBody, paramFloat(ctx.RuleParams, "default_seconds", 5), "duration", "seconds", "metadata.duration", "metadata.durationSeconds")
	if outputSeconds <= 0 {
		outputSeconds = paramFloat(ctx.RuleParams, "default_seconds", 5)
	}
	inputVideoSeconds := jsonFloat(ctx.RequestBody, 0, "input_video_duration", "metadata.input_video_duration", "metadata.inputVideoDuration")
	hasVideo := inputVideoSeconds > 0 || hasVideoInput(ctx.RequestBody)
	if hasVideo && inputVideoSeconds <= 0 {
		inputVideoSeconds = paramFloat(ctx.RuleParams, "default_input_video_seconds", 0)
	}

	resolution := strings.ToLower(jsonString(ctx.RequestBody, paramString(ctx.RuleParams, "default_resolution", "720p"), "resolution", "metadata.resolution", "size"))
	width, height := seedanceResolutionDimensions(ctx.RuleParams, resolution)
	fast := paramBool(ctx.RuleParams, "fast", strings.Contains(strings.ToLower(ctx.UpstreamModelName), "fast"))

	return seedanceBillingParams{
		Resolution:        normalizeResolution(resolution),
		Width:             width,
		Height:            height,
		FPS:               fps,
		OutputSeconds:     outputSeconds,
		InputVideoSeconds: inputVideoSeconds,
		HasVideoInput:     hasVideo,
		Fast:              fast,
	}
}

func hasVideoInput(body []byte) bool {
	if hasAny(body, "video", "input_video", "video_url", "metadata.video", "metadata.input_video", "metadata.video_url") {
		return true
	}
	content := strings.ToLower(gjsonString(body, "metadata.content"))
	return strings.Contains(content, "video_url") || strings.Contains(content, `"type":"video"`) || strings.Contains(content, `"type": "video"`)
}

func gjsonString(body []byte, path string) string {
	return jsonString(body, "", path)
}

func seedanceResolutionDimensions(params map[string]any, resolution string) (float64, float64) {
	if w, h, ok := parseSize(resolution); ok {
		return w, h
	}
	switch normalizeResolution(resolution) {
	case "480p":
		return paramFloat(params, "width_480p", 832), paramFloat(params, "height_480p", 480)
	case "1080p":
		return paramFloat(params, "width_1080p", 1920), paramFloat(params, "height_1080p", 1080)
	default:
		return paramFloat(params, "width_720p", 1280), paramFloat(params, "height_720p", 720)
	}
}

func parseSize(size string) (float64, float64, bool) {
	size = strings.TrimSpace(strings.ToLower(size))
	size = strings.ReplaceAll(size, "*", "x")
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, errW := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	h, errH := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func normalizeResolution(resolution string) string {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if strings.Contains(resolution, "1080") {
		return "1080p"
	}
	if strings.Contains(resolution, "480") {
		return "480p"
	}
	return "720p"
}

func seedanceRatePerMillion(params map[string]any, resolution string, hasVideoInput, fast bool) float64 {
	if fast {
		if hasVideoInput {
			return paramFloat(params, "fast_video_input_rate_per_m_tokens", 3.3)
		}
		return paramFloat(params, "fast_no_video_rate_per_m_tokens", 5.6)
	}
	if normalizeResolution(resolution) == "1080p" {
		if hasVideoInput {
			return paramFloat(params, "standard_1080p_video_input_rate_per_m_tokens", 4.7)
		}
		return paramFloat(params, "standard_1080p_no_video_rate_per_m_tokens", 7.7)
	}
	if hasVideoInput {
		return paramFloat(params, "standard_720p_video_input_rate_per_m_tokens", 4.3)
	}
	return paramFloat(params, "standard_720p_no_video_rate_per_m_tokens", 7.0)
}

func seedanceMinTokens(params map[string]any, resolution string, hasVideoInput, fast bool) float64 {
	if !hasVideoInput {
		return 0
	}
	key := "min_video_input_tokens"
	if fast {
		key = "fast_min_video_input_tokens"
	}
	if normalizeResolution(resolution) == "1080p" {
		key += "_1080p"
	}
	return math.Max(0, paramFloat(params, key, 0))
}
