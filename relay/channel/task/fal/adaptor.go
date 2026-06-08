package fal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL  string
	queueURL string
	apiKey   string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if a.baseURL == "" {
		a.baseURL = "https://fal.run"
	}
	a.queueURL = strings.TrimRight(strings.Replace(a.baseURL, "https://fal.run", "https://queue.fal.run", 1), "/")
	if !strings.Contains(a.queueURL, "queue.fal.run") {
		a.queueURL = "https://queue.fal.run"
	}
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	modelPath := strings.TrimLeft(info.UpstreamModelName, "/")
	if modelPath == "" {
		return "", fmt.Errorf("fal: upstream model is required")
	}
	return a.queueURL + "/" + modelPath, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Key "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var submit SubmitResponse
	if err := common.Unmarshal(body, &submit); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", body), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if submit.RequestID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("fal: empty request_id in response: %s", common.LocalLogPreview(string(body))), "empty_request_id", http.StatusBadRequest)
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)

	return encodeTaskID(info.UpstreamModelName, submit.RequestID), body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	modelPath, requestID := decodeTaskID(taskID)
	if modelPath == "" || requestID == "" {
		return nil, fmt.Errorf("invalid fal task_id")
	}
	queueURL := strings.TrimRight(strings.Replace(baseURL, "https://fal.run", "https://queue.fal.run", 1), "/")
	if !strings.Contains(queueURL, "queue.fal.run") {
		queueURL = "https://queue.fal.run"
	}
	statusURL := fmt.Sprintf("%s/%s/requests/%s/status", queueURL, strings.TrimLeft(modelPath, "/"), requestID)
	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Key "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	statusBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	var status StatusResponse
	if err := common.Unmarshal(statusBody, &status); err != nil || status.Status != "COMPLETED" {
		resp.Body = io.NopCloser(bytes.NewReader(statusBody))
		resp.ContentLength = int64(len(statusBody))
		return resp, nil
	}

	resultURL := fmt.Sprintf("%s/%s/requests/%s", queueURL, strings.TrimLeft(modelPath, "/"), requestID)
	resultReq, err := http.NewRequest(http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, err
	}
	resultReq.Header.Set("Authorization", "Key "+key)
	resultReq.Header.Set("Accept", "application/json")
	resultResp, err := client.Do(resultReq)
	if err != nil || resultResp == nil || resultResp.Body == nil {
		resp.Body = io.NopCloser(bytes.NewReader(statusBody))
		resp.ContentLength = int64(len(statusBody))
		return resp, nil
	}
	defer resultResp.Body.Close()
	resultBody, err := io.ReadAll(resultResp.Body)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(statusBody))
		resp.ContentLength = int64(len(statusBody))
		return resp, nil
	}
	var result ResultResponse
	if err := common.Unmarshal(resultBody, &result); err == nil && result.Video != nil {
		status.Video = result.Video
		if merged, err := common.Marshal(status); err == nil {
			resp.Body = io.NopCloser(bytes.NewReader(merged))
			resp.ContentLength = int64(len(merged))
			return resp, nil
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(statusBody))
	resp.ContentLength = int64(len(statusBody))
	return resp, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var status StatusResponse
	if err := common.Unmarshal(respBody, &status); err != nil {
		return nil, errors.Wrap(err, "unmarshal fal status response failed")
	}
	taskInfo := &relaycommon.TaskInfo{}
	switch status.Status {
	case "COMPLETED":
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Progress = taskcommon.ProgressComplete
		if status.Video != nil {
			taskInfo.Url = status.Video.URL
		}
	case "FAILED":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = fmt.Sprintf("%v", status.Error)
		if taskInfo.Reason == "" || taskInfo.Reason == "<nil>" {
			taskInfo.Reason = "fal task failed"
		}
	case "IN_QUEUE":
		taskInfo.Status = model.TaskStatusQueued
		taskInfo.Progress = taskcommon.ProgressQueued
	case "IN_PROGRESS":
		taskInfo.Status = model.TaskStatusInProgress
		taskInfo.Progress = taskcommon.ProgressInProgress
	default:
		taskInfo.Status = model.TaskStatusInProgress
		taskInfo.Progress = taskcommon.ProgressInProgress
	}
	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := originTask.ToOpenAIVideo()
	return common.Marshal(openAIVideo)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (VideoPayload, error) {
	modelPath := info.UpstreamModelName
	payload := VideoPayload{"prompt": req.Prompt}
	if req.Size != "" {
		payload["aspect_ratio"] = req.Size
	}
	if req.Duration > 0 {
		payload["duration"] = req.Duration
	}

	switch {
	case strings.Contains(modelPath, "/kling-video/"):
		if len(req.Images) > 0 {
			payload["image_urls"] = req.Images
		}
		payload["generate_audio"] = true
	case strings.Contains(modelPath, "/seedance-2/"):
		if len(req.Images) > 0 {
			payload["first_frame_image"] = req.Images[0]
		}
		if len(req.Images) > 1 {
			payload["image_urls"] = req.Images[1:]
		}
		if req.Duration > 0 {
			payload["duration"] = req.Duration
		} else {
			payload["duration"] = "auto"
		}
		payload["resolution"] = "720p"
		payload["generate_audio"] = true
	case strings.Contains(modelPath, "/sora-2/"):
		if len(req.Images) > 0 {
			payload["image_url"] = req.Images[0]
		}
	case strings.Contains(modelPath, "/grok-imagine-video/"):
		if len(req.Images) > 0 {
			payload["image_url"] = req.Images[0]
		}
		payload["resolution"] = "720p"
	default:
		if len(req.Images) > 0 {
			payload["image_url"] = req.Images[0]
		}
	}

	if err := taskcommon.UnmarshalMetadata(req.Metadata, &payload); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return payload, nil
}

func encodeTaskID(modelPath, requestID string) string {
	return strings.TrimLeft(modelPath, "/") + "::" + requestID
}

func decodeTaskID(taskID string) (string, string) {
	parts := strings.SplitN(taskID, "::", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
