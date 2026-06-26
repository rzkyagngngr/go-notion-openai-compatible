package models

import (
	"log"
	"strings"
	"sync"
	"time"
)

var DefaultModelMap = map[string]string{
	"notion-ai": "ambrosia-tart-high", "gpt-4o": "ambrosia-tart-high", "gpt-4": "ambrosia-tart-high",
	"gpt-3.5-turbo": "almond-croissant-low", "gpt-5.2": "oatmeal-cookie",
	"gpt-5.4": "oval-kumquat-medium", "gpt-5.5": "opal-quince-medium",
	"opus-4.8": "ambrosia-tart-high", "opus-4.7": "apricot-sorbet-high",
	"opus-4.6": "avocado-froyo-medium", "sonnet-4.6": "almond-croissant-low",
	"haiku-4.5": "anthropic-haiku-4.5", "gemini-2.5-flash": "vertex-gemini-2.5-flash",
	"gemini-3-flash": "gingerbread", "minimax-m2.5": "fireworks-minimax-m2.5",
	"ambrosia-tart-high": "ambrosia-tart-high",
}

var anthropicAliases = map[string]string{
	"claude-opus-4-7": "opus-4.7", "claude-opus-4-6": "opus-4.6",
	"claude-sonnet-4-6": "sonnet-4.6", "claude-haiku-4-5": "haiku-4.5",
}

const cacheTTL = 300 * time.Second

type cacheEntry struct {
	at        time.Time
	models    []map[string]any
	aliasMap  map[string]string
}

var (
	mu    sync.RWMutex
	cache *cacheEntry
)

func FriendlyAlias(modelMessage string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(modelMessage), " ", "-"))
}

func ParseAvailableModels(response map[string]any) map[string]string {
	out := make(map[string]string)
	models, _ := response["models"].([]any)
	for _, item := range models {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if disabled, _ := entry["isDisabled"].(bool); disabled {
			continue
		}
		msg, _ := entry["modelMessage"].(string)
		mid, _ := entry["model"].(string)
		if msg == "" || mid == "" {
			continue
		}
		primary := FriendlyAlias(msg)
		if primary == "" {
			continue
		}
		aliases := map[string]bool{primary: true, mid: true}
		if strings.HasPrefix(primary, "claude-") {
			aliases[strings.TrimPrefix(primary, "claude-")] = true
		}
		for _, short := range []string{
			"opus-4.8", "opus-4.7", "opus-4.6", "sonnet-4.6", "haiku-4.5",
			"gemini-3-flash", "gemini-2.5-flash", "gpt-5.5", "gpt-5.4", "gpt-5.2", "gpt-4o", "minimax-m2.5",
		} {
			if strings.Contains(primary, short) {
				aliases[short] = true
			}
		}
		for alias := range aliases {
			out[alias] = mid
		}
	}
	return out
}

func ListOpenAIModelsFromNotion(response map[string]any) []map[string]any {
	var models []map[string]any
	seenIDs := make(map[string]bool)
	seenNotion := make(map[string]bool)
	items, _ := response["models"].([]any)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if disabled, _ := entry["isDisabled"].(bool); disabled {
			continue
		}
		msg, _ := entry["modelMessage"].(string)
		notionID, _ := entry["model"].(string)
		if msg == "" || notionID == "" || seenNotion[notionID] {
			continue
		}
		modelID := FriendlyAlias(msg)
		if modelID == "" || seenIDs[modelID] {
			continue
		}
		seenIDs[modelID] = true
		seenNotion[notionID] = true
		models = append(models, map[string]any{
			"id": modelID, "object": "model", "owned_by": "notion", "created": time.Now().Unix(),
		})
	}
	return models
}

func ListOpenAIModelsFallback() []map[string]any {
	return nil
}

func CacheOpenAIModels(models []map[string]any, aliasMap map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	cache = &cacheEntry{at: time.Now(), models: models, aliasMap: aliasMap}
}

func ClearCache() {
	mu.Lock()
	defer mu.Unlock()
	cache = nil
}

func GetCachedOpenAIModels() []map[string]any {
	mu.RLock()
	defer mu.RUnlock()
	if cache == nil || time.Since(cache.at) > cacheTTL {
		return nil
	}
	return cache.models
}

func GetCachedAliasMap() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	if cache == nil || time.Since(cache.at) > cacheTTL {
		return nil
	}
	return cache.aliasMap
}

func NormalizeRequestModel(model string) string {
	cleaned := strings.TrimSpace(model)
	for strings.Contains(cleaned, "/") {
		parts := strings.Split(cleaned, "/")
		cleaned = strings.TrimSpace(parts[len(parts)-1])
	}
	return cleaned
}

func lookupModel(name string, mapping map[string]string) string {
	if name == "" || len(mapping) == 0 {
		return ""
	}
	if v, ok := mapping[name]; ok {
		return v
	}
	lower := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	if v, ok := mapping[lower]; ok {
		return v
	}
	for alias, notionID := range mapping {
		if strings.EqualFold(alias, lower) {
			return notionID
		}
	}
	for _, notionID := range mapping {
		if notionID == name {
			return name
		}
	}
	return ""
}

func ResolveModel(model, defaultModel string, aliasMap map[string]string) string {
	dynamic := aliasMap
	if dynamic == nil {
		dynamic = GetCachedAliasMap()
	}
	if dynamic == nil {
		dynamic = map[string]string{}
	}
	model = NormalizeRequestModel(model)
	defaultModel = NormalizeRequestModel(defaultModel)
	if defaultModel == "" {
		defaultModel = "ambrosia-tart-high"
	}
	if model == "" {
		return ResolveModel(defaultModel, defaultModel, aliasMap)
	}
	if model == "notion-ai" {
		if hit := lookupModel("notion-ai", dynamic); hit != "" {
			return hit
		}
		if hit := lookupModel("notion-ai", DefaultModelMap); hit != "" {
			return hit
		}
		return ResolveModel(defaultModel, defaultModel, aliasMap)
	}
	if hit := lookupModel(model, dynamic); hit != "" {
		return hit
	}
	if hit := lookupModel(model, DefaultModelMap); hit != "" {
		return hit
	}
	if alias, ok := anthropicAliases[model]; ok {
		if hit := lookupModel(alias, dynamic); hit != "" {
			return hit
		}
		if hit := lookupModel(alias, DefaultModelMap); hit != "" {
			return hit
		}
	}
	lower := strings.ToLower(strings.ReplaceAll(model, "_", "-"))
	if strings.Contains(lower, "opus") {
		for _, key := range []string{"opus-4.8", "opus-4.7", "opus-4.6"} {
			if strings.Contains(lower, key) {
				if hit := lookupModel(key, dynamic); hit != "" {
					return hit
				}
				if hit := lookupModel(key, DefaultModelMap); hit != "" {
					return hit
				}
			}
		}
	}
	if strings.Contains(lower, "sonnet") {
		if hit := lookupModel("sonnet-4.6", dynamic); hit != "" {
			return hit
		}
		if hit := lookupModel("sonnet-4.6", DefaultModelMap); hit != "" {
			return hit
		}
	}
	if strings.Contains(lower, "haiku") {
		if hit := lookupModel("haiku-4.5", dynamic); hit != "" {
			return hit
		}
		if hit := lookupModel("haiku-4.5", DefaultModelMap); hit != "" {
			return hit
		}
	}
	log.Printf("Unknown model %q — passing through to Notion", model)
	return model
}