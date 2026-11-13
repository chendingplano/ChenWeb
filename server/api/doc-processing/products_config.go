package docprocessing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProductsConfig contains operator overrides. Missing keys use the launch environment.
type ProductsConfig map[string]string

var productConfigKeys = map[string]string{
	"EXTRACT_PRODUCT_MENTIONS_PROMPT": "text", "EXTRACT_PRODUCT_MENTIONS_MODEL_NAME": "text", "ENRICH_PRODUCT_MENTION_PROMPT": "text", "ENRICH_PRODUCT_RELATIONS_MODEL_NAME": "text", "EXTRACT_PRODUCT_MODEL_NAME": "text", "EXTRACT_PRODUCT_MODEL_FALLBACK": "text", "EXTRACT_PRODUCT_MERGED_PROMPT": "text", "EXTRACT_PRODUCTS_MERGED_PASS": "bool", "EXTRACT_PRODUCTS_REASONING": "bool", "EXTRACT_PRODUCTS_MAX_TASKS": "int", "EXTRACT_PRODUCT_TRANSLATION": "translation", "TRANSLATE_PRODUCTS_PROMPT": "text", "TRANSLATION_MODEL_NAME": "text", "TRANSLATE_PRODUCTS_BATCH_SIZE": "int", "TRANSLATE_PRODUCTS_THINKING": "text", "EXTRACT_PRODUCT_PASS_1_ONLY": "bool",
}

func ProductsConfigPath() string {
	return filepath.Join(filepath.Dir(PipelineDefaultsPath()), "extract-products-config.json")
}
func LoadProductsConfig(path string) (ProductsConfig, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ProductsConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c ProductsConfig
	if err = json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c == nil {
		c = ProductsConfig{}
	}
	return c, nil
}
func SaveProductsConfig(path string, c ProductsConfig) error {
	for k, v := range c {
		kind, ok := productConfigKeys[k]
		if !ok {
			return fmt.Errorf("unknown setting %s", k)
		}
		if strings.TrimSpace(v) == "" {
			delete(c, k)
			continue
		}
		switch kind {
		case "bool":
			if v != "true" && v != "false" {
				return fmt.Errorf("invalid boolean for %s", k)
			}
		case "int":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 {
				return fmt.Errorf("invalid positive integer for %s", k)
			}
		case "translation":
			if v != "on" && v != "off" {
				return fmt.Errorf("invalid translation setting")
			}
		}
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".products-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (c ProductsConfig) value(key string) string {
	if v, ok := c[key]; ok {
		return v
	}
	return os.Getenv(key)
}
func (c ProductsConfig) first(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(c.value(k)); v != "" {
			return v
		}
	}
	return ""
}
func (c ProductsConfig) boolean(key string, fallback bool) bool {
	v, e := strconv.ParseBool(strings.TrimSpace(c.value(key)))
	if e != nil {
		return fallback
	}
	return v
}
func (c ProductsConfig) integer(key string, fallback int) int {
	v, e := strconv.Atoi(strings.TrimSpace(c.value(key)))
	if e != nil || v < 1 {
		return fallback
	}
	return v
}
