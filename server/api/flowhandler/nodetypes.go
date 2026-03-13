// server/api/flowhandler/nodetypes.go
package flowhandler

import (
	"net/http"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// GetNodeTypes returns the static list of 11 node type definitions.
// GET /api/v1/flow-node-types
func GetNodeTypes(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_100")
	logger.Info("GetNodeTypes called")
	return c.JSON(http.StatusOK, map[string]any{
		"nodeTypes": []map[string]any{
			{
				"id": "ai-assistant", "label": "AI Assistant", "category": "AI", "icon": "Bot",
				"inputs": []string{"text", "context"}, "outputs": []string{"response"},
				"defaultData": map[string]any{"model": "gpt-4o", "system_prompt": "", "temperature": 0.7},
			},
			{
				"id": "coding-assistant", "label": "Coding Assistant", "category": "AI", "icon": "Terminal",
				"inputs": []string{"code", "context"}, "outputs": []string{"code", "explanation"},
				"defaultData": map[string]any{"language": "typescript", "model": "gpt-4o"},
			},
			{
				"id": "text", "label": "Text", "category": "Data", "icon": "Type",
				"inputs": []string{}, "outputs": []string{"text"},
				"defaultData": map[string]any{"content": ""},
			},
			{
				"id": "document", "label": "Document", "category": "Data", "icon": "FileText",
				"inputs": []string{}, "outputs": []string{"document"},
				"defaultData": map[string]any{"doc_id": "", "doc_source": ""},
			},
			{
				"id": "file", "label": "File", "category": "Data", "icon": "File",
				"inputs": []string{}, "outputs": []string{"file"},
				"defaultData": map[string]any{"file_path": "", "file_type": "txt"},
			},
			{
				"id": "media", "label": "Media", "category": "Data", "icon": "Image",
				"inputs": []string{}, "outputs": []string{"media"},
				"defaultData": map[string]any{"media_url": "", "media_type": "image"},
			},
			{
				"id": "tool", "label": "Tool", "category": "Actions", "icon": "Wrench",
				"inputs": []string{"args"}, "outputs": []string{"result"},
				"defaultData": map[string]any{"tool_name": "", "tool_config": "{}"},
			},
			{
				"id": "mcp", "label": "MCP", "category": "Actions", "icon": "Plug",
				"inputs": []string{"request"}, "outputs": []string{"response"},
				"defaultData": map[string]any{"server_url": "", "auth_token": ""},
			},
			{
				"id": "http-request", "label": "HTTP Request", "category": "Actions", "icon": "Globe",
				"inputs": []string{"body"}, "outputs": []string{"response"},
				"defaultData": map[string]any{"url": "", "method": "GET", "headers": "{}"},
			},
			{
				"id": "rule", "label": "Rule", "category": "Actions", "icon": "Filter",
				"inputs": []string{"data"}, "outputs": []string{"pass", "fail"},
				"defaultData": map[string]any{"rule_expression": "", "description": ""},
			},
			{
				"id": "git", "label": "GIT", "category": "Actions", "icon": "GitBranch",
				"inputs": []string{"files"}, "outputs": []string{"result"},
				"defaultData": map[string]any{"repo_url": "", "branch": "main", "operation": "status"},
			},
		},
	})
}
