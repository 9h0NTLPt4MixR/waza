package copilotconfig

import (
	"fmt"
	"strings"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/go-viper/mapstructure/v2"
)

// ConvertMCPServers converts eval YAML mcp_servers entries into Copilot SDK MCP
// configs. Invalid entries are skipped after emitting a warning through warnf.
func ConvertMCPServers(serverConfigs map[string]any, warnf func(string, ...any)) map[string]copilot.MCPServerConfig {
	if len(serverConfigs) == 0 {
		return nil
	}

	result := make(map[string]copilot.MCPServerConfig, len(serverConfigs))
	for name, cfg := range serverConfigs {
		cfgMap, ok := cfg.(map[string]any)
		if !ok {
			warnf("Warning: mcp_server %q config is not a map, skipping\n", name)
			continue
		}

		serverType, _ := cfgMap["type"].(string)
		switch strings.ToLower(serverType) {
		case "", "stdio":
			var stdio copilot.MCPStdioServerConfig
			if err := decode(cfgMap, &stdio); err != nil {
				warnf("Warning: mcp_server %q stdio config is invalid: %v, skipping\n", name, err)
				continue
			}
			result[name] = stdio
		case "http", "sse":
			var http copilot.MCPHTTPServerConfig
			if err := decode(cfgMap, &http); err != nil {
				warnf("Warning: mcp_server %q http config is invalid: %v, skipping\n", name, err)
				continue
			}
			result[name] = http
		default:
			warnf("Warning: mcp_server %q has unsupported type %q, skipping\n", name, serverType)
		}
	}

	return result
}

func decode(input map[string]any, output any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           output,
		TagName:          "json",
		WeaklyTypedInput: true,
	})
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(input); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}
