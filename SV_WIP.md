# StoryVerse — WIP / Backlog

Forward-looking SV work items (kept separate from `SV_CHANGES.md` to avoid merge churn).
Append new items at the bottom. Use `[ ]` open / `[x]` done.

## Backlog

- [ ] **Record task/relay failure reasons** — persist and surface the upstream failure
  reason on failed relay calls and async tasks (e.g. content-moderation /
  sensitive-content rejections), instead of only a generic status. Today the reason
  lives only in the upstream response and has to be fetched manually to diagnose.
