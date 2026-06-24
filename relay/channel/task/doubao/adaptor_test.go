package doubao

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestConvertToRequestPayloadIncludesReferenceMediaAndAutoDuration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "dreamina-seedance-2-0-260128",
		Prompt: "make a video",
		Images: []string{"asset://image-1"},
		Audios: []string{"asset://audio-1"},
		Videos: []string{"asset://video-1"},
		Metadata: map[string]interface{}{
			"duration":       -1,
			"ratio":          "16:9",
			"resolution":     "480p",
			"generate_audio": true,
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}

	if payload.Duration == nil || int(*payload.Duration) != -1 {
		t.Fatalf("duration = %#v, want -1", payload.Duration)
	}

	if len(payload.Content) != 4 {
		t.Fatalf("content length = %d, want 4", len(payload.Content))
	}

	assertContentItem(t, payload.Content[0], "image_url", "reference_image", "asset://image-1")
	assertContentItem(t, payload.Content[1], "audio_url", "reference_audio", "asset://audio-1")
	assertContentItem(t, payload.Content[2], "video_url", "reference_video", "asset://video-1")
	assertContentItem(t, payload.Content[3], "text", "", "make a video")
}

func assertContentItem(t *testing.T, item ContentItem, itemType, role, urlOrText string) {
	t.Helper()
	if item.Type != itemType {
		t.Fatalf("content type = %q, want %q", item.Type, itemType)
	}
	if item.Role != role {
		t.Fatalf("content role = %q, want %q", item.Role, role)
	}

	switch itemType {
	case "image_url":
		if item.ImageURL == nil || item.ImageURL.URL != urlOrText {
			t.Fatalf("image url = %#v, want %q", item.ImageURL, urlOrText)
		}
	case "audio_url":
		if item.AudioURL == nil || item.AudioURL.URL != urlOrText {
			t.Fatalf("audio url = %#v, want %q", item.AudioURL, urlOrText)
		}
	case "video_url":
		if item.VideoURL == nil || item.VideoURL.URL != urlOrText {
			t.Fatalf("video url = %#v, want %q", item.VideoURL, urlOrText)
		}
	case "text":
		if item.Text != urlOrText {
			t.Fatalf("text = %q, want %q", item.Text, urlOrText)
		}
	default:
		t.Fatalf("unexpected content type %q", itemType)
	}
}
