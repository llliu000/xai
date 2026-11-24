package controller

import (
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type modelCatalogEntry struct {
	ID                  string             `json:"id"`
	Object              string             `json:"object"`
	Created             int64              `json:"created"`
	OwnedBy             string             `json:"owned_by"`
	DisplayName         string             `json:"display_name"`
	Capabilities        []string           `json:"capabilities"`
	Protocols           []string           `json:"protocols"`
	SupportsStream      bool               `json:"supports_stream"`
	SupportsIdempotency bool               `json:"supports_idempotency"`
	Limits              modelCatalogLimits `json:"limits"`
	SupportedParameters []string           `json:"supported_parameters"`
	Prices              []modelCatalogRate `json:"prices"`
	PriceVersion        string             `json:"price_version"`
	Status              string             `json:"status"`
}

type modelCatalogLimits struct {
	ContextTokens         *int  `json:"context_tokens"`
	MaxOutputTokens       *int  `json:"max_output_tokens"`
	MaxInputBytes         int64 `json:"max_input_bytes"`
	MaxImages             int   `json:"max_images"`
	MaxDurationSeconds    *int  `json:"max_duration_seconds"`
	MaxConcurrentRequests *int  `json:"max_concurrent_requests"`
	RequestsPerMinute     *int  `json:"requests_per_minute"`
}

type modelCatalogRate struct {
	Metric       string            `json:"metric"`
	UnitQuantity int               `json:"unit_quantity"`
	UnitPrice    string            `json:"unit_price"`
	Currency     string            `json:"currency"`
	Qualifiers   map[string]string `json:"qualifiers"`
}

func writeModelCatalog(c *gin.Context, visible []dto.OpenAIModels, groups modelListGroups) {
	entries := make([]modelCatalogEntry, 0, len(visible))
	if len(visible) > 0 {
		model.GetPricing()
	}
	plugins := jsplugin.DefaultRegistry.Generation()
	for _, base := range visible {
		routes := model.GetModelCatalogRoutes(base.Id, groups.ownerGroups)
		entry := modelCatalogEntry{
			ID: base.Id, Object: base.Object, Created: int64(base.Created), OwnedBy: base.OwnedBy,
			DisplayName: base.Id, Capabilities: []string{}, Protocols: []string{},
			SupportedParameters: []string{}, Prices: []modelCatalogRate{},
			Limits: modelCatalogLimits{MaxInputBytes: common.RequestBodyLimitBytes()},
			Status: "available",
		}
		var taskRoutes bool
		for _, group := range groups.ownerGroups {
			for _, route := range routes[group] {
				taskRoutes = entry.addRoute(route, plugins) || taskRoutes
			}
		}
		entry.setPrices(groups, routes, taskRoutes)
		if len(entry.Protocols) == 0 {
			entry.Status = "degraded"
		}
		if len(routes) == 0 {
			entry.Status = "unavailable"
		}
		if len(entry.Protocols) == 1 && entry.Protocols[0] == "video_generations" {
			entry.Limits.MaxInputBytes = min(entry.Limits.MaxInputBytes, relay.VideoGenerationMaxBodyBytes)
		}
		slices.Sort(entry.Capabilities)
		slices.Sort(entry.Protocols)
		slices.Sort(entry.SupportedParameters)
		entries = append(entries, entry)
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": entries})
}

func (entry *modelCatalogEntry) addProtocol(protocol, capability string, stream bool, parameters ...string) {
	if !slices.Contains(entry.Protocols, protocol) {
		entry.Protocols = append(entry.Protocols, protocol)
	}
	if !slices.Contains(entry.Capabilities, capability) {
		entry.Capabilities = append(entry.Capabilities, capability)
	}
	for _, parameter := range parameters {
		if !slices.Contains(entry.SupportedParameters, parameter) {
			entry.SupportedParameters = append(entry.SupportedParameters, parameter)
		}
	}
	entry.SupportsStream = entry.SupportsStream || stream
	entry.SupportsIdempotency = true
}

func (entry *modelCatalogEntry) addRoute(route model.ModelCatalogRoute, plugins *jsplugin.RoutingGeneration) bool {
	pluginKey := route.PluginKey
	if pluginKey == "doubao" {
		if plugin, ok := plugins.Get(pluginKey); ok && slices.Contains(plugin.Meta.Models, entry.ID) {
			if seconds, images, edit := relay.SeedanceVideoGenerationLimits(entry.ID); seconds > 0 {
				entry.addProtocol("video_generations", "video_generation", false,
					"model", "mode", "prompt", "duration_seconds", "resolution", "aspect_ratio", "references", "generate_audio", "watermark", "metadata")
				entry.Limits.MaxDurationSeconds = &seconds
				entry.Limits.MaxImages = images
				if edit && !slices.Contains(entry.Capabilities, "video_edit") {
					entry.Capabilities = append(entry.Capabilities, "video_edit")
				}
				return true
			}
		}
	}
	task := false
	if route.Endpoint == "" || route.PluginKey == "" {
		for _, operation := range []struct{ path, protocol, capability string }{
			{"/v1/responses", "responses", "text"},
			{"/v1/images/generations", "image_generations", "image_generation"},
			{"/v1/images/edits", "image_edits", "image_edit"},
		} {
			for _, binding := range plugins.LookupEndpointCandidates(http.MethodPost, operation.path, entry.ID) {
				meta := binding.Plugin.Meta
				if meta.Key != pluginKey && (pluginKey != "" || !slices.Contains(meta.ChannelTypes, route.ChannelType)) {
					continue
				}
				task = true
				switch operation.protocol {
				case "responses":
					stream := meta.ProtocolSupports(binding.Protocol, "stream")
					entry.addProtocol(operation.protocol, operation.capability, stream, "model", "input")
					if stream && !slices.Contains(entry.SupportedParameters, "stream") {
						entry.SupportedParameters = append(entry.SupportedParameters, "stream")
					}
				case "image_edits":
					entry.addProtocol(operation.protocol, operation.capability, false, "model", "prompt", "image")
				default:
					entry.addProtocol(operation.protocol, operation.capability, false, "model", "prompt")
				}
			}
		}
	}
	switch route.Endpoint {
	case constant.EndpointTypeOpenAI:
		if !common.IsImageGenerationModel(entry.ID) || route.ChannelType == constant.ChannelTypeAdvancedCustom {
			entry.addProtocol("chat_completions", "text", true, "model", "messages", "stream")
		}
	case constant.EndpointTypeOpenAIResponse:
		entry.addProtocol("responses", "text", true, "model", "input", "stream", "max_output_tokens", "store")
	case constant.EndpointTypeAnthropic:
		entry.addProtocol("messages", "text", true, "model", "messages", "stream", "max_tokens")
	case constant.EndpointTypeImageGeneration:
		entry.addProtocol("image_generations", "image_generation", false, "model", "prompt", "n")
		if route.ChannelType == constant.ChannelTypeOpenAI && strings.HasPrefix(entry.ID, "gpt-image-") {
			entry.addProtocol("image_edits", "image_edit", false, "model", "prompt", "image", "n")
		}
	}
	return task
}

func (entry *modelCatalogEntry) setPrices(groups modelListGroups, routes map[string][]model.ModelCatalogRoute, taskRoutes bool) {
	schedule := getModelCatalogSchedule(entry.ID, entry.Protocols, taskRoutes, routes)
	hash := sha256.New()
	fmt.Fprintf(hash, "v1\x00%s\x00%s", entry.ID, schedule.source)
	for _, group := range groups.ownerGroups {
		if _, exists := routes[group]; !exists {
			continue
		}
		ratio := service.GetUserGroupRatio(groups.userGroup, group)
		fmt.Fprintf(hash, "\x00%s\x00%g", group, ratio)
		if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			entry.Status = "degraded"
			continue
		}
		groupRatio := decimal.NewFromFloat(ratio)
		for _, rate := range schedule.rates {
			qualifiers := map[string]string{}
			if rate.tokenType != "" {
				qualifiers["token_type"] = rate.tokenType
			}
			if len(routes) > 1 {
				qualifiers["group"] = group
			}
			if rate.condition != "" {
				qualifiers[rate.condition] = rate.boundary
			}
			if rate.condition == "pricing" {
				entry.Status = "degraded"
			}
			entry.Prices = append(entry.Prices, modelCatalogRate{
				Metric: rate.metric, UnitQuantity: rate.quantity, UnitPrice: rate.price.Mul(groupRatio).StringFixed(8),
				Currency: "USD", Qualifiers: qualifiers,
			})
		}
	}
	entry.PriceVersion = fmt.Sprintf("price_%x", hash.Sum(nil))
	if len(entry.Prices) == 0 {
		entry.Status = "degraded"
	}
}
