package llmimport

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	toml "github.com/pelletier/go-toml/v2"
)

type ImportedAccount struct {
	AccountKey string
	Name       string
	Provider   string
	BaseURL    string
	APIKey     string
}

type ImportedProfile struct {
	AccountKey           string
	ProfileName          string
	ModelName            string
	ThinkingType         string
	TimeoutSec           int
	MaxInflight          int
	MaxRequestsPerMinute int
	MaxTokensPerMinute   int
	TokenReservePerCall  int
}

type ParsedModels struct {
	Accounts []ImportedAccount
	Profiles []ImportedProfile
}

func ParseModelsTOML(raw []byte) (ParsedModels, error) {
	var models ApiTypes.LLMModelsFile
	if err := toml.Unmarshal(raw, &models); err != nil {
		return ParsedModels{}, err
	}

	keys := make([]string, 0, len(models))
	for key := range models {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	accountMap := map[string]ImportedAccount{}
	profiles := make([]ImportedProfile, 0, len(keys))
	for _, key := range keys {
		model := models[key]
		provider := inferProvider(key, model.BaseURL)
		accountKey := makeAccountKey(provider, model.BaseURL, model.APIKey)
		if _, exists := accountMap[accountKey]; !exists {
			accountMap[accountKey] = ImportedAccount{
				AccountKey: accountKey,
				Name:       defaultAccountName(provider, model.BaseURL),
				Provider:   provider,
				BaseURL:    model.BaseURL,
				APIKey:     model.APIKey,
			}
		}
		profiles = append(profiles, ImportedProfile{
			AccountKey:           accountKey,
			ProfileName:          key,
			ModelName:            model.ModelName,
			ThinkingType:         model.ThinkingType,
			TimeoutSec:           model.TimeoutSec,
			MaxInflight:          model.MaxInflight,
			MaxRequestsPerMinute: model.MaxRequestsPerMinute,
			MaxTokensPerMinute:   model.MaxTokensPerMinute,
			TokenReservePerCall:  model.TokenReservePerCall,
		})
	}

	accounts := make([]ImportedAccount, 0, len(accountMap))
	for _, key := range sortedAccountKeys(accountMap) {
		accounts = append(accounts, accountMap[key])
	}

	return ParsedModels{
		Accounts: accounts,
		Profiles: profiles,
	}, nil
}

func inferProvider(profileName, baseURL string) string {
	base := strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case strings.Contains(base, "deepseek"):
		return "deepseek"
	case strings.Contains(base, "openai"):
		return "openai"
	case strings.Contains(base, "dashscope"), strings.HasPrefix(strings.ToLower(profileName), "qwen"):
		return "qwen"
	default:
		return "openai_compatible"
	}
}

func makeAccountKey(provider, baseURL, apiKey string) string {
	sum := sha1.Sum([]byte(provider + "\n" + strings.TrimSpace(baseURL) + "\n" + strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}

func defaultAccountName(provider, baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return provider
	}
	return fmt.Sprintf("%s:%s", provider, u.Host)
}

func sortedAccountKeys(accounts map[string]ImportedAccount) []string {
	keys := make([]string, 0, len(accounts))
	for key := range accounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
