package doubao

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func endpointReferences(kind string, count int) []string {
	refs := make([]string, count)
	for i := range refs {
		refs[i] = fmt.Sprintf("https://example.com/%s-%d", kind, i)
	}
	return refs
}

func buildEndpointPayload(t *testing.T, req relaycommon.TaskSubmitReq, origin, upstream string) (*requestPayload, error) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", req)
	info := &relaycommon.RelayInfo{
		OriginModelName: origin,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: upstream, IsModelMapped: true},
	}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	var payload requestPayload
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != upstream {
		t.Fatalf("upstream model = %q, want unchanged %q", payload.Model, upstream)
	}
	return &payload, nil
}

func TestMappedEndpointCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, version := range []string{"2.0", "2.5"} {
		maxImages, maxVideos, maxAudios := 9, 3, 3
		if version == "2.5" {
			maxImages, maxVideos, maxAudios = 30, 10, 10
		}
		cases := []struct {
			name, mode             string
			images, videos, audios int
			wantError              string
		}{
			{"text", "text-to-video", 0, 0, 0, ""},
			{"first frame", "first-frame", 1, 0, 0, ""},
			{"image limit accepted", "image-ref", maxImages, 0, 0, ""},
			{"image overflow rejected", "image-ref", maxImages + 1, 0, 0, fmt.Sprintf("up to %d reference images", maxImages)},
			{"video limit accepted", "video-ref", 0, maxVideos, 0, ""},
			{"video overflow rejected", "video-ref", 0, maxVideos + 1, 0, fmt.Sprintf("up to %d reference videos", maxVideos)},
			{"audio limit accepted", "video-ref", 1, 0, maxAudios, ""},
			{"audio overflow rejected", "video-ref", 1, 0, maxAudios + 1, fmt.Sprintf("up to %d reference audios", maxAudios)},
			{"all references at limits", "video-ref", maxImages, maxVideos, maxAudios, ""},
			{"first last frame", "first-last-frame", 2, 0, 0, ""},
			{"video edit", "video-edit", 0, 1, 0, ""},
			{"video extend", "video-extend", 0, 1, 0, ""},
			{"audio only", "video-ref", 0, 0, 1, ""},
		}
		for _, tc := range cases {
			t.Run(version+"/"+tc.name, func(t *testing.T) {
				wantError := tc.wantError
				if version == "2.0" {
					if tc.mode == "first-last-frame" || tc.mode == "video-edit" || tc.mode == "video-extend" {
						wantError = "does not support " + tc.mode
					}
					if tc.name == "audio only" {
						wantError = "requires image or video reference media"
					}
				}
				model := "sv-seedance-" + version
				req := relaycommon.TaskSubmitReq{Model: model, Prompt: "test", Mode: tc.mode,
					Images: endpointReferences("image", tc.images), Videos: endpointReferences("video", tc.videos), Audios: endpointReferences("audio", tc.audios)}
				payload, err := buildEndpointPayload(t, req, model, "ep-opaque-deployment")
				if wantError != "" {
					if err == nil || !strings.Contains(err.Error(), wantError) {
						t.Fatalf("error = %v, want %q", err, wantError)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if version == "2.5" && (payload.Duration == nil || int(*payload.Duration) != -1 || payload.Resolution != "720p" || payload.Ratio != "adaptive") {
					t.Fatalf("2.5 defaults lost: duration=%v, resolution=%s, ratio=%s", payload.Duration, payload.Resolution, payload.Ratio)
				}
				if version == "2.0" && (payload.Duration != nil || payload.OutputFormat != "") {
					t.Fatal("2.5 defaults leaked into 2.0")
				}
				if tc.mode == "first-last-frame" && (payload.Content[0].Role != "first_frame" || payload.Content[1].Role != "last_frame") {
					t.Fatal("first/last frame roles lost")
				}
			})
		}
	}
}

func TestSeedanceCapabilityModelSelection(t *testing.T) {
	for _, tc := range []struct{ name, request, original, upstream, want string }{
		{"endpoint uses original", "ep-rewritten", "sv-seedance-2.5", "ep-opaque", "sv-seedance-2.5"},
		{"endpoint falls back to request", "sv-seedance-2.5", "", "ep-opaque", "sv-seedance-2.5"},
		{"2.0 endpoint stays 2.0", "sv-seedance-2.0", "sv-seedance-2.0", "ep-opaque", "sv-seedance-2.0"},
		{"canonical mapped 2.5", "custom", "custom", "dreamina-seedance-2-5-260628", "dreamina-seedance-2-5-260628"},
		{"canonical mapped 2.0 remains authoritative", "sv-seedance-2.5", "sv-seedance-2.5", "doubao-seedance-2-0-260128", "doubao-seedance-2-0-260128"},
		{"unmapped request", "sv-seedance-2.5", "", "", "sv-seedance-2.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: tc.original, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.upstream}}
			got := effectiveSeedanceModel(&relaycommon.TaskSubmitReq{Model: tc.request}, info)
			if got != tc.want {
				t.Fatalf("capability model = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMappedSeedance25ParameterValidation(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		metadata   map[string]interface{}
		wantError  string
	}{
		{"edit fixed duration", "video-edit", map[string]interface{}{"duration": 5}, "requires Auto (-1) duration"},
		{"extend fixed ratio", "video-extend", map[string]interface{}{"ratio": "16:9"}, "requires adaptive ratio"},
		{"duration overflow", "video-ref", map[string]interface{}{"duration": 31}, "integer from 4 to 30"},
		{"duration too short", "video-ref", map[string]interface{}{"duration": 3}, "integer from 4 to 30"},
		{"explicit zero duration", "video-ref", map[string]interface{}{"duration": 0}, "integer from 4 to 30"},
		{"invalid resolution", "video-ref", map[string]interface{}{"resolution": "1080p"}, "does not support 1080p"},
		{"invalid format", "video-ref", map[string]interface{}{"output_format": "gif"}, "does not support gif"},
		{"explicit false preserved", "video-edit", map[string]interface{}{"generate_audio": false, "watermark": false, "resolution": "480p", "output_format": "mov"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{Model: "sv-seedance-2.5", Mode: tc.mode, Prompt: "test", Videos: endpointReferences("video", 1), Metadata: tc.metadata}
			p, err := buildEndpointPayload(t, req, req.Model, "ep-opaque")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.GenerateAudio == nil || bool(*p.GenerateAudio) || p.Watermark == nil || bool(*p.Watermark) || p.Resolution != "480p" || p.OutputFormat != "mov" {
				t.Fatal("explicit parameters changed during mapped request conversion")
			}
		})
	}
}
