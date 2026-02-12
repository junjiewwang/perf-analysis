package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/utils"
	"github.com/perf-analysis/pkg/viewurl"
)

// CallbackNotifier sends callback notifications to external services
// after task analysis completes.
type CallbackNotifier struct {
	httpClient     *http.Client
	cfg            *config.Config
	viewURLBuilder *viewurl.Builder
	logger         utils.Logger
	maxRetries     int
}

// CallbackPayload represents the payload sent to the callback URL.
type CallbackPayload struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"` // "completed" or "failed"
	ViewURL string `json:"view_url,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewCallbackNotifier creates a new CallbackNotifier.
func NewCallbackNotifier(cfg *config.Config, logger utils.Logger) *CallbackNotifier {
	maxRetries := cfg.Callback.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	return &CallbackNotifier{
		httpClient: &http.Client{
			Timeout: cfg.Callback.GetTimeout(),
		},
		cfg:            cfg,
		viewURLBuilder: viewurl.NewBuilder(&cfg.ViewURL, &cfg.WebUI, &cfg.Retention),
		logger:         logger,
		maxRetries:     maxRetries,
	}
}

// ResolveCallbackURL resolves the effective callback URL using three-level fallback:
// task-level > source-level > global default.
func (n *CallbackNotifier) ResolveCallbackURL(taskCallbackURL, sourceCallbackURL string) string {
	if taskCallbackURL != "" {
		return taskCallbackURL
	}
	if sourceCallbackURL != "" {
		return sourceCallbackURL
	}
	return n.cfg.Callback.DefaultURL
}

// NotifySuccess sends a success callback notification with the view URL.
func (n *CallbackNotifier) NotifySuccess(ctx context.Context, callbackURL, taskUUID string) error {
	if callbackURL == "" {
		return nil
	}

	viewURL := n.viewURLBuilder.BuildViewURL(taskUUID)
	payload := CallbackPayload{
		TaskID:  taskUUID,
		Status:  "completed",
		ViewURL: viewURL,
	}

	return n.send(ctx, callbackURL, payload)
}

// NotifyFailure sends a failure callback notification.
func (n *CallbackNotifier) NotifyFailure(ctx context.Context, callbackURL, taskUUID string, taskErr error) error {
	if callbackURL == "" {
		return nil
	}

	errMsg := ""
	if taskErr != nil {
		errMsg = taskErr.Error()
	}

	payload := CallbackPayload{
		TaskID: taskUUID,
		Status: "failed",
		Error:  errMsg,
	}

	return n.send(ctx, callbackURL, payload)
}

// send performs the HTTP POST to the callback URL with retry logic.
func (n *CallbackNotifier) send(ctx context.Context, callbackURL string, payload CallbackPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal callback payload: %w", err)
	}

	// Retry with exponential backoff
	var lastErr error
	for attempt := 0; attempt < n.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create callback request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := n.httpClient.Do(req)
		if err != nil {
			lastErr = err
			n.logger.Warn("callback attempt %d failed for task %s: %v", attempt+1, payload.TaskID, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			n.logger.Info("callback sent successfully for task %s to %s", payload.TaskID, callbackURL)
			return nil
		}

		lastErr = fmt.Errorf("callback returned status %d", resp.StatusCode)
		n.logger.Warn("callback attempt %d returned status %d for task %s",
			attempt+1, resp.StatusCode, payload.TaskID)
	}

	return fmt.Errorf("callback failed after %d attempts: %w", n.maxRetries, lastErr)
}
