# StoryVerse — WIP / Backlog

Forward-looking SV work items (kept separate from `SV_CHANGES.md` to avoid merge churn).
Append new items at the bottom. Use `[ ]` open / `[x]` done.

## Backlog

- [ ] **Record task/relay failure reasons** — persist and surface the upstream failure
  reason on failed relay calls and async tasks (e.g. content-moderation /
  sensitive-content rejections), instead of only a generic status. Today the reason
  lives only in the upstream response and has to be fetched manually to diagnose.

- [ ] **Asset proxy: multi-channel-per-group account mismatch** — the BytePlus asset
  proxy (`POST /v1/assets`, `RegisterAsset`) resolves the channel via `Distribute`
  (by `model + group`), so the asset is registered in the account that serves
  generation. **Not a problem today** (Seedance has one channel, `byteplus-seedance-2`),
  but if a group ever has more than one channel serving the same Seedance model (e.g.
  two BytePlus accounts for load-balancing/failover), `Distribute` could pick channel A
  for `/v1/assets` and channel B for generation. Since `asset://` is account-scoped, B
  can't resolve an id created under A → generation fails / mis-bills. Mitigations to
  bake in before that happens:
  1. **Deterministic routing** — ensure (and test) that `(model, group)` maps to a
     stable channel for both endpoints; avoid weighted/random selection diverging
     between the asset call and the generation call.
  2. **Optional `channel_id` pin** — let the client pass `channel_id` to both
     `/v1/assets` and generation to force the same channel/account.
  3. **Client cache key must include the resolved channel/account**, not just the model,
     so a cached `asset_id` is never reused against a different account.
  4. **Validation** — `GetAssetStatus` returning `not found` for a cached id should make
     the client drop the cache and re-register.
