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

func TestResolveSubmitPath(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		images int
		videos int
		want   string
	}{
		{"veo text-to-video", "fal-ai/veo3.1", 0, 0, "fal-ai/veo3.1/text-to-video"},
		{"veo image-to-video", "fal-ai/veo3.1", 1, 0, "fal-ai/veo3.1/image-to-video"},
		{"kling v3 text-to-video", "fal-ai/kling-video/v3/pro", 0, 0, "fal-ai/kling-video/v3/pro/text-to-video"},
		{"kling v3 image-to-video", "fal-ai/kling-video/v3/pro", 1, 0, "fal-ai/kling-video/v3/pro/image-to-video"},
		// Grok base id routes by input shape across four sub-endpoints.
		{"grok text-to-video", "xai/grok-imagine-video", 0, 0, "xai/grok-imagine-video/text-to-video"},
		{"grok image-to-video", "xai/grok-imagine-video", 1, 0, "xai/grok-imagine-video/image-to-video"},
		{"grok reference-to-video", "xai/grok-imagine-video", 2, 0, "xai/grok-imagine-video/reference-to-video"},
		{"grok extend-video", "xai/grok-imagine-video", 0, 1, "xai/grok-imagine-video/extend-video"},
		{"grok extend wins over images", "xai/grok-imagine-video", 2, 1, "xai/grok-imagine-video/extend-video"},
		// Single-endpoint models are already full paths and pass through.
		{"sora unchanged", "fal-ai/sora-2/image-to-video", 1, 0, "fal-ai/sora-2/image-to-video"},
		{"kling o3 unchanged", "fal-ai/kling-video/o3/pro/reference-to-video", 1, 0, "fal-ai/kling-video/o3/pro/reference-to-video"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &relaycommon.TaskSubmitReq{
				Images: make([]string, tc.images),
				Videos: make([]string, tc.videos),
			}
			if got := resolveSubmitPath(tc.model, req); got != tc.want {
				t.Fatalf("submit = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConvertGrokPayload(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xai/grok-imagine-video"},
	}

	// Multiple images -> reference-to-video uses image_urls (plural).
	refReq := &relaycommon.TaskSubmitReq{
		Prompt: "ref shot",
		Images: []string{"https://example.com/a.png", "https://example.com/b.png"},
	}
	refPayload, err := adaptor.convertToRequestPayload(refReq, info)
	if err != nil {
		t.Fatalf("reference payload error: %v", err)
	}
	if urls, ok := refPayload["image_urls"].([]string); !ok || len(urls) != 2 {
		t.Fatalf("image_urls = %#v", refPayload["image_urls"])
	}

	// A video ref -> extend-video uses video_url.
	extReq := &relaycommon.TaskSubmitReq{
		Prompt: "extend",
		Videos: []string{"https://example.com/in.mp4"},
	}
	extPayload, err := adaptor.convertToRequestPayload(extReq, info)
	if err != nil {
		t.Fatalf("extend payload error: %v", err)
	}
	if extPayload["video_url"] != extReq.Videos[0] {
		t.Fatalf("video_url = %#v", extPayload["video_url"])
	}
}

func TestFalRequestPath(t *testing.T) {
	// Prefer the response_url path fal returns (app-root namespace), which is
	// NOT the submit sub-endpoint.
	got := falRequestPath(SubmitResponse{
		RequestID: "abc",
		ResultURL: "https://queue.fal.run/fal-ai/kling-video/requests/abc",
		StatusURL: "https://queue.fal.run/fal-ai/kling-video/requests/abc/status",
	}, "fal-ai/kling-video/v3/pro/text-to-video", "abc")
	if got != "fal-ai/kling-video/requests/abc" {
		t.Fatalf("from response_url: got %q", got)
	}

	// Fall back to status_url (minus /status) when response_url is absent.
	got = falRequestPath(SubmitResponse{
		RequestID: "abc",
		StatusURL: "https://queue.fal.run/fal-ai/veo/requests/abc/status",
	}, "fal-ai/veo3.1/text-to-video", "abc")
	if got != "fal-ai/veo/requests/abc" {
		t.Fatalf("from status_url: got %q", got)
	}

	// Fall back to reconstruction when fal returns no URLs.
	got = falRequestPath(SubmitResponse{RequestID: "abc"}, "fal-ai/sora-2/image-to-video", "abc")
	if got != "fal-ai/sora-2/image-to-video/requests/abc" {
		t.Fatalf("reconstructed: got %q", got)
	}
}

func TestConvertKlingV3Payload(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Prompt: "a hero shot",
		Images: []string{"https://example.com/first.png", "https://example.com/last.png"},
		Size:   "16:9",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "fal-ai/kling-video/v3/pro",
		},
	}
	payload, err := adaptor.convertToRequestPayload(req, info)
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload["image_url"] != req.Images[0] {
		t.Fatalf("image_url = %#v", payload["image_url"])
	}
	if payload["tail_image_url"] != req.Images[1] {
		t.Fatalf("tail_image_url = %#v", payload["tail_image_url"])
	}
	if _, ok := payload["image_urls"]; ok {
		t.Fatalf("kling v3 should not set image_urls (plural)")
	}
}

func TestConvertVeoPayload(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Prompt: "a sunset",
		Images: []string{"https://example.com/first.png"},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "fal-ai/veo3.1",
		},
	}
	payload, err := adaptor.convertToRequestPayload(req, info)
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload["image_url"] != req.Images[0] {
		t.Fatalf("image_url = %#v", payload["image_url"])
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
