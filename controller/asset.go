package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Asset registration proxy for providers with an asset library (currently BytePlus
// Ark / Seedance). The channel is resolved by middleware.Distribute() from the `model`
// field in the request body, so registration uses the SAME account that will serve
// generation. The asset_id cache stays client-side. See
// relay/channel/task/SEEDANCE_ASSET_PROXY_WIP.md.

type assetRegisterBody struct {
	URL       string `json:"url"`
	AssetType string `json:"asset_type"`
}

type assetStatusBody struct {
	ID string `json:"id"`
}

func assetError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// resolveAssetRegistrar builds the relay info for the distributed channel and returns
// the channel's AssetRegistrar capability, or writes an error response and returns nil.
func resolveAssetRegistrar(c *gin.Context) (channel.AssetRegistrar, *relaycommon.RelayInfo) {
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		assetError(c, http.StatusInternalServerError, "gen_relay_info_failed: "+err.Error())
		return nil, nil
	}
	info.InitChannelMeta(c)

	adaptor := relay.GetTaskAdaptor(relay.GetTaskPlatform(c))
	if adaptor == nil {
		assetError(c, http.StatusBadRequest, "invalid api platform for asset registration")
		return nil, nil
	}
	adaptor.Init(info)

	registrar, ok := adaptor.(channel.AssetRegistrar)
	if !ok {
		assetError(c, http.StatusNotImplemented, "asset registration not supported for this model/channel")
		return nil, nil
	}
	return registrar, info
}

// RelayAssetRegister handles POST /v1/assets — registers a reference media item and
// returns its asset id + status.
func RelayAssetRegister(c *gin.Context) {
	var body assetRegisterBody
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		assetError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.URL == "" {
		assetError(c, http.StatusBadRequest, "url is required")
		return
	}

	registrar, info := resolveAssetRegistrar(c)
	if registrar == nil {
		return
	}

	res, err := registrar.RegisterAsset(c, info, channel.AssetRegisterRequest{
		URL:       body.URL,
		AssetType: body.AssetType,
	})
	if err != nil {
		logger.LogError(c, "asset register failed: "+err.Error())
		assetError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, res)
}

// RelayAssetStatus handles POST /v1/assets/status — returns an asset's current status.
func RelayAssetStatus(c *gin.Context) {
	var body assetStatusBody
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		assetError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.ID == "" {
		assetError(c, http.StatusBadRequest, "id is required")
		return
	}

	registrar, info := resolveAssetRegistrar(c)
	if registrar == nil {
		return
	}

	res, err := registrar.GetAssetStatus(c, info, body.ID)
	if err != nil {
		logger.LogError(c, "asset status failed: "+err.Error())
		assetError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, res)
}
