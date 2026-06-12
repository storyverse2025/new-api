#!/usr/bin/env bash
# deploy/update_byteplus_assets.sh
# Patch an EXISTING BytePlus seedance channel in-place to add the asset-library
# credentials in its `setting` — byteplus_access_key / byteplus_secret_key /
# byteplus_asset_group_id — which the /v1/assets proxy needs to register
# real-person / live-action reference media as `asset://`.
#
# Why this exists: seed_channels.sh is create-if-missing — it SKIPS a channel
# that already exists, so a gateway seeded BEFORE the asset-proxy feature never
# got these credentials written into the channel. Re-running the seed won't fix
# it. This script patches the live channel instead (same approach as
# update_fal_channel.sh).
#
# Secrets are NEVER hardcoded — AK/SK/group are read from the environment
# (deploy/.env.prod): BYTEPLUS_ACCESS_KEY, BYTEPLUS_SECRET_KEY, BYTEPLUS_GROUP_ID.
# The channel's Ark Bearer `key` (used for generation) is left untouched: the
# PUT sends an empty key and GORM `Updates()` skips zero-value fields, so the
# existing key is preserved (identical pattern to update_fal_channel.sh).
#
# Usage:
#   bash deploy/update_byteplus_assets.sh
#   CHANNEL_NAME=byteplus bash deploy/update_byteplus_assets.sh
#   GATEWAY_URL=... GATEWAY_ROOT_ACCESS_TOKEN=... bash deploy/update_byteplus_assets.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env.prod"
if [[ -f "${ENV_FILE}" ]]; then
  set -a; source "${ENV_FILE}"; set +a
  echo "[bp-assets] Loaded env from ${ENV_FILE}"
fi

GATEWAY_URL="${GATEWAY_URL:-http://localhost:3000}"
GATEWAY_ROOT_USERNAME="${GATEWAY_ROOT_USERNAME:-root}"
CHANNEL_NAME="${CHANNEL_NAME:-byteplus}"
COOKIE_JAR="$(mktemp /tmp/sv-bpassets-cookies.XXXXXX)"
trap 'rm -f "${COOKIE_JAR}"' EXIT

command -v jq >/dev/null   || { echo "[bp-assets] ERROR: jq is required." >&2; exit 1; }
command -v curl >/dev/null || { echo "[bp-assets] ERROR: curl is required." >&2; exit 1; }

# ── Required credentials (from env; never hardcoded) ──────────────
: "${BYTEPLUS_ACCESS_KEY:?set BYTEPLUS_ACCESS_KEY in deploy/.env.prod}"
: "${BYTEPLUS_SECRET_KEY:?set BYTEPLUS_SECRET_KEY in deploy/.env.prod}"
: "${BYTEPLUS_GROUP_ID:?set BYTEPLUS_GROUP_ID in deploy/.env.prod}"

# ── Auth (mirrors seed_channels.sh / update_fal_channel.sh) ───────
json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
AUTH_ARGS=()
if [[ -n "${GATEWAY_ROOT_ACCESS_TOKEN:-}" ]]; then
  echo "[bp-assets] Authenticating via GATEWAY_ROOT_ACCESS_TOKEN"
  AUTH_ARGS=(-H "Authorization: Bearer ${GATEWAY_ROOT_ACCESS_TOKEN}" -H "New-Api-User: 1")
elif [[ -n "${GATEWAY_ROOT_PASSWORD:-}" ]]; then
  echo "[bp-assets] Authenticating via password login as ${GATEWAY_ROOT_USERNAME}"
  LOGIN_BODY="{\"username\":\"$(json_escape "${GATEWAY_ROOT_USERNAME}")\",\"password\":\"$(json_escape "${GATEWAY_ROOT_PASSWORD}")\"}"
  HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' -c "${COOKIE_JAR}" \
    -X POST "${GATEWAY_URL}/api/user/login" \
    -H "Content-Type: application/json" -H "New-Api-User: 1" -d "${LOGIN_BODY}")
  [[ "${HTTP_CODE}" == "200" ]] || { echo "[bp-assets] ERROR: login failed (HTTP ${HTTP_CODE})." >&2; exit 1; }
  AUTH_ARGS=(-b "${COOKIE_JAR}" -H "New-Api-User: 1")
  echo "[bp-assets] Login successful."
else
  echo "[bp-assets] ERROR: set GATEWAY_ROOT_PASSWORD or GATEWAY_ROOT_ACCESS_TOKEN." >&2
  exit 1
fi

api_get() { curl -s "${AUTH_ARGS[@]}" "${GATEWAY_URL}$1"; }
api_put() { curl -s -X PUT "${AUTH_ARGS[@]}" -H "Content-Type: application/json" --data-binary "$2" "${GATEWAY_URL}$1"; }

# ── 1. Find the channel id by name ────────────────────────────────
echo "[bp-assets] Looking up channel '${CHANNEL_NAME}' ..."
CHANNELS_JSON="$(api_get "/api/channel/?p=0&size=100")"
CH_ID="$(printf '%s' "${CHANNELS_JSON}" | jq -r --arg n "${CHANNEL_NAME}" '.data.items[]? // .data[]? | select(.name==$n) | .id' | head -1)"
[[ -n "${CH_ID}" && "${CH_ID}" != "null" ]] || { echo "[bp-assets] ERROR: channel '${CHANNEL_NAME}' not found." >&2; exit 1; }
echo "[bp-assets] Found channel id=${CH_ID}"

# ── 2. Fetch the full channel object ──────────────────────────────
CH="$(api_get "/api/channel/${CH_ID}" | jq '.data')"
[[ -n "${CH}" && "${CH}" != "null" ]] || { echo "[bp-assets] ERROR: could not read channel detail." >&2; exit 1; }

# ── 3. Build the setting JSON from env (jq handles escaping) ──────
SETTING_JSON="$(jq -cn \
  --arg ak  "${BYTEPLUS_ACCESS_KEY}" \
  --arg sk  "${BYTEPLUS_SECRET_KEY}" \
  --arg gid "${BYTEPLUS_GROUP_ID}" \
  '{byteplus_access_key:$ak, byteplus_secret_key:$sk, byteplus_asset_group_id:$gid}')"

# ── 4. PUT the updated channel ────────────────────────────────────
#   - `setting` stored as a JSON string (matches seed_channels.sh)
#   - `key` blanked so GORM Updates() preserves the existing Ark token
UPDATED="$(printf '%s' "${CH}" | jq -c --argjson s "${SETTING_JSON}" '.key = "" | .setting = ($s | tojson)')"
RESP="$(api_put "/api/channel/" "${UPDATED}")"
if ! printf '%s' "${RESP}" | grep -q '"success":true'; then
  echo "[bp-assets] ERROR: update failed: ${RESP}" >&2
  exit 1
fi
echo "[bp-assets] OK: channel '${CHANNEL_NAME}' (id=${CH_ID}) asset credentials set; Ark key preserved."

# ── 5. Verify (prints key NAMES only, never values) ───────────────
PRESENT="$(api_get "/api/channel/${CH_ID}" | jq -r '.data.setting // "" | (try (fromjson | keys | join(", ")) catch "(empty/unparseable)")')"
echo "[bp-assets] Verify: setting keys now present -> ${PRESENT}"
