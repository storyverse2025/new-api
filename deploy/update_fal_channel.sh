#!/usr/bin/env bash
# deploy/update_fal_channel.sh
# Updates the EXISTING "fal" channel in-place to add the models/mappings
# that seed_channels.sh would only create on a fresh instance.
#
# Why this exists: seed_channels.sh is create-if-missing — it SKIPS a channel
# that already exists, so re-running it never adds newly-introduced models to a
# gateway that was seeded earlier. This script patches the live channel instead.
#
# It fetches the current "fal" channel, merges in the new fal video
# models (kling v3 pro, veo 3.1) into its `models` list and `model_mapping`
# (idempotent — existing entries are left untouched), and PUTs it back.
#
# Usage:
#   GATEWAY_URL=https://gateway... GATEWAY_ROOT_PASSWORD=... bash deploy/update_fal_channel.sh
#   # or authenticate with a root access token:
#   GATEWAY_URL=... GATEWAY_ROOT_ACCESS_TOKEN=... bash deploy/update_fal_channel.sh
#
# Reads creds from deploy/.env.prod (like seed_channels.sh) or the environment.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env.prod"
if [[ -f "${ENV_FILE}" ]]; then
  set -a; source "${ENV_FILE}"; set +a
  echo "[update] Loaded env from ${ENV_FILE}"
fi

GATEWAY_URL="${GATEWAY_URL:-http://localhost:3000}"
GATEWAY_ROOT_USERNAME="${GATEWAY_ROOT_USERNAME:-root}"
CHANNEL_NAME="${CHANNEL_NAME:-fal}"
COOKIE_JAR="$(mktemp /tmp/sv-update-cookies.XXXXXX)"
trap 'rm -f "${COOKIE_JAR}"' EXIT

command -v jq >/dev/null || { echo "[update] ERROR: jq is required." >&2; exit 1; }

# New models to ensure are present (alias -> upstream fal path).
declare -a NEW_ALIASES=("sv-kling-3.0" "sv-veo-3.1")
declare -A NEW_MAPPING=(
  ["sv-kling-3.0"]="fal-ai/kling-video/v3/pro"
  ["sv-veo-3.1"]="fal-ai/veo3.1"
)
# Raw upstream paths to also register in the models list.
declare -a NEW_UPSTREAMS=("fal-ai/kling-video/v3/pro" "fal-ai/veo3.1")

# ── Auth (mirrors seed_channels.sh) ───────────────────────────────
json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

AUTH_ARGS=()
if [[ -n "${GATEWAY_ROOT_ACCESS_TOKEN:-}" ]]; then
  echo "[update] Authenticating via GATEWAY_ROOT_ACCESS_TOKEN"
  AUTH_ARGS=(-H "Authorization: Bearer ${GATEWAY_ROOT_ACCESS_TOKEN}" -H "New-Api-User: 1")
elif [[ -n "${GATEWAY_ROOT_PASSWORD:-}" ]]; then
  echo "[update] Authenticating via password login as ${GATEWAY_ROOT_USERNAME}"
  LOGIN_BODY="{\"username\":\"$(json_escape "${GATEWAY_ROOT_USERNAME}")\",\"password\":\"$(json_escape "${GATEWAY_ROOT_PASSWORD}")\"}"
  HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' -c "${COOKIE_JAR}" \
    -X POST "${GATEWAY_URL}/api/user/login" \
    -H "Content-Type: application/json" -H "New-Api-User: 1" -d "${LOGIN_BODY}")
  [[ "${HTTP_CODE}" == "200" ]] || { echo "[update] ERROR: login failed (HTTP ${HTTP_CODE})." >&2; exit 1; }
  AUTH_ARGS=(-b "${COOKIE_JAR}" -H "New-Api-User: 1")
  echo "[update] Login successful."
else
  echo "[update] ERROR: set GATEWAY_ROOT_PASSWORD or GATEWAY_ROOT_ACCESS_TOKEN." >&2
  exit 1
fi

api_get()  { curl -s "${AUTH_ARGS[@]}" "${GATEWAY_URL}$1"; }
api_put()  { curl -s -X PUT "${AUTH_ARGS[@]}" -H "Content-Type: application/json" -d "$2" "${GATEWAY_URL}$1"; }

# ── 1. Find the channel id by name ────────────────────────────────
echo "[update] Looking up channel '${CHANNEL_NAME}' ..."
CHANNELS_JSON="$(api_get "/api/channel/?p=0&size=100")"
CH_ID="$(printf '%s' "${CHANNELS_JSON}" | jq -r --arg n "${CHANNEL_NAME}" '.data.items[]? // .data[]? | select(.name==$n) | .id' | head -1)"
if [[ -z "${CH_ID}" || "${CH_ID}" == "null" ]]; then
  echo "[update] ERROR: channel '${CHANNEL_NAME}' not found." >&2
  exit 1
fi
echo "[update] Found channel id=${CH_ID}"

# ── 2. Fetch full channel object ──────────────────────────────────
CH_JSON="$(api_get "/api/channel/${CH_ID}")"
CH="$(printf '%s' "${CH_JSON}" | jq '.data')"
[[ -n "${CH}" && "${CH}" != "null" ]] || { echo "[update] ERROR: could not read channel detail: ${CH_JSON}" >&2; exit 1; }

# ── 3. Merge models + model_mapping (idempotent) ──────────────────
CUR_MODELS="$(printf '%s' "${CH}" | jq -r '.models // ""')"
CUR_MAPPING="$(printf '%s' "${CH}" | jq -r '.model_mapping // "{}"')"
[[ -z "${CUR_MAPPING}" || "${CUR_MAPPING}" == "null" ]] && CUR_MAPPING="{}"

# Build the merged models CSV.
declare -A have=()
IFS=',' read -ra parts <<< "${CUR_MODELS}"
for m in "${parts[@]}"; do [[ -n "$m" ]] && have["$m"]=1; done
ADD=()
for m in "${NEW_ALIASES[@]}" "${NEW_UPSTREAMS[@]}"; do
  [[ -z "${have[$m]:-}" ]] && { ADD+=("$m"); have["$m"]=1; }
done
if [[ ${#ADD[@]} -gt 0 ]]; then
  EXTRA="$(IFS=','; echo "${ADD[*]}")"
  if [[ -n "${CUR_MODELS}" ]]; then NEW_MODELS="${CUR_MODELS},${EXTRA}"; else NEW_MODELS="${EXTRA}"; fi
else
  NEW_MODELS="${CUR_MODELS}"
fi

# Merge mapping with jq.
MAP_ARGS=()
JQ_SETS="."
for alias in "${!NEW_MAPPING[@]}"; do
  MAP_ARGS+=(--arg "k_${alias//[^a-zA-Z0-9]/_}" "${alias}" --arg "v_${alias//[^a-zA-Z0-9]/_}" "${NEW_MAPPING[$alias]}")
  key="k_${alias//[^a-zA-Z0-9]/_}"; val="v_${alias//[^a-zA-Z0-9]/_}"
  JQ_SETS="${JQ_SETS} | (.[\$${key}]) //= \$${val}"
done
NEW_MAPPING_JSON="$(printf '%s' "${CUR_MAPPING}" | jq -c "${MAP_ARGS[@]}" "${JQ_SETS}")"

echo "[update] models:  +[${ADD[*]:-none}]"
echo "[update] mapping: ${NEW_MAPPING_JSON}"

# ── 4. PUT the updated channel (full object, models/mapping replaced) ──
UPDATED="$(printf '%s' "${CH}" | jq -c \
  --arg models "${NEW_MODELS}" \
  --argjson mapping "${NEW_MAPPING_JSON}" \
  '.models = $models | .model_mapping = ($mapping | tojson)')"

RESP="$(api_put "/api/channel/" "${UPDATED}")"
if printf '%s' "${RESP}" | grep -q '"success":true'; then
  echo "[update] Channel '${CHANNEL_NAME}' updated successfully."
else
  echo "[update] ERROR: update failed: ${RESP}" >&2
  exit 1
fi
