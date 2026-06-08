package fal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	baseURL string
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if a.baseURL == "" {
		a.baseURL = "https://fal.run"
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	if model == "" {
		return "", errors.New("fal: model is required")
	}
	return a.baseURL + "/" + strings.TrimLeft(model, "/"), nil
}

func (a *Adaptor) SetupRequestHeader(_ *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	header.Set("Authorization", "Key "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	return nil
}

func (a *Adaptor) ConvertAudioRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != relayconstant.RelayModeAudioSpeech {
		return nil, errors.New("fal: unsupported audio relay mode")
	}
	model := info.UpstreamModelName
	var payload any
	switch {
	case strings.Contains(model, "/elevenlabs/tts/"):
		payload = ElevenLabsTTSRequest{
			Text:  request.Input,
			Voice: request.Voice,
			Speed: request.Speed,
		}
	case strings.Contains(model, "/minimax/"):
		voiceSetting := map[string]interface{}{}
		if request.Voice != "" {
			voiceSetting["voice_id"] = request.Voice
		}
		if request.Speed != nil {
			voiceSetting["speed"] = *request.Speed
		}
		if len(voiceSetting) == 0 {
			voiceSetting = nil
		}
		payload = MiniMaxTTSRequest{
			Prompt:       request.Input,
			OutputFormat: "url",
			VoiceSetting: voiceSetting,
		}
	case strings.Contains(model, "/sound-effects/"):
		meta := struct {
			DurationSeconds *float64 `json:"duration_seconds,omitempty"`
		}{}
		if len(request.Metadata) > 0 {
			if err := common.Unmarshal(request.Metadata, &meta); err != nil {
				return nil, fmt.Errorf("fal: parse sound effect metadata failed: %w", err)
			}
		}
		payload = SoundEffectRequest{Text: request.Input, DurationSeconds: clampDuration(meta.DurationSeconds, 0.5, 22)}
	default:
		payload = map[string]any{"text": request.Input, "voice": request.Voice}
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("fal: chat not supported")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("fal: rerank not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("fal: embedding not supported")
}

func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("fal: image not supported in this channel")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("fal: responses not supported")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, request)
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("fal: gemini not supported")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, types.NewError(fmt.Errorf("fal: read response failed: %w", readErr), types.ErrorCodeReadResponseBodyFailed)
	}
	var falResp AudioResponse
	if unmarshalErr := common.Unmarshal(body, &falResp); unmarshalErr != nil {
		return nil, types.NewError(fmt.Errorf("fal: parse response failed: %w", unmarshalErr), types.ErrorCodeBadResponse)
	}
	audioURL := falResp.AudioURL
	if audioURL == "" && falResp.Audio != nil {
		audioURL = falResp.Audio.URL
		if audioURL == "" {
			audioURL = falResp.Audio.AudioURL
		}
	}
	if audioURL == "" {
		return nil, types.NewError(fmt.Errorf("fal: no audio url in response: %s", common.LocalLogPreview(string(body))), types.ErrorCodeBadResponse)
	}
	c.Redirect(http.StatusFound, audioURL)
	return &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func clampDuration(value *float64, min, max float64) *float64 {
	if value == nil {
		return nil
	}
	v := *value
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return &v
}

var _ channel.Adaptor = (*Adaptor)(nil)
