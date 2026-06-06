package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestGetArtifactSearchConfigPrefersArtifactSearchSection(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		ArtifactSearch: legacyArtifactSearchConfig{
			Dictionary:      "english",
			DefaultPageSize: 33,
			PhraseFriendly:  false,
			MinRank:         0.42,
		},
		MetricSearch: legacyArtifactSearchConfig{
			Dictionary:      "simple",
			DefaultPageSize: 20,
			PhraseFriendly:  true,
			MinRank:         0.0,
		},
	}
	viper.Set("artifact_search.phrase_friendly", false)
	viper.Set("metric_search.phrase_friendly", true)

	cfg := GetArtifactSearchConfig()
	if cfg.Dictionary != "english" {
		t.Fatalf("dictionary=%q", cfg.Dictionary)
	}
	if cfg.DefaultPageSize != 33 {
		t.Fatalf("default_page_size=%d", cfg.DefaultPageSize)
	}
	if cfg.PhraseFriendly {
		t.Fatalf("phrase_friendly=%v", cfg.PhraseFriendly)
	}
	if cfg.MinRank != 0.42 {
		t.Fatalf("min_rank=%v", cfg.MinRank)
	}
}

func TestGetArtifactSearchConfigFallsBackToLegacyMetricSearchSection(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		MetricSearch: legacyArtifactSearchConfig{
			Dictionary:      "english",
			DefaultPageSize: 44,
			MaxPageSize:     88,
			PreviewMaxWords: 12,
			PhraseFriendly:  false,
			MinRank:         0.25,
		},
	}
	viper.Set("metric_search.phrase_friendly", false)

	cfg := GetArtifactSearchConfig()
	if cfg.Dictionary != "english" {
		t.Fatalf("dictionary=%q", cfg.Dictionary)
	}
	if cfg.DefaultPageSize != 44 {
		t.Fatalf("default_page_size=%d", cfg.DefaultPageSize)
	}
	if cfg.MaxPageSize != 88 {
		t.Fatalf("max_page_size=%d", cfg.MaxPageSize)
	}
	if cfg.PreviewMaxWords != 12 {
		t.Fatalf("preview_max_words=%d", cfg.PreviewMaxWords)
	}
	if cfg.PhraseFriendly {
		t.Fatalf("phrase_friendly=%v", cfg.PhraseFriendly)
	}
	if cfg.MinRank != 0.25 {
		t.Fatalf("min_rank=%v", cfg.MinRank)
	}
}

func TestGetMetricSearchWeightsConfigPrefersTopLevelBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		ArtifactSearch: legacyArtifactSearchConfig{
			Weights: ArtifactSearchWeightsConfig{
				MetricName: 9.9,
			},
		},
		MetricSearchWeights: ArtifactSearchWeightsConfig{
			MetricName:         1.1,
			MetricSubject:      1.2,
			MetricKeywords:     1.3,
			MetricDesc:         1.4,
			MetricContext:      1.5,
			ValueClass:         1.6,
			MetricUnit:         1.7,
			TableNameOrSection: 1.8,
			CategoryPaths:      1.9,
		},
	}

	cfg := GetMetricSearchWeightsConfig()
	if cfg.MetricName != 1.1 || cfg.CategoryPaths != 1.9 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetMetricSearchWeightsConfigFallsBackToLegacyNestedWeights(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		ArtifactSearch: legacyArtifactSearchConfig{
			Weights: ArtifactSearchWeightsConfig{
				MetricName:         2.1,
				MetricSubject:      2.2,
				MetricKeywords:     2.3,
				MetricDesc:         2.4,
				MetricContext:      2.5,
				ValueClass:         2.6,
				MetricUnit:         2.7,
				TableNameOrSection: 2.8,
				CategoryPaths:      2.9,
			},
		},
	}

	cfg := GetMetricSearchWeightsConfig()
	if cfg.MetricName != 2.1 || cfg.CategoryPaths != 2.9 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetSemanticProjectionSearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		SemanticProjectionSearchWeights: SemanticProjectionSearchWeightsConfig{
			DescriptiveName:    1.1,
			Keywords:           1.2,
			SemanticProjection: 1.3,
			CategoryPaths:      1.4,
		},
	}

	cfg := GetSemanticProjectionSearchWeightsConfig()
	if cfg.DescriptiveName != 1.1 || cfg.CategoryPaths != 1.4 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetInventoryItemSearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		InventoryItemSearchWeights: InventoryItemSearchWeightsConfig{
			ItemName:         1.1,
			CanonicalName:    1.2,
			ItemCategories:   1.3,
			Manufacturer:     1.4,
			Brand:            1.5,
			ModelNumber:      1.6,
			PartNumber:       1.7,
			Aliases:          1.8,
			Standards:        1.9,
			NormalizedSpecs:  2.0,
			DedupeKey:        2.1,
			ConfidenceReason: 2.2,
		},
	}

	cfg := GetInventoryItemSearchWeightsConfig()
	if cfg.ItemName != 1.1 || cfg.ConfidenceReason != 2.2 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetSceneBlockSearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() {
		AppConfig = oldConfig
		viper.Reset()
	})
	viper.Reset()

	AppConfig = AppConfigDef{
		SceneBlockSearchWeights: SceneBlockSearchWeightsConfig{
			Title:     1.1,
			SceneType: 1.2,
			Summary:   1.3,
			Keywords:  1.4,
		},
	}

	cfg := GetSceneBlockSearchWeightsConfig()
	if cfg.Title != 1.1 || cfg.Keywords != 1.4 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetProvisionSearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig; viper.Reset() })
	viper.Reset()
	AppConfig = AppConfigDef{ProvisionSearchWeights: ProvisionSearchWeightsConfig{ProvisionName: 1.1, ProvisionType: 1.2, ProvisionDesc: 1.3, Keywords: 1.4, CategoryPaths: 1.5}}
	cfg := GetProvisionSearchWeightsConfig()
	if cfg.ProvisionName != 1.1 || cfg.CategoryPaths != 1.5 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetSummarySearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig; viper.Reset() })
	viper.Reset()
	AppConfig = AppConfigDef{SummarySearchWeights: SummarySearchWeightsConfig{SummaryText: 1.1, Keywords: 1.2, CategoryPaths: 1.3}}
	cfg := GetSummarySearchWeightsConfig()
	if cfg.SummaryText != 1.1 || cfg.CategoryPaths != 1.3 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetTopicSearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig; viper.Reset() })
	viper.Reset()
	AppConfig = AppConfigDef{TopicSearchWeights: TopicSearchWeightsConfig{TopicType: 1.1, TopicDesc: 1.2, Keywords: 1.3, CategoryPaths: 1.4}}
	cfg := GetTopicSearchWeightsConfig()
	if cfg.TopicType != 1.1 || cfg.CategoryPaths != 1.4 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetEntitySearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig; viper.Reset() })
	viper.Reset()
	AppConfig = AppConfigDef{EntitySearchWeights: EntitySearchWeightsConfig{Entity: 1.1, EntityType: 1.2, Aliases: 1.3, DescText: 1.4, Keywords: 1.5}}
	cfg := GetEntitySearchWeightsConfig()
	if cfg.Entity != 1.1 || cfg.Keywords != 1.5 {
		t.Fatalf("weights=%+v", cfg)
	}
}

func TestGetRelationSearchWeightsConfigUsesConfiguredBlock(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig; viper.Reset() })
	viper.Reset()
	AppConfig = AppConfigDef{RelationSearchWeights: RelationSearchWeightsConfig{Subject: 1.1, Predicate: 1.2, Object: 1.3, DescText: 1.4, Keywords: 1.5}}
	cfg := GetRelationSearchWeightsConfig()
	if cfg.Subject != 1.1 || cfg.Keywords != 1.5 {
		t.Fatalf("weights=%+v", cfg)
	}
}
