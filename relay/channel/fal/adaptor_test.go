package fal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func TestConvertMiniMaxAudioRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	speed := 1.2
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "fal-ai/minimax/speech-2.8-hd",
		},
	}

	reader, err := (&Adaptor{}).ConvertAudioRequest(nil, info, dto.AudioRequest{
		Input: "hello",
		Voice: "female-shaonv",
		Speed: &speed,
	})
	if err != nil {
		t.Fatalf("ConvertAudioRequest returned error: %v", err)
	}

	var payload MiniMaxTTSRequest
	if err := common.DecodeJson(reader, &payload); err != nil {
		t.Fatalf("DecodeJson returned error: %v", err)
	}
	if payload.Prompt != "hello" {
		t.Fatalf("prompt = %q", payload.Prompt)
	}
	if payload.OutputFormat != "url" {
		t.Fatalf("output_format = %q", payload.OutputFormat)
	}
	if payload.VoiceSetting["voice_id"] != "female-shaonv" {
		t.Fatalf("voice_setting = %#v", payload.VoiceSetting)
	}
}

func TestFalAudioResponseExtractsURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"audio":{"url":"https://cdn.example/a.mp3"}}`)),
	}
	usage, err := (&Adaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{})
	if err != nil {
		t.Fatalf("DoResponse returned error: %v", err)
	}
	if usage == nil {
		t.Fatalf("usage is nil")
	}
}
