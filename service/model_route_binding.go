package service

import (
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const modelRouteBindingContextKey = "model_route_binding"

type ModelRouteBindingContext struct {
	BindingId     int    `json:"binding_id"`
	Group         string `json:"group"`
	ModelName     string `json:"model_name"`
	ChannelId     int    `json:"channel_id"`
	ChannelName   string `json:"channel_name"`
	ChannelType   int    `json:"channel_type"`
	UpstreamModel string `json:"upstream_model"`
}

func MarkModelRouteBindingUsed(ctx *gin.Context, binding *model.ModelRouteBinding, channel *model.Channel, requestedModel string) {
	if ctx == nil || binding == nil || channel == nil {
		return
	}
	ctx.Set(modelRouteBindingContextKey, ModelRouteBindingContext{
		BindingId:     binding.Id,
		Group:         binding.Group,
		ModelName:     binding.ModelName,
		ChannelId:     channel.Id,
		ChannelName:   channel.Name,
		ChannelType:   channel.Type,
		UpstreamModel: model.ResolveChannelUpstreamModel(channel, requestedModel),
	})
}

func AppendModelRouteBindingAdminInfo(ctx *gin.Context, adminInfo map[string]interface{}) {
	if ctx == nil || adminInfo == nil {
		return
	}
	value, ok := ctx.Get(modelRouteBindingContextKey)
	if !ok {
		return
	}
	route, ok := value.(ModelRouteBindingContext)
	if !ok {
		return
	}
	adminInfo["routing_mode"] = "manual_binding"
	adminInfo[modelRouteBindingContextKey] = route
}

func HasModelRouteBinding(ctx *gin.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Get(modelRouteBindingContextKey)
	return ok
}
