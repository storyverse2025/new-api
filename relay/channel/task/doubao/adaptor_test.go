package doubao

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func seedance25Info() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "dreamina-seedance-2-5-260628",
		},
	}
}

func TestConvertSeedance25FirstLastFrame(t *testing.T) {
	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  "sv-seedance-2.5",
		Prompt: "Move from dawn to dusk",
		Mode:   "first-last-frame",
		Images: []string{"asset://first", "https://relay.example/last.png"},
	}, seedance25Info())
	if err != nil {
		t.Fatal(err)
	}

	if payload.Ratio != "adaptive" {
		t.Fatalf("ratio = %q, want adaptive", payload.Ratio)
	}
	if payload.Resolution != "720p" {
		t.Fatalf("resolution = %q, want 720p", payload.Resolution)
	}
	if payload.Duration == nil || int(*payload.Duration) != -1 {
		t.Fatalf("duration = %v, want -1", payload.Duration)
	}
	if payload.OutputFormat != "mp4" {
		t.Fatalf("output_format = %q, want mp4", payload.OutputFormat)
	}
	if payload.GenerateAudio == nil || !bool(*payload.GenerateAudio) {
		t.Fatalf("generate_audio = %v, want true", payload.GenerateAudio)
	}
	if payload.Watermark == nil || bool(*payload.Watermark) {
		t.Fatalf("watermark = %v, want false", payload.Watermark)
	}
	if got := []string{payload.Content[0].Role, payload.Content[1].Role, payload.Content[2].Type}; fmt.Sprint(got) != "[first_frame last_frame text]" {
		t.Fatalf("content roles/types = %v, want first/last/text", got)
	}
	if payload.Content[0].ImageURL.URL != "asset://first" {
		t.Fatalf("asset url = %q, want asset://first", payload.Content[0].ImageURL.URL)
	}
}

func TestConvertSeedance25MultimodalReferences(t *testing.T) {
	images := make([]string, 30)
	for i := range images {
		images[i] = fmt.Sprintf("https://relay.example/image-%d.png", i)
	}
	audios := make([]string, 10)
	for i := range audios {
		audios[i] = fmt.Sprintf("https://relay.example/audio-%d.mp3", i)
	}
	videos := make([]string, 10)
	for i := range videos {
		videos[i] = fmt.Sprintf("https://relay.example/video-%d.mp4", i)
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  "sv-seedance-2.5",
		Prompt: "Use every reference",
		Mode:   "video-ref",
		Images: images,
		Audios: audios,
		Videos: videos,
		Metadata: map[string]interface{}{
			"duration":       30,
			"ratio":          "21:9",
			"resolution":     "480p",
			"generate_audio": false,
			"output_format":  "mov",
		},
	}, seedance25Info())
	if err != nil {
		t.Fatal(err)
	}

	if len(payload.Content) != 51 {
		t.Fatalf("content length = %d, want 51", len(payload.Content))
	}
	imageCount, audioCount, videoCount := contentReferenceCounts(payload.Content)
	if imageCount != 30 || audioCount != 10 || videoCount != 10 {
		t.Fatalf("counts = image:%d audio:%d video:%d, want 30/10/10", imageCount, audioCount, videoCount)
	}
	if payload.Duration == nil || int(*payload.Duration) != 30 {
		t.Fatalf("duration = %v, want 30", payload.Duration)
	}
	if payload.Ratio != "21:9" || payload.Resolution != "480p" || payload.OutputFormat != "mov" {
		t.Fatalf("params = ratio:%q resolution:%q format:%q", payload.Ratio, payload.Resolution, payload.OutputFormat)
	}
	if payload.GenerateAudio == nil || bool(*payload.GenerateAudio) {
		t.Fatalf("generate_audio = %v, want false", payload.GenerateAudio)
	}
}

func TestConvertSeedance25Validation(t *testing.T) {
	cases := []struct {
		name    string
		req     relaycommon.TaskSubmitReq
		wantErr string
	}{
		{
			name: "too many images",
			req: relaycommon.TaskSubmitReq{
				Model:  "sv-seedance-2.5",
				Prompt: "Too many images",
				Mode:   "image-ref",
				Images: make([]string, 31),
			},
			wantErr: "up to 30 reference images",
		},
		{
			name: "first frame requires adaptive ratio",
			req: relaycommon.TaskSubmitReq{
				Model:  "sv-seedance-2.5",
				Prompt: "Wrong ratio",
				Mode:   "first-frame",
				Images: []string{"https://relay.example/first.png"},
				Metadata: map[string]interface{}{
					"ratio": "16:9",
				},
			},
			wantErr: "requires adaptive ratio",
		},
		{
			name: "video edit requires auto duration",
			req: relaycommon.TaskSubmitReq{
				Model:  "sv-seedance-2.5",
				Prompt: "Edit the video",
				Mode:   "video-edit",
				Videos: []string{"https://relay.example/source.mp4"},
				Metadata: map[string]interface{}{
					"duration": 10,
				},
			},
			wantErr: "requires Auto (-1) duration",
		},
		{
			name: "video ref requires references",
			req: relaycommon.TaskSubmitReq{
				Model:  "sv-seedance-2.5",
				Prompt: "No refs",
				Mode:   "video-ref",
			},
			wantErr: "requires reference media",
		},
		{
			name: "seedance 2.0 keeps old image limit",
			req: relaycommon.TaskSubmitReq{
				Model:  "doubao-seedance-2-0-260128",
				Prompt: "Too many for 2.0",
				Mode:   "image-ref",
				Images: make([]string, 10),
			},
			wantErr: "up to 9 reference images",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.req.Images {
				tc.req.Images[i] = fmt.Sprintf("https://relay.example/image-%d.png", i)
			}
			_, err := (&TaskAdaptor{}).convertToRequestPayload(&tc.req, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestSeedanceTaskEndpoint(t *testing.T) {
	if got := seedanceTaskEndpoint("https://ark.ap-southeast.bytepluses.com", ""); got != "https://ark.ap-southeast.bytepluses.com/api/v3/contents/generations/tasks" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := seedanceTaskEndpoint("https://ark.ap-southeast.bytepluses.com/api/v3/contents/generations/tasks", ""); got != "https://ark.ap-southeast.bytepluses.com/api/v3/contents/generations/tasks" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := seedanceTaskEndpoint("https://ignored.example", "https://gateway.example/custom/tasks///"); got != "https://gateway.example/custom/tasks" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestBuildRequestURLUsesChannelEndpointSetting(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://ark.ap-southeast.bytepluses.com",
			ChannelSetting: dto.ChannelSettings{
				BytePlusSeedanceEndpoint: "https://gateway.example/custom/tasks///",
			},
		},
	})

	got, err := adaptor.BuildRequestURL(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://gateway.example/custom/tasks" {
		t.Fatalf("url = %q, want custom endpoint", got)
	}
}
