package fal

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestConvertKlingPayload(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Prompt:   "a hero shot",
		Images:   []string{"https://example.com/ref.png"},
		Size:     "16:9",
		Duration: 5,
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "fal-ai/kling-video/o3/pro/reference-to-video",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req, info)
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload["prompt"] != req.Prompt {
		t.Fatalf("prompt = %q", payload["prompt"])
	}
	if payload["aspect_ratio"] != "16:9" {
		t.Fatalf("aspect_ratio = %q", payload["aspect_ratio"])
	}
	if payload["duration"] != 5 {
		t.Fatalf("duration = %#v", payload["duration"])
	}
	images, ok := payload["image_urls"].([]string)
	if !ok || len(images) != 1 || images[0] != req.Images[0] {
		t.Fatalf("image_urls = %#v", payload["image_urls"])
	}
}

func TestParseCompletedStatusWithVideo(t *testing.T) {
	body := []byte(`{"status":"COMPLETED","video":{"url":"https://cdn.example/v.mp4"}}`)
	got, err := (&TaskAdaptor{}).ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if got.Status != "SUCCESS" {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Url != "https://cdn.example/v.mp4" {
		t.Fatalf("url = %q", got.Url)
	}
}

func TestEncodeDecodeTaskID(t *testing.T) {
	modelPath := "fal-ai/sora-2/image-to-video"
	requestID := "req_123"
	gotModel, gotReq := decodeTaskID(encodeTaskID(modelPath, requestID))
	if gotModel != modelPath || gotReq != requestID {
		t.Fatalf("decoded = %q %q", gotModel, gotReq)
	}
}

func TestSubmitResponseUnmarshal(t *testing.T) {
	var resp SubmitResponse
	if err := common.Unmarshal([]byte(`{"request_id":"abc"}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.RequestID != "abc" {
		t.Fatalf("request_id = %q", resp.RequestID)
	}
}
