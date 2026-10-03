package client

import "strings"

// normalizeToolCatalog is shared by registry and ephemeral sessions.
func normalizeToolCatalog(server string, listed []Tool) (map[string]Tool, error) {
	tools := make(map[string]Tool, len(listed))
	for _, tool := range listed {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" {
			err := newError("MCP_INVALID_RESPONSE", "MCP tools/list returned an empty tool name", false, map[string]any{"server": server}, nil)
			return nil, err
		}
		if _, duplicate := tools[tool.Name]; duplicate {
			err := newError("MCP_INVALID_RESPONSE", "MCP tools/list returned duplicate tool names", false, map[string]any{"server": server, "tool": tool.Name}, nil)
			return nil, err
		}
		if tool.InputSchema == nil {
			tool.InputSchema = map[string]any{"type": "object", "additionalProperties": true}
		}
		validator, err := compileToolInputSchema(tool.InputSchema)
		if err != nil {
			schemaErr := newError(
				"MCP_SCHEMA_INVALID",
				"MCP tools/list returned an invalid input schema",
				false,
				map[string]any{"server": server, "tool": tool.Name, "reason": err.Error()},
				err,
			)
			return nil, schemaErr
		}
		tool.inputValidator = validator
		tools[tool.Name] = tool
	}
	return tools, nil
}
