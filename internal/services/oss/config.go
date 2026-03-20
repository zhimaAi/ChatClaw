package oss

import (
	_ "embed"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed uploader.yaml
var uploaderConfigData []byte

type uploaderConfig struct {
	Mode   string       `yaml:"mode"`
	Custom customConfig `yaml:"custom"`
}

type customConfig struct {
	Endpoint string `yaml:"endpoint"`

	AuthMethod      string `yaml:"auth_method"`
	AuthHeaderName  string `yaml:"auth_header_name"`
	AuthHeaderValue string `yaml:"auth_header_value"`
	AuthQueryParam  string `yaml:"auth_query_param"`
	AuthQueryValue  string `yaml:"auth_query_value"`

	FileField string `yaml:"file_field"`

	ResponseURLPath          string `yaml:"response_url_path"`
	ResponseSuccessCodeField string `yaml:"response_success_code_field"`
	ResponseSuccessCodeValue string `yaml:"response_success_code_value"`
}

var (
	configOnce   sync.Once
	parsedConfig uploaderConfig
)

// loadUploaderConfig parses the embedded uploader.yaml exactly once.
// Falls back to chatclaw mode if parsing fails.
func loadUploaderConfig() uploaderConfig {
	configOnce.Do(func() {
		parsedConfig = uploaderConfig{Mode: "chatclaw"}
		_ = yaml.Unmarshal(uploaderConfigData, &parsedConfig)
		if parsedConfig.Mode == "" {
			parsedConfig.Mode = "chatclaw"
		}
	})
	return parsedConfig
}
