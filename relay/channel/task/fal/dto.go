package fal

type SubmitResponse struct {
	RequestID string `json:"request_id"`
	StatusURL string `json:"status_url,omitempty"`
	ResultURL string `json:"response_url,omitempty"`
}

type StatusResponse struct {
	Status string     `json:"status"`
	Error  any        `json:"error,omitempty"`
	Logs   []any      `json:"logs,omitempty"`
	Video  *VideoFile `json:"video,omitempty"`
}

type ResultResponse struct {
	Video *VideoFile `json:"video,omitempty"`
	Error any        `json:"error,omitempty"`
}

type VideoFile struct {
	URL string `json:"url,omitempty"`
}

type VideoPayload map[string]any
