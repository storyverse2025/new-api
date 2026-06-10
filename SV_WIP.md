# StoryVerse — WIP / Backlog

Forward-looking SV work items (kept separate from `SV_CHANGES.md` to avoid merge churn).
Append new items at the bottom. Use `[ ]` open / `[x]` done.

## Backlog

- [ ] **Record task/relay failure reasons** — persist and surface the upstream failure
  reason on failed relay calls and async tasks (e.g. content-moderation /
  sensitive-content rejections), instead of only a generic status. Today the reason
  lives only in the upstream response and has to be fetched manually to diagnose.

- [ ] **Gateway-side BytePlus asset library (`asset://`) for Seedance reference-to-video**
  — *Deferred; recording the design decision.* The client → gateway contract is plain
  reference-image URLs (`images: [...]`), which the doubao/volcengine task adaptor already
  turns into Ark `content[].image_url`. This is provider-agnostic and matches how the
  monorepo's fal / volcengine seedance paths already pass references (plain URLs).
  `asset://` is a **BytePlus-only optimization**, not a requirement for reference-to-video
  (the monorepo's `byteplus.py` itself falls back to a plain URL when no asset group is
  configured).

  Optional future enhancement: let the BytePlus channel absorb the asset-library flow
  *internally* (transparent to the client) — on submit, upload each reference image via the
  VolcEngine UniversalApi (AK/SK-signed `CreateAsset`), poll `GetAsset` until `Active`
  (≤300s), cache the `asset_id`, and rewrite the payload to `asset://` refs. Upside:
  upload-once/reuse caching, moderation-skip, finetuned-character assets.

  Why deferred (non-trivial): (1) a second auth scheme — VolcEngine AK/SK request signing —
  must be implemented in Go (Ark inference uses a Bearer key; `relay/channel/jimeng/sign.go`
  is a partial reference); (2) it turns a stateless relay submit into a stateful
  create + poll(≤300s) + cache flow; (3) the reuse cache is keyed by business entities
  (character_version / storyboard_frame) that the gateway does not have — it only sees a URL;
  (4) `asset://` is bound to the uploading account + asset group, so with multiple BytePlus
  channels the cache must be per-channel/account and the submit must pin to the uploading
  channel, which conflicts with load-balanced channel selection.

  Related (separate repo `bragi-canvas`, not new-api): `svnewapi.ts buildVideoBody` currently
  drops reference images on the seedance branch (sets only `metadata`). Fix is to send
  `body.images = imageUrls` so the gateway receives them — tracked/fixed independently of the
  asset-library work above.
