# Video reference inputs (image / audio / video ref) — capability map & plan

Status of **image-ref / audio-ref / video-ref** input support across the video task
adaptors, measured against how the same upstream providers are used natively in
`bragi-canvas` (the canonical client). Priority: satisfy bragi-canvas first;
storyverse-monorepo-only needs (e.g. Kling `kling-v3-omni` `voice_ids`,
Wan 2.7 `media[]` API) are tracked separately as "later".

## How reference inputs flow

Unified entry struct: `relaycommon.TaskSubmitReq` (`relay/common/relay_info.go`).
Reference media travels as URL arrays:

- `images []string` (`Images`) — image refs / first frame / first-last frame
- `audios []string` (`Audios`) — audio refs (lip-sync / audio-driven)   ← added
- `videos []string` (`Videos`) — video refs / video extend               ← added

`taskcommon.UnmarshalMetadata` JSON-round-trips `metadata` **into the adaptor's
payload struct**, so only fields declared on that struct survive. Exception: the
**fal** adaptor's payload is a `map[string]any` (`VideoPayload`), so any metadata
key passes through verbatim.

## Channels bragi-canvas actually routes through new-api

Per `deploy/seed_channels.sh` (group `bragi-canvas`):

| bragi virtual model | channel (name / type) | adaptor |
|---|---|---|
| `sv-video-seedance` | `byteplus-seedance-2` (type **45** VolcEngine, BytePlus Ark URL) | `doubao` |
| `sv-video-seedance-fal`, `sv-video-kling-fal`, `sv-video-kling-v3-fal`, `sv-video-sora-fal`, `sv-video-grok-fal`, `sv-video-veo-fal` | `fal-media` (type **59**) | `fal` |

Note: channel types **45 (VolcEngine)** and **54 (DoubaoVideo)** share the single
`taskdoubao` adaptor — BytePlus Ark and Volcengine Ark are the same content API.
So "byteplus seedance" is served by the **doubao adaptor**, even though the channel
is named `byteplus-seedance-2` and typed `45`.

## Target spec (from bragi-canvas native providers)

- Seedance — `src/providers/seedance.ts`: `content[]` of
  `{type:image_url, role:reference_image}` (≤9), `{type:audio_url, role:reference_audio}` (≤3),
  `{type:video_url, role:reference_video}` (≤3), then `{type:text}`.
- Veo — `src/providers/veo.ts`: `image` (first frame), `lastFrame` (first-last),
  `referenceImages[]` (≤3, `referenceType:asset`). No audio.
- Grok (via fal) — `src/providers/fal.ts`: `/image-to-video` (`image_url`),
  `/reference-to-video` (`image_urls` multi), `/extend-video` (`video_url`).
- Kling (via fal) — `image_url` + optional `tail_image_url` (first / first-last).

## Gaps & fixes

| # | Channel | Capability gap | Fix | Priority |
|---|---|---|---|---|
| P0 | doubao (seedance) | images sent without `role`; **no audio-ref**, **no video-ref** first-class | build `content[]` with roles from `Images`/`Audios`/`Videos`; detect `Videos` in billing | bragi-critical |
| P1 | fal (grok) | mapping locked to `/image-to-video`; **no multi-image reference-to-video, no video extend** | route by mode → `/reference-to-video` (image_urls) / `/extend-video` (video_url); register base model | bragi |
| later | gemini (veo) | `referenceImages` + `lastFrame` are `// TODO` | implement on `VeoInstance` | storyverse / direct gemini channel (bragi uses fal for Veo) |
| later | ali (wan) | flat wan2.1/2.2/2.5 API; bragi uses wan2.7 `media[]` (driving_audio/reference_voice/first_clip) | new wan2.7 route | not routed via new-api yet |

## Client (bragi-canvas) follow-up — separate repo

`src/providers/svnewapi.ts buildVideoBody` currently sends only `images` (seedance)
or a single `image`. To exercise the new capabilities it must also send
`audios`/`videos` arrays (seedance) and multi-`images` for grok reference mode.
Tracked as a bragi-canvas change, not in this repo.
