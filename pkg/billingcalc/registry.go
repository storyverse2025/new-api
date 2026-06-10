package billingcalc

import "sync"

var registry = struct {
	sync.RWMutex
	rules map[string]Rule
}{
	rules: make(map[string]Rule),
}

func Register(rule Rule) {
	if rule == nil || rule.Name() == "" {
		return
	}
	registry.Lock()
	defer registry.Unlock()
	registry.rules[rule.Name()] = rule
}

func Get(name string) (Rule, bool) {
	registry.RLock()
	defer registry.RUnlock()
	rule, ok := registry.rules[name]
	return rule, ok
}

func Estimate(ruleName string, ctx Context) (Result, error) {
	rule, ok := Get(ruleName)
	if !ok {
		return Result{}, ErrRuleNotFound(ruleName)
	}
	ctx.Stage = StageEstimate
	result, err := rule.Estimate(ctx)
	if err != nil {
		return Result{}, err
	}
	if result.RuleName == "" {
		result.RuleName = rule.Name()
	}
	ApplyResultDefaults(ctx, &result)
	return result, nil
}

func SettleSnapshot(snapshot *Snapshot, ctx Context) (Result, error) {
	if snapshot == nil {
		return Result{}, ErrRuleNotFound("")
	}
	rule, ok := Get(snapshot.RuleName)
	if !ok {
		return Result{}, ErrRuleNotFound(snapshot.RuleName)
	}
	ctx.Stage = StageSettle
	ctx.RuleKey = snapshot.RuleKey
	ctx.ChannelID = snapshot.ChannelID
	ctx.ChannelName = snapshot.ChannelName
	ctx.ChannelType = snapshot.ChannelType
	ctx.ModelName = snapshot.ModelName
	ctx.UpstreamModelName = snapshot.UpstreamModelName
	ctx.GroupRatio = snapshot.GroupRatio
	ctx.QuotaPerUnit = snapshot.QuotaPerUnit
	ctx.RuleParams = cloneMap(snapshot.RuleParams)
	ctx.Snapshot = snapshot
	result, err := rule.Settle(ctx)
	if err != nil {
		return Result{}, err
	}
	if result.RuleName == "" {
		result.RuleName = rule.Name()
	}
	ApplyResultDefaults(ctx, &result)
	return result, nil
}
