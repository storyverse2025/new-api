package doubao

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// BytePlus Ark asset library proxy. Lets the gateway register reference media
// (image/audio/video) on the client's behalf using the channel's own AK/SK, so
// real-person / live-action Seedance refs can be moderated at registration time and
// referenced as asset://<id> during generation. See SEEDANCE_ASSET_PROXY_WIP.md.

const (
	assetAPIVersion       = "2024-01-01"
	assetServiceName      = "ark"
	assetDefaultRegion    = "ap-southeast-1"
	assetDefaultProject   = "default"
	assetHTTPClientTimout = 30 * time.Second
)

type assetCreds struct {
	accessKey   string
	secretKey   string
	groupID     string
	projectName string
	region      string
}

func assetCredsFromInfo(info *relaycommon.RelayInfo) (assetCreds, error) {
	s := info.ChannelSetting
	creds := assetCreds{
		accessKey:   strings.TrimSpace(s.BytePlusAccessKey),
		secretKey:   strings.TrimSpace(s.BytePlusSecretKey),
		groupID:     strings.TrimSpace(s.BytePlusAssetGroupID),
		projectName: strings.TrimSpace(s.BytePlusAssetProjectName),
		region:      strings.TrimSpace(s.BytePlusAssetRegion),
	}
	if creds.projectName == "" {
		creds.projectName = assetDefaultProject
	}
	if creds.region == "" {
		creds.region = assetDefaultRegion
	}
	if creds.accessKey == "" || creds.secretKey == "" {
		return creds, errors.New("channel is missing BytePlus asset credentials (byteplus_access_key/secret_key)")
	}
	return creds, nil
}

// universal API response envelope.
type assetUniversalResp struct {
	ResponseMetadata struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
			CodeN   int    `json:"CodeN"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result struct {
		Id           string            `json:"Id"`
		Status       string            `json:"Status"`
		FailedReason string            `json:"FailedReason"`
		Error        *assetResultError `json:"Error"`
	} `json:"Result"`
}

type assetResultError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func assetFailedReason(resp *assetUniversalResp) string {
	if resp == nil {
		return ""
	}
	if reason := strings.TrimSpace(resp.Result.FailedReason); reason != "" {
		return reason
	}
	if resp.Result.Error == nil {
		return ""
	}
	code := strings.TrimSpace(resp.Result.Error.Code)
	message := strings.TrimSpace(resp.Result.Error.Message)
	switch {
	case code != "" && message != "":
		return code + ": " + message
	case message != "":
		return message
	default:
		return code
	}
}

func callAssetAPI(creds assetCreds, action string, body any) (*assetUniversalResp, error) {
	payload, err := common.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal asset request failed")
	}
	reqURL, headers := signVolcRequest(creds.accessKey, creds.secretKey, creds.region,
		assetServiceName, action, assetAPIVersion, string(payload))

	httpReq, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: assetHTTPClientTimout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, errors.Wrapf(err, "BytePlus %s request failed", action)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed assetUniversalResp
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, errors.Wrapf(err, "BytePlus %s: unmarshal response failed (body: %s)", action, truncate(string(respBody), 300))
	}
	if e := parsed.ResponseMetadata.Error; e != nil && e.Code != "" {
		return &parsed, errors.Errorf("BytePlus %s: %s — %s", action, e.Code, e.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &parsed, errors.Errorf("BytePlus %s: HTTP %d — %s", action, resp.StatusCode, truncate(string(respBody), 200))
	}
	return &parsed, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func randomAssetName() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// extremely unlikely; fall back to a timestamp-based name
		return fmt.Sprintf("ref-%d", time.Now().UnixNano())
	}
	return "ref-" + hex.EncodeToString(b)
}

// RegisterAsset implements channel.AssetRegistrar. Creates an asset under the
// channel's group with Moderation.Strategy=Skip (the mechanism that lets real-person
// refs become Active and bypass inline generation-time moderation).
func (a *TaskAdaptor) RegisterAsset(_ *gin.Context, info *relaycommon.RelayInfo, req channel.AssetRegisterRequest) (*channel.AssetResult, error) {
	creds, err := assetCredsFromInfo(info)
	if err != nil {
		return nil, err
	}
	if creds.groupID == "" {
		return nil, errors.New("channel is missing byteplus_asset_group_id (asset groups are never auto-created)")
	}
	if strings.TrimSpace(req.URL) == "" {
		return nil, errors.New("asset url is required")
	}
	assetType := req.AssetType
	if assetType == "" {
		assetType = "Image"
	}

	resp, err := callAssetAPI(creds, "CreateAsset", map[string]any{
		"GroupId":     creds.groupID,
		"URL":         req.URL,
		"AssetType":   assetType,
		"Name":        randomAssetName(),
		"ProjectName": creds.projectName,
		"Moderation":  map[string]any{"Strategy": "Skip"},
	})
	if err != nil {
		return nil, err
	}
	if resp.Result.Id == "" {
		return nil, errors.New("BytePlus CreateAsset: no asset Id in response")
	}
	status := resp.Result.Status
	if status == "" {
		status = "Processing"
	}
	return &channel.AssetResult{ID: resp.Result.Id, Status: status, FailedReason: assetFailedReason(resp)}, nil
}

// GetAssetStatus implements channel.AssetRegistrar.
func (a *TaskAdaptor) GetAssetStatus(_ *gin.Context, info *relaycommon.RelayInfo, assetID string) (*channel.AssetResult, error) {
	creds, err := assetCredsFromInfo(info)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(assetID) == "" {
		return nil, errors.New("asset id is required")
	}
	resp, err := callAssetAPI(creds, "GetAsset", map[string]any{
		"Id":          assetID,
		"ProjectName": creds.projectName,
	})
	if err != nil {
		return nil, err
	}
	status := resp.Result.Status
	if status == "" {
		status = "Unknown"
	}
	return &channel.AssetResult{ID: assetID, Status: status, FailedReason: assetFailedReason(resp)}, nil
}
