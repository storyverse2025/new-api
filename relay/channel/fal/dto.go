package fal

type AudioResponse struct {
	Audio    *AudioFile `json:"audio,omitempty"`
	AudioURL string     `json:"audio_url,omitempty"`
	Error    any        `json:"error,omitempty"`
}

type AudioFile struct {
	URL      string `json:"url,omitempty"`
	AudioURL string `json:"audio_url,omitempty"`
}

type ElevenLabsTTSRequest struct {
	Text  string   `json:"text"`
	Voice string   `json:"voice,omitempty"`
	Speed *float64 `json:"speed,omitempty"`
}

type MiniMaxTTSRequest struct {
	Prompt       string                 `json:"prompt"`
	OutputFormat string                 `json:"output_format"`
	VoiceSetting map[string]interface{} `json:"voice_setting,omitempty"`
}

type SoundEffectRequest struct {
	Text            string   `json:"text"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
}
