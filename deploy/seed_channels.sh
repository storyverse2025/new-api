#!/usr/bin/env bash
# deploy/seed_channels.sh
# Idempotently provisions a fresh SV Gateway instance:
#   - Sets system options (groups, self-use mode)
#   - Creates access tokens
#   - Creates upstream channels
#
# Usage:
#   bash deploy/seed_channels.sh
#   GATEWAY_URL=https://gateway.storyverseai.art bash deploy/seed_channels.sh
#
# Reads all secrets from deploy/.env.prod (or from the current env).
# Re-running is safe — existing tokens/channels are skipped.

set -euo pipefail

# ── Load .env.prod if present ────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env.prod"
if [[ -f "${ENV_FILE}" ]]; then
  # Export variables, skip comment lines and blank lines
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
  echo "[seed] Loaded env from ${ENV_FILE}"
else
  echo "[seed] Warning: ${ENV_FILE} not found — relying on existing environment."
fi

# ── Config ───────────────────────────────────────────────────────────────────
GATEWAY_URL="${GATEWAY_URL:-http://localhost:3000}"
GATEWAY_ROOT_USERNAME="${GATEWAY_ROOT_USERNAME:-root}"
FAL_API_KEY="${FAL_API_KEY:-${FAL_KEY:-}}"
COOKIE_JAR="$(mktemp /tmp/sv-seed-cookies.XXXXXX)"
trap 'rm -f "${COOKIE_JAR}"' EXIT

echo "[seed] Gateway URL: ${GATEWAY_URL}"

# ── Validate required keys ───────────────────────────────────────────────────
MISSING=()
for var in \
  TOKENROUTER_API_KEY \
  APIMART_API_KEY \
  BYTEPLUS_ARK_API_KEY \
  SEEDREAM_LITE_ENDPOINT_ID \
  SEEDANCE_20_ENDPOINT_ID \
  MINIMAX_API_KEY \
  FAL_API_KEY; do
  if [[ -z "${!var:-}" ]]; then
    MISSING+=("${var}")
  fi
done

# Need at least one of these auth methods
if [[ -z "${GATEWAY_ROOT_PASSWORD:-}" && -z "${GATEWAY_ROOT_ACCESS_TOKEN:-}" ]]; then
  MISSING+=("GATEWAY_ROOT_PASSWORD or GATEWAY_ROOT_ACCESS_TOKEN")
fi

if [[ ${#MISSING[@]} -gt 0 ]]; then
  echo "[seed] ERROR: The following required env vars are not set:" >&2
  for v in "${MISSING[@]}"; do echo "  - ${v}" >&2; done
  exit 1
fi

# ── Auth helpers ─────────────────────────────────────────────────────────────
json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

# Build the common curl options (auth header or cookie jar)
AUTH_ARGS=()
if [[ -n "${GATEWAY_ROOT_ACCESS_TOKEN:-}" ]]; then
  echo "[seed] Authenticating via GATEWAY_ROOT_ACCESS_TOKEN"
  AUTH_ARGS=(-H "Authorization: Bearer ${GATEWAY_ROOT_ACCESS_TOKEN}")
else
  echo "[seed] Authenticating via password login as ${GATEWAY_ROOT_USERNAME}"
  LOGIN_BODY="{\"username\":\"$(json_escape "${GATEWAY_ROOT_USERNAME}")\",\"password\":\"$(json_escape "${GATEWAY_ROOT_PASSWORD}")\"}"
  LOGIN_RESP=$(curl -s -w "\n%{http_code}" \
    -c "${COOKIE_JAR}" \
    -X POST "${GATEWAY_URL}/api/user/login" \
    -H "Content-Type: application/json" \
    -H "New-Api-User: 1" \
    -d "${LOGIN_BODY}")
  HTTP_CODE="${LOGIN_RESP##*$'\n'}"
  LOGIN_RESP_BODY="${LOGIN_RESP%$'\n'*}"
  if [[ "${HTTP_CODE}" != "200" ]]; then
    echo "[seed] ERROR: Login failed (HTTP ${HTTP_CODE}). Check GATEWAY_ROOT_PASSWORD." >&2
    exit 1
  fi
  if ! echo "${LOGIN_RESP_BODY}" | grep -q '"success":true'; then
    echo "[seed] ERROR: Login failed. Check GATEWAY_ROOT_USERNAME and GATEWAY_ROOT_PASSWORD." >&2
    echo "[seed] Response: ${LOGIN_RESP_BODY}" >&2
    exit 1
  fi
  AUTH_ARGS=(-b "${COOKIE_JAR}" -H "New-Api-User: 1")
  echo "[seed] Login successful."
fi

# Wrapper: authenticated GET, returns response body
api_get() {
  local path="$1"
  curl -s "${AUTH_ARGS[@]}" "${GATEWAY_URL}${path}"
}

# Wrapper: authenticated PUT, takes JSON body string
api_put() {
  local path="$1"
  local body="$2"
  curl -s -X PUT "${AUTH_ARGS[@]}" \
    -H "Content-Type: application/json" \
    -d "${body}" \
    "${GATEWAY_URL}${path}"
}

api_put_expect_success() {
  local description="$1"
  local path="$2"
  local body="$3"
  local resp
  resp="$(api_put "${path}" "${body}")"
  if ! echo "${resp}" | grep -q '"success":true'; then
    echo "[seed] ERROR: Failed to ${description}: ${resp}" >&2
    exit 1
  fi
  echo "${resp}" | grep -o '"message":"[^"]*"' || true
}

# Wrapper: authenticated POST, takes JSON body string, returns response body
api_post() {
  local path="$1"
  local body="$2"
  curl -s -X POST "${AUTH_ARGS[@]}" \
    -H "Content-Type: application/json" \
    -d "${body}" \
    "${GATEWAY_URL}${path}"
}

# ── 1. Set system options ─────────────────────────────────────────────────────
echo ""
echo "[seed] === Setting system options ==="

# SelfUseModeEnabled — bypasses per-model pricing check (required for channels to work)
echo "[seed] Setting SelfUseModeEnabled=true ..."
api_put_expect_success "set SelfUseModeEnabled" "/api/option/" '{"key":"SelfUseModeEnabled","value":"true"}'

# GroupRatio — register groups with ratio 1
echo "[seed] Setting GroupRatio for sv-monorepo, bragi-canvas ..."
api_put_expect_success "set GroupRatio" "/api/option/" '{"key":"GroupRatio","value":"{\"sv-monorepo\":1,\"bragi-canvas\":1}"}'

# UserUsableGroups — make both groups available to users
echo "[seed] Setting UserUsableGroups ..."
api_put_expect_success "set UserUsableGroups" "/api/option/" '{"key":"UserUsableGroups","value":"{\"sv-monorepo\":\"sv-monorepo\",\"bragi-canvas\":\"bragi-canvas\"}"}'

echo "[seed] System options set."

# ── 2. Create tokens ─────────────────────────────────────────────────────────
echo ""
echo "[seed] === Creating access tokens ==="

# Fetch existing tokens to enable idempotency
EXISTING_TOKENS=$(api_get "/api/token/?p=0&size=100")

create_token_if_missing() {
  local name="$1"
  local body="$2"
  if echo "${EXISTING_TOKENS}" | grep -q "\"name\":\"${name}\""; then
    echo "[seed] Token '${name}' already exists — skipping."
  else
    RESP=$(api_post "/api/token/" "${body}")
    if echo "${RESP}" | grep -q '"success":true'; then
      echo "[seed] Token '${name}' created."
    else
      echo "[seed] WARNING: Failed to create token '${name}': ${RESP}" >&2
    fi
  fi
}

# sv-monorepo-token: unlimited quota, no expiry, group sv-monorepo
create_token_if_missing "sv-monorepo-token" '{
  "name": "sv-monorepo-token",
  "remain_quota": -1,
  "unlimited_quota": true,
  "expired_time": -1,
  "group": "sv-monorepo"
}'

# bragi-canvas-token: unlimited quota, no expiry, group bragi-canvas
create_token_if_missing "bragi-canvas-token" '{
  "name": "bragi-canvas-token",
  "remain_quota": -1,
  "unlimited_quota": true,
  "expired_time": -1,
  "group": "bragi-canvas"
}'

# ── 3. Create channels ────────────────────────────────────────────────────────
echo ""
echo "[seed] === Creating channels ==="

# Fetch existing channels to enable idempotency
EXISTING_CHANNELS=$(api_get "/api/channel/?p=0&size=100")

create_channel_if_missing() {
  local name="$1"
  local body="$2"
  if echo "${EXISTING_CHANNELS}" | grep -q "\"name\":\"${name}\""; then
    echo "[seed] Channel '${name}' already exists — skipping."
  else
    RESP=$(api_post "/api/channel/" "${body}")
    if echo "${RESP}" | grep -q '"success":true'; then
      echo "[seed] Channel '${name}' created."
    else
      echo "[seed] WARNING: Failed to create channel '${name}': ${RESP}" >&2
    fi
  fi
}

# Channel 1: tokenrouter-gpt55 (type=1, OpenAI-compatible)
create_channel_if_missing "tokenrouter-gpt55" "{
  \"mode\": \"single\",
  \"channel\": {
    \"name\": \"tokenrouter-gpt55\",
    \"type\": 1,
    \"key\": \"${TOKENROUTER_API_KEY}\",
    \"base_url\": \"https://api.tokenrouter.com\",
    \"models\": \"openai/gpt-5.5,sv-text-pro\",
    \"model_mapping\": \"{\\\"sv-text-pro\\\":\\\"openai/gpt-5.5\\\"}\",
    \"group\": \"sv-monorepo,bragi-canvas\",
    \"status\": 1
  }
}"

# Channel 2: byteplus-seedream-lite (type=45, VolcEngine/Ark)
create_channel_if_missing "byteplus-seedream-lite" "{
  \"mode\": \"single\",
  \"channel\": {
    \"name\": \"byteplus-seedream-lite\",
    \"type\": 45,
    \"key\": \"${BYTEPLUS_ARK_API_KEY}\",
    \"base_url\": \"https://ark.ap-southeast.bytepluses.com\",
    \"models\": \"${SEEDREAM_LITE_ENDPOINT_ID},sv-image-seedream-lite\",
    \"model_mapping\": \"{\\\"sv-image-seedream-lite\\\":\\\"${SEEDREAM_LITE_ENDPOINT_ID}\\\"}\",
    \"group\": \"sv-monorepo,bragi-canvas\",
    \"status\": 1
  }
}"

# Channel 3: byteplus-seedance-2 (type=45, VolcEngine/Ark)
create_channel_if_missing "byteplus-seedance-2" "{
  \"mode\": \"single\",
  \"channel\": {
    \"name\": \"byteplus-seedance-2\",
    \"type\": 45,
    \"key\": \"${BYTEPLUS_ARK_API_KEY}\",
    \"base_url\": \"https://ark.ap-southeast.bytepluses.com\",
    \"models\": \"${SEEDANCE_20_ENDPOINT_ID},sv-video-seedance\",
    \"model_mapping\": \"{\\\"sv-video-seedance\\\":\\\"${SEEDANCE_20_ENDPOINT_ID}\\\"}\",
    \"group\": \"sv-monorepo,bragi-canvas\",
    \"status\": 1
  }
}"

# Channel 4: minimax-music (type=35, MiniMax)
create_channel_if_missing "minimax-music" "{
  \"mode\": \"single\",
  \"channel\": {
    \"name\": \"minimax-music\",
    \"type\": 35,
    \"key\": \"${MINIMAX_API_KEY}\",
    \"base_url\": \"https://api.minimax.io\",
    \"models\": \"music-2.6,sv-music-minimax\",
    \"model_mapping\": \"{\\\"sv-music-minimax\\\":\\\"music-2.6\\\"}\",
    \"group\": \"sv-monorepo,bragi-canvas\",
    \"status\": 1
  }
}"

# Channel 5: apimart-images (type=58, APImart — SV custom adaptor)
create_channel_if_missing "apimart-images" "{
  \"mode\": \"single\",
  \"channel\": {
    \"name\": \"apimart-images\",
    \"type\": 58,
    \"key\": \"${APIMART_API_KEY}\",
    \"base_url\": \"https://api.apimart.ai\",
    \"models\": \"gpt-image-2,gemini-3-pro-image-preview,sv-image-gpt,sv-image-banana-pro\",
    \"model_mapping\": \"{\\\"sv-image-gpt\\\":\\\"gpt-image-2\\\",\\\"sv-image-banana-pro\\\":\\\"gemini-3-pro-image-preview\\\"}\",
    \"group\": \"sv-monorepo,bragi-canvas\",
    \"status\": 1
  }
}"

# Channel 6: fal-media (type=59, fal — audio + async video)
create_channel_if_missing "fal-media" "{
  \"mode\": \"single\",
  \"channel\": {
    \"name\": \"fal-media\",
    \"type\": 59,
    \"key\": \"${FAL_API_KEY}\",
    \"base_url\": \"https://fal.run\",
    \"models\": \"sv-video-kling-fal,fal-ai/kling-video/o3/pro/reference-to-video,sv-video-seedance-fal,fal-ai/seedance-2/reference-to-video,sv-video-sora-fal,fal-ai/sora-2/image-to-video,sv-video-grok-fal,xai/grok-imagine-video/image-to-video,sv-voice-elevenlabs-fal,fal-ai/elevenlabs/tts/eleven-v3,sv-voice-minimax-fal,fal-ai/minimax/speech-2.8-hd,sv-sfx-elevenlabs-fal,fal-ai/elevenlabs/sound-effects/v2\",
    \"model_mapping\": \"{\\\"sv-video-kling-fal\\\":\\\"fal-ai/kling-video/o3/pro/reference-to-video\\\",\\\"sv-video-seedance-fal\\\":\\\"fal-ai/seedance-2/reference-to-video\\\",\\\"sv-video-sora-fal\\\":\\\"fal-ai/sora-2/image-to-video\\\",\\\"sv-video-grok-fal\\\":\\\"xai/grok-imagine-video/image-to-video\\\",\\\"sv-voice-elevenlabs-fal\\\":\\\"fal-ai/elevenlabs/tts/eleven-v3\\\",\\\"sv-voice-minimax-fal\\\":\\\"fal-ai/minimax/speech-2.8-hd\\\",\\\"sv-sfx-elevenlabs-fal\\\":\\\"fal-ai/elevenlabs/sound-effects/v2\\\"}\",
    \"group\": \"sv-monorepo,bragi-canvas\",
    \"status\": 1
  }
}"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "[seed] =============================="
echo "[seed] Seeding complete."
echo "[seed] Run the following to verify the gateway is working:"
echo ""
echo "  # Get a token key from the admin panel and test:"
echo "  curl -s ${GATEWAY_URL}/v1/chat/completions \\"
echo "    -H 'Authorization: Bearer <sv-monorepo-token-key>' \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"model\":\"sv-text-pro\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}'"
echo ""
echo "  # fal audio smoke test:"
echo "  curl -i ${GATEWAY_URL}/v1/audio/speech \\"
echo "    -H 'Authorization: Bearer <sv-monorepo-token-key>' \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"model\":\"sv-voice-minimax-fal\",\"input\":\"ping from local gateway\",\"voice\":\"female-shaonv\"}'"
echo "[seed] =============================="
