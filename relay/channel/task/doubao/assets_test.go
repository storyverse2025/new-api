package doubao

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestAssetCredsFromInfoDefaults(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
		BytePlusAccessKey:    "ak",
		BytePlusSecretKey:    "sk",
		BytePlusAssetGroupID: "group-1",
	}}}
	creds, err := assetCredsFromInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	if creds.region != assetDefaultRegion {
		t.Fatalf("region default = %q, want %q", creds.region, assetDefaultRegion)
	}
	if creds.projectName != assetDefaultProject {
		t.Fatalf("project default = %q, want %q", creds.projectName, assetDefaultProject)
	}
}

func TestAssetCredsFromInfoMissing(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{}}}
	if _, err := assetCredsFromInfo(info); err == nil {
		t.Fatal("expected error when AK/SK missing")
	}
}

func TestSignVolcRequestShape(t *testing.T) {
	reqURL, headers := signVolcRequest("AKID", "SECRET", "ap-southeast-1", "ark",
		"CreateAsset", "2024-01-01", `{"a":1}`)

	if want := "https://open.volcengineapi.com/?Action=CreateAsset&Version=2024-01-01"; reqURL != want {
		t.Fatalf("url = %q, want %q", reqURL, want)
	}
	auth := headers["Authorization"]
	if !strings.HasPrefix(auth, "HMAC-SHA256 Credential=AKID/") {
		t.Fatalf("authorization prefix wrong: %q", auth)
	}
	if !strings.Contains(auth, "/ap-southeast-1/ark/request") {
		t.Fatalf("credential scope wrong: %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-content-sha256;x-date") {
		t.Fatalf("signed headers wrong: %q", auth)
	}
	for _, h := range []string{"X-Date", "X-Content-Sha256", "Content-Type", "Host"} {
		if headers[h] == "" {
			t.Fatalf("missing header %s", h)
		}
	}
}

func TestAssetFailedReasonUsesResultErrorMessage(t *testing.T) {
	resp := &assetUniversalResp{}
	resp.Result.FailedReason = ""
	resp.Result.Error = &assetResultError{
		Code:    "InputVideoSensitiveContentDetected.PolicyViolation",
		Message: "The request failed because the input video may be related to copyright restrictions.",
	}

	got := assetFailedReason(resp)
	want := "InputVideoSensitiveContentDetected.PolicyViolation: The request failed because the input video may be related to copyright restrictions."
	if got != want {
		t.Fatalf("failed reason = %q, want %q", got, want)
	}
}

func TestAssetFailedReasonPrefersFailedReason(t *testing.T) {
	resp := &assetUniversalResp{}
	resp.Result.FailedReason = "provider explicit reason"
	resp.Result.Error = &assetResultError{
		Code:    "PolicyViolation",
		Message: "fallback reason",
	}

	if got := assetFailedReason(resp); got != "provider explicit reason" {
		t.Fatalf("failed reason = %q, want explicit FailedReason", got)
	}
}
