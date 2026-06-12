package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

type ModelRouteBinding struct {
	Id          int    `json:"id"`
	Group       string `json:"group" gorm:"column:group;size:64;not null;uniqueIndex:idx_model_route_group_model"`
	ModelName   string `json:"model_name" gorm:"size:255;not null;uniqueIndex:idx_model_route_group_model"`
	ChannelId   int    `json:"channel_id" gorm:"not null;index"`
	Enabled     bool   `json:"enabled" gorm:"default:true;index"`
	Reason      string `json:"reason" gorm:"type:text"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
	UpdatedBy   int    `json:"updated_by"`
}

type ModelRouteBindingView struct {
	Group            string             `json:"group"`
	ModelName        string             `json:"model_name"`
	CandidateCount   int64              `json:"candidate_count"`
	Binding          *ModelRouteBinding `json:"binding,omitempty"`
	Channel          *ModelRouteChannel `json:"channel,omitempty"`
	AutomaticChannel *ModelRouteChannel `json:"automatic_channel,omitempty"`
}

type ModelRouteChannel struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	Type          int    `json:"type"`
	Status        int    `json:"status"`
	Priority      int64  `json:"priority"`
	Weight        uint   `json:"weight"`
	UpstreamModel string `json:"upstream_model"`
}

type abilityModelPair struct {
	Group string `gorm:"column:group"`
	Model string `gorm:"column:model"`
}

type routeCandidateRow struct {
	Channel
	AbilityPriority *int64 `gorm:"column:ability_priority"`
	AbilityWeight   uint   `gorm:"column:ability_weight"`
}

func GetEnabledModelRouteBinding(group string, modelName string) (*ModelRouteBinding, error) {
	binding, err := getEnabledModelRouteBindingExact(group, modelName)
	if err != nil || binding != nil {
		return binding, err
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return nil, nil
	}
	return getEnabledModelRouteBindingExact(group, normalized)
}

func getEnabledModelRouteBindingExact(group string, modelName string) (*ModelRouteBinding, error) {
	if group == "" || modelName == "" {
		return nil, nil
	}
	var binding ModelRouteBinding
	err := DB.Where(commonGroupCol+" = ? and model_name = ? and enabled = ?", group, modelName, true).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

func ListModelRouteBindings(group string, keyword string, offset int, limit int) ([]ModelRouteBindingView, int64, error) {
	pairs, err := listAbilityModelPairs(group, keyword, 0, 0)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(pairs))

	pairs, err = listAbilityModelPairs(group, keyword, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]ModelRouteBindingView, 0, len(pairs))
	for _, pair := range pairs {
		view := ModelRouteBindingView{
			Group:          pair.Group,
			ModelName:      pair.Model,
			CandidateCount: CountModelRouteCandidates(pair.Group, pair.Model),
		}
		if binding, err := getEnabledModelRouteBindingExact(pair.Group, pair.Model); err != nil {
			return nil, 0, err
		} else if binding != nil {
			view.Binding = binding
			if channel, err := CacheGetChannel(binding.ChannelId); err == nil && channel != nil {
				view.Channel = BuildModelRouteChannel(channel, pair.Model)
			}
		}
		if view.Binding == nil {
			view.AutomaticChannel = GetAutomaticModelRouteChannel(pair.Group, pair.Model)
		}
		items = append(items, view)
	}

	return items, total, nil
}

func listAbilityModelPairs(group string, keyword string, offset int, limit int) ([]abilityModelPair, error) {
	query := DB.Model(&Ability{}).
		Select(commonGroupCol+", model").
		Where("enabled = ?", true).
		Group(commonGroupCol + ", model").
		Order(commonGroupCol + " ASC").
		Order("model ASC")
	group = strings.TrimSpace(group)
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		query = query.Where("model LIKE ?", "%"+keyword+"%")
	}
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	var pairs []abilityModelPair
	if err := query.Scan(&pairs).Error; err != nil {
		return nil, err
	}
	return pairs, nil
}

func CountModelRouteCandidates(group string, modelName string) int64 {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, modelName, true).
		Count(&count).Error
	if err != nil {
		return 0
	}
	return count
}

func GetAutomaticModelRouteChannel(group string, modelName string) *ModelRouteChannel {
	channels, err := ListModelRouteCandidateChannels(group, modelName)
	if err != nil || len(channels) == 0 {
		return nil
	}
	return &channels[0]
}

func ListModelRouteCandidateChannels(group string, modelName string) ([]ModelRouteChannel, error) {
	var rows []routeCandidateRow
	err := DB.Table("abilities").
		Select("channels.*, abilities.priority as ability_priority, abilities.weight as ability_weight").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities."+commonGroupCol+" = ? and abilities.model = ? and abilities.enabled = ?", group, modelName, true).
		Order("abilities.priority DESC").
		Order("abilities.weight DESC").
		Order("abilities.channel_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	channels := make([]ModelRouteChannel, 0, len(rows))
	for _, row := range rows {
		channel := row.Channel
		if row.AbilityPriority != nil {
			channel.Priority = row.AbilityPriority
		}
		weight := row.AbilityWeight
		channel.Weight = &weight
		channels = append(channels, *BuildModelRouteChannel(&channel, modelName))
	}
	return channels, nil
}

func BuildModelRouteChannel(channel *Channel, modelName string) *ModelRouteChannel {
	if channel == nil {
		return nil
	}
	return &ModelRouteChannel{
		Id:            channel.Id,
		Name:          channel.Name,
		Type:          channel.Type,
		Status:        channel.Status,
		Priority:      channel.GetPriority(),
		Weight:        uint(channel.GetWeight()),
		UpstreamModel: ResolveChannelUpstreamModel(channel, modelName),
	}
}

func ResolveChannelUpstreamModel(channel *Channel, modelName string) string {
	if channel == nil {
		return modelName
	}
	modelMap := make(map[string]string)
	if mapping := channel.GetModelMapping(); mapping != "" && mapping != "{}" {
		if err := common.UnmarshalJsonStr(mapping, &modelMap); err != nil {
			return modelName
		}
	}
	currentModel := modelName
	visited := map[string]bool{currentModel: true}
	for {
		next, ok := modelMap[currentModel]
		if !ok || next == "" {
			break
		}
		if visited[next] {
			break
		}
		visited[next] = true
		currentModel = next
	}
	return currentModel
}

func UpsertModelRouteBinding(group string, modelName string, channelID int, reason string, updatedBy int) (*ModelRouteBinding, error) {
	now := common.GetTimestamp()
	var binding ModelRouteBinding
	err := DB.Where(commonGroupCol+" = ? and model_name = ?", group, modelName).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		binding = ModelRouteBinding{
			Group:       group,
			ModelName:   modelName,
			ChannelId:   channelID,
			Enabled:     true,
			Reason:      reason,
			CreatedTime: now,
			UpdatedTime: now,
			UpdatedBy:   updatedBy,
		}
		return &binding, DB.Create(&binding).Error
	}
	if err != nil {
		return nil, err
	}
	binding.ChannelId = channelID
	binding.Enabled = true
	binding.Reason = reason
	binding.UpdatedTime = now
	binding.UpdatedBy = updatedBy
	return &binding, DB.Save(&binding).Error
}

func DisableModelRouteBinding(group string, modelName string, updatedBy int) error {
	return DB.Model(&ModelRouteBinding{}).
		Where(commonGroupCol+" = ? and model_name = ?", group, modelName).
		Updates(map[string]interface{}{
			"enabled":      false,
			"updated_time": common.GetTimestamp(),
			"updated_by":   updatedBy,
		}).Error
}
