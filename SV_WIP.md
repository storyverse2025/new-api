# SV WIP

## 2026-06-09 - Seed video model pricing gap

### Finding

`deploy/seed_channels.sh` seeds several public video model aliases, including:

- `sv-video-seedance`
- `sv-video-kling-fal`
- `sv-video-seedance-fal`
- `sv-video-sora-fal`
- `sv-video-grok-fal`
- `sv-video-kling-v3-fal`
- `sv-video-veo-fal`

The seed script currently configures channel/model mappings and groups, and sets `SelfUseModeEnabled=true`, but it does not write matching `ModelPrice` or `ModelRatio` entries for these `sv-video-*` aliases.

Task billing uses `OriginModelName` for pricing, not the mapped upstream model name. Because these aliases do not hit the default `ModelPrice` table, they fall back through `GetModelRatio()` under self-use mode to the default unset ratio of `37.5`.

For per-call task billing, that means the current fallback charge is approximately:

```text
37.5 / 2 * QuotaPerUnit * groupRatio
= 37.5 / 2 * 500000 * 1
= 9,375,000 quota
```

This is effectively about `$18.75` per call with the current `QuotaPerUnit`, and is not a deliberate per-provider price for Kling/Sora/Veo/Grok/Seedance.

### Follow-Up

- Add explicit pricing for the seeded `sv-video-*` aliases, likely via `ModelPrice` in the seed flow or a default code table.
- Decide whether FAL video pricing should be flat per call or include `OtherRatios` for duration, size, quality, or provider-specific variants.
- Consider adding a guard or smoke check that flags seeded models without explicit billing config when `SelfUseModeEnabled` is relied on.
