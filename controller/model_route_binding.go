package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type saveModelRouteBindingRequest struct {
	Group     string `json:"group"`
	ModelName string `json:"model_name"`
	ChannelId int    `json:"channel_id"`
	Reason    string `json:"reason"`
}

func GetModelRouteBindings(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	group := strings.TrimSpace(c.Query("group"))
	keyword := strings.TrimSpace(c.Query("keyword"))

	items, total, err := model.ListModelRouteBindings(group, keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func GetModelRouteCandidates(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	modelName := strings.TrimSpace(c.Query("model"))
	if group == "" || modelName == "" {
		common.ApiErrorMsg(c, "group and model are required")
		return
	}
	channels, err := model.ListModelRouteCandidateChannels(group, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channels)
}

func SaveModelRouteBinding(c *gin.Context) {
	var req saveModelRouteBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.Group = strings.TrimSpace(req.Group)
	req.ModelName = strings.TrimSpace(req.ModelName)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Group == "" || req.ModelName == "" || req.ChannelId <= 0 {
		common.ApiErrorMsg(c, "group, model_name and channel_id are required")
		return
	}
	channel, err := model.CacheGetChannel(req.ChannelId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Status != common.ChannelStatusEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "selected channel is disabled"})
		return
	}
	if !model.IsChannelEnabledForGroupModel(req.Group, req.ModelName, req.ChannelId) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "selected channel does not support this group/model"})
		return
	}

	userId := c.GetInt("id")
	binding, err := model.UpsertModelRouteBinding(req.Group, req.ModelName, req.ChannelId, req.Reason, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLogWithAdminInfo(userId, model.LogTypeManage,
		fmt.Sprintf("updated model route binding: group=%s model=%s channel=%d", req.Group, req.ModelName, req.ChannelId),
		map[string]interface{}{
			"action":       "model_route_binding.update",
			"group":        req.Group,
			"model":        req.ModelName,
			"channel_id":   req.ChannelId,
			"channel_name": channel.Name,
			"reason":       req.Reason,
		},
	)
	common.ApiSuccess(c, binding)
}

func DisableModelRouteBinding(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	modelName := strings.TrimSpace(c.Query("model"))
	if group == "" || modelName == "" {
		common.ApiErrorMsg(c, "group and model are required")
		return
	}
	userId := c.GetInt("id")
	if err := model.DisableModelRouteBinding(group, modelName, userId); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLogWithAdminInfo(userId, model.LogTypeManage,
		fmt.Sprintf("disabled model route binding: group=%s model=%s", group, modelName),
		map[string]interface{}{
			"action": "model_route_binding.disable",
			"group":  group,
			"model":  modelName,
		},
	)
	common.ApiSuccess(c, nil)
}
