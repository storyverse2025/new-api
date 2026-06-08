#!/usr/bin/env bash
# deploy/smoke_test.sh
# Minimal-cost connectivity smoke test for the SV Gateway.
#
# Proves each upstream provider's credentials + routing are wired, using the
# CHEAPEST modality per provider, and exercises the async-video pipeline with
# exactly ONE short video call.
#
# Paid calls (default): 4 tiny (text/image/image/audio) + 1 short video = 5 total.
# Plus free negative tests (bad token / bad model).
#
# Usage:
#   cp deploy/.smoke.env.example deploy/.smoke.env   # then fill GATEWAY_URL + SV_TOKEN
#   bash deploy/smoke_test.sh
#   SKIP_VIDEO=1 bash deploy/smoke_test.sh           # cheap calls only
#
# Reads GATEWAY_URL / SV_TOKEN from deploy/.smoke.env or the environment.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.smoke.env"
if [[ -f "${ENV_FILE}" ]]; then
  set -a; source "${ENV_FILE}"; set +a
fi

GATEWAY_URL="${GATEWAY_URL:-}"
SV_TOKEN="${SV_TOKEN:-}"
SKIP_VIDEO="${SKIP_VIDEO:-0}"
VIDEO_POLL_TIMEOUT="${VIDEO_POLL_TIMEOUT:-360}"
VIDEO_BODY="${VIDEO_BODY:-{\"model\":\"sv-video-seedance\",\"prompt\":\"a red dot\",\"duration\":5,\"resolution\":\"480p\"}}"

GATEWAY_URL="${GATEWAY_URL%/}"

if [[ -z "${GATEWAY_URL}" || -z "${SV_TOKEN}" ]]; then
  echo "ERROR: GATEWAY_URL and SV_TOKEN must be set (fill deploy/.smoke.env)." >&2
  exit 1
fi

PASS=0; FAIL=0
green() { printf '\033[32m%s\033[0m' "$1"; }
red()   { printf '\033[31m%s\033[0m' "$1"; }
ok()   { PASS=$((PASS+1)); echo "  [$(green PASS)] $1"; }
bad()  { FAIL=$((FAIL+1)); echo "  [$(red FAIL)] $1"; }

AUTH=(-H "Authorization: Bearer ${SV_TOKEN}" -H "Content-Type: application/json")

# POST and capture "<body>\n<http_code>"
post() { # $1=path $2=body
  curl -s -w $'\n%{http_code}' "${AUTH[@]}" -X POST "${GATEWAY_URL}$1" -d "$2"
}
get() { # $1=path
  curl -s -w $'\n%{http_code}' "${AUTH[@]}" "${GATEWAY_URL}$1"
}
code_of() { printf '%s' "$1" | tail -n1; }
body_of() { printf '%s' "$1" | sed '$d'; }
# extract first "key":"value" string value
field() { printf '%s' "$1" | grep -oE "\"$2\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/'; }

echo "== SV Gateway smoke test =="
echo "Gateway: ${GATEWAY_URL}"
echo ""

# ── 0. preflight: token + model list ──────────────────────────────
echo "[0] preflight (free)"
R=$(get "/v1/models"); C=$(code_of "$R")
if [[ "$C" == "200" ]]; then ok "GET /v1/models -> 200"; else bad "GET /v1/models -> $C : $(body_of "$R" | head -c 200)"; fi

# ── 1. text (tokenrouter) ─────────────────────────────────────────
echo "[1] text  sv-text-pro  (tokenrouter)"
R=$(post "/v1/chat/completions" '{"model":"sv-text-pro","messages":[{"role":"user","content":"hi"}],"max_tokens":64}')
C=$(code_of "$R"); B=$(body_of "$R")
if [[ "$C" == "200" && "$B" == *'"choices"'* ]]; then ok "200 + choices"; else bad "$C : $(echo "$B" | head -c 300)"; fi

# ── 2. image (byteplus seedream  -> proves Ark key) ───────────────
echo "[2] image  sv-image-seedream-lite  (byteplus/Ark)"
R=$(post "/v1/images/generations" '{"model":"sv-image-seedream-lite","prompt":"a red dot","n":1}')
C=$(code_of "$R"); B=$(body_of "$R")
if [[ "$C" == "200" && ( "$B" == *'"url"'* || "$B" == *'b64_json'* ) ]]; then ok "200 + image"; else bad "$C : $(echo "$B" | head -c 300)"; fi

# ── 3. image (apimart) ────────────────────────────────────────────
echo "[3] image  sv-image-gpt  (apimart)"
R=$(post "/v1/images/generations" '{"model":"sv-image-gpt","prompt":"a red dot","n":1}')
C=$(code_of "$R"); B=$(body_of "$R")
if [[ "$C" == "200" && ( "$B" == *'"url"'* || "$B" == *'b64_json'* ) ]]; then ok "200 + image"; else bad "$C : $(echo "$B" | head -c 300)"; fi

# ── 4. audio (fal  -> proves fal key) ─────────────────────────────
echo "[4] audio  sv-voice-minimax-fal  (fal)"
# fal audio returns 302 -> follow (-L) and download; check we got non-empty audio bytes.
# NOTE: do NOT pass -X POST here. With -d curl already POSTs the first request, and on the
# 302 it correctly switches to GET to fetch the file. -X POST would force POST on the
# redirected file URL too, which the CDN rejects with 405.
TMP_AUDIO="$(mktemp)"
HTTP=$(curl -sL -o "${TMP_AUDIO}" -w '%{http_code}' "${AUTH[@]}" \
  "${GATEWAY_URL}/v1/audio/speech" -d '{"model":"sv-voice-minimax-fal","input":"hi","voice":"female-shaonv"}')
SZ=$(wc -c < "${TMP_AUDIO}" | tr -d ' ')
if [[ "$HTTP" == "200" && "${SZ:-0}" -gt 256 ]]; then ok "200 + ${SZ} bytes audio"; else bad "http=$HTTP size=${SZ}B : $(head -c 200 "${TMP_AUDIO}")"; fi
rm -f "${TMP_AUDIO}"

# ── 5. ONE short async video (seedance) ───────────────────────────
if [[ "${SKIP_VIDEO}" == "1" ]]; then
  echo "[5] video  -> SKIPPED (SKIP_VIDEO=1)"
else
  echo "[5] video  sv-video-seedance  (byteplus, async — the only paid video call)"
  R=$(post "/v1/video/generations" "${VIDEO_BODY}")
  C=$(code_of "$R"); B=$(body_of "$R")
  TASK_ID="$(field "$B" task_id)"; [[ -z "$TASK_ID" ]] && TASK_ID="$(field "$B" request_id)"; [[ -z "$TASK_ID" ]] && TASK_ID="$(field "$B" id)"
  if [[ "$C" == "200" && -n "$TASK_ID" ]]; then
    echo "      submitted, task_id=${TASK_ID}; polling up to ${VIDEO_POLL_TIMEOUT}s ..."
    DEADLINE=$((SECONDS + VIDEO_POLL_TIMEOUT)); DONE=0
    while [[ $SECONDS -lt $DEADLINE ]]; do
      sleep 15
      PR=$(get "/v1/video/generations/${TASK_ID}"); PB=$(body_of "$PR")
      ST="$(field "$PB" status)"
      echo "      status=${ST:-?}"
      shopt -s nocasematch
      if [[ "$PB" == *succeeded* || "$ST" == completed || "$ST" == COMPLETED || "$PB" == *'"video_url"'* ]]; then DONE=1; shopt -u nocasematch; break; fi
      if [[ "$ST" == failed || "$ST" == FAILED || "$PB" == *'"failed"'* ]]; then shopt -u nocasematch; break; fi
      shopt -u nocasematch
    done
    if [[ "$DONE" == "1" ]]; then ok "video completed"; else bad "video not completed in ${VIDEO_POLL_TIMEOUT}s (last: $(echo "$PB" | head -c 200))"; fi
  else
    bad "submit $C : $(echo "$B" | head -c 300)"
  fi
fi

# ── 6. negative tests (free) ──────────────────────────────────────
echo "[6] negative (free)"
R=$(curl -s -w $'\n%{http_code}' -H "Authorization: Bearer sk-INVALID-TOKEN" -H "Content-Type: application/json" \
  -X POST "${GATEWAY_URL}/v1/chat/completions" -d '{"model":"sv-text-pro","messages":[{"role":"user","content":"x"}],"max_tokens":1}')
C=$(code_of "$R"); [[ "$C" == "401" ]] && ok "bad token -> 401" || bad "bad token -> $C (expected 401)"
R=$(post "/v1/chat/completions" '{"model":"sv-does-not-exist","messages":[{"role":"user","content":"x"}],"max_tokens":1}')
C=$(code_of "$R"); [[ "$C" != "200" ]] && ok "unknown model -> $C (non-200 ok)" || bad "unknown model -> 200 (expected error)"

echo ""
echo "== Summary:  $(green "${PASS} passed"),  $([[ $FAIL -gt 0 ]] && red "${FAIL} failed" || echo "0 failed") =="
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
