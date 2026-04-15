// Package viewurl provides view URL generation for the perf-analysis service.
package viewurl

import (
	"fmt"
	"time"

	"github.com/junjiewwang/perf-analysis/pkg/auth"
	"github.com/junjiewwang/perf-analysis/pkg/config"
)

// Builder constructs signed view URLs for task results.
type Builder struct {
	viewURLCfg   *config.ViewURLConfig
	webUICfg     *config.WebUIConfig
	retentionCfg *config.RetentionConfig
}

// NewBuilder creates a new view URL builder from configuration.
func NewBuilder(viewURLCfg *config.ViewURLConfig, webUICfg *config.WebUIConfig, retentionCfg *config.RetentionConfig) *Builder {
	return &Builder{
		viewURLCfg:   viewURLCfg,
		webUICfg:     webUICfg,
		retentionCfg: retentionCfg,
	}
}

// GetBaseURL returns the effective base URL for view URLs.
// Priority: explicit base_url > auto-derive from webui.enabled + webui.port.
func (b *Builder) GetBaseURL() string {
	if b.viewURLCfg.BaseURL != "" {
		return b.viewURLCfg.BaseURL
	}
	if b.webUICfg.Enabled && b.webUICfg.Port > 0 {
		return fmt.Sprintf("http://localhost:%d", b.webUICfg.Port)
	}
	return ""
}

// BuildViewURL generates a signed view URL for a given task UUID.
func (b *Builder) BuildViewURL(taskUUID string) string {
	baseURL := b.GetBaseURL()
	if baseURL == "" {
		return ""
	}

	if !b.viewURLCfg.Auth.Enabled || b.viewURLCfg.Auth.Secret == "" {
		return fmt.Sprintf("%s/?task=%s", baseURL, taskUUID)
	}

	expireAt := time.Now().Add(b.retentionCfg.GetDefaultRetention()).Unix()
	token := auth.GenerateViewToken(b.viewURLCfg.Auth.Secret, taskUUID, expireAt)
	return fmt.Sprintf("%s/?task=%s&token=%s&exp=%d", baseURL, taskUUID, token, expireAt)
}
