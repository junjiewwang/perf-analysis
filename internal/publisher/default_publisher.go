package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/perf-analysis/internal/formatter"
	"github.com/perf-analysis/internal/storage"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// DefaultResultPublisher is the standard implementation of ResultPublisher.
// It uploads output files, generates summary.json, and generates
// type-specific detail files (e.g. retainer_analysis.json for heap dumps).
type DefaultResultPublisher struct {
	storage  storage.Storage
	registry *formatter.Registry
	logger   utils.Logger
}

// NewDefaultResultPublisher creates a new DefaultResultPublisher.
func NewDefaultResultPublisher(s storage.Storage, logger utils.Logger) *DefaultResultPublisher {
	return &DefaultResultPublisher{
		storage:  s,
		registry: formatter.NewRegistry(),
		logger:   logger,
	}
}

// Publish publishes all result files to storage.
func (p *DefaultResultPublisher) Publish(ctx context.Context, req *PublishRequest) (*PublishOutput, error) {
	output := &PublishOutput{
		UploadedFiles: make(map[string]string),
	}

	// 1. Upload analyzer-generated output files (flame graph, call graph, etc.)
	p.publishOutputFiles(ctx, req, output)

	// 2. Generate and upload summary.json for WebUI Overview
	p.publishSummary(ctx, req, output)

	// 3. Generate and upload type-specific detail files
	p.publishDetailFiles(ctx, req, output)

	return output, nil
}

// publishOutputFiles uploads each OutputFile to storage.
func (p *DefaultResultPublisher) publishOutputFiles(ctx context.Context, req *PublishRequest, output *PublishOutput) {
	for _, file := range req.Response.OutputFiles {
		if file.LocalPath == "" {
			continue
		}

		if _, err := os.Stat(file.LocalPath); os.IsNotExist(err) {
			continue
		}

		key := file.COSKey
		if key == "" {
			key = fmt.Sprintf("%s/%s", req.TaskUUID, filepath.Base(file.LocalPath))
		}

		if err := p.storage.UploadFile(ctx, key, file.LocalPath); err != nil {
			p.logger.Error("Failed to upload %s: %v", file.Name, err)
			continue
		}
		output.UploadedFiles[file.Name] = key
	}
}

// publishSummary generates summary.json using the formatter registry and
// uploads it to storage. The file is consumed by WebUI's Overview tab.
func (p *DefaultResultPublisher) publishSummary(ctx context.Context, req *PublishRequest, output *PublishOutput) {
	summary := p.registry.FormatSummary(req.Response)
	if summary == nil {
		return
	}

	// Build unified metadata block from strong-typed fields; skip zero values.
	metadata := map[string]interface{}{
		"mode":       req.Mode,
		"created_at": time.Now().Format(time.RFC3339),
	}
	if req.AnalysisVersion != "" {
		metadata["analysis_version"] = req.AnalysisVersion
	}
	if req.ModeDescription != "" {
		metadata["mode_description"] = req.ModeDescription
	}
	if req.Profile != "" {
		metadata["profile"] = req.Profile
	}
	if req.InputFile != "" {
		metadata["input_file"] = req.InputFile
	}
	if req.AnalysisTimeMs > 0 {
		metadata["analysis_time_ms"] = req.AnalysisTimeMs
	}
	summary["metadata"] = metadata

	output.Summary = summary

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		p.logger.Warn("Failed to marshal summary for task %s: %v", req.TaskUUID, err)
		return
	}

	localPath := filepath.Join(req.TaskDir, "summary.json")
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		p.logger.Warn("Failed to write summary file for task %s: %v", req.TaskUUID, err)
		return
	}

	key := fmt.Sprintf("%s/summary.json", req.TaskUUID)
	if err := p.storage.UploadFile(ctx, key, localPath); err != nil {
		p.logger.Warn("Failed to upload summary.json for task %s: %v", req.TaskUUID, err)
	}
}

// publishDetailFiles generates and uploads type-specific detail files.
// Currently this handles retainer_analysis.json for heap dump analysis.
func (p *DefaultResultPublisher) publishDetailFiles(ctx context.Context, req *PublishRequest, output *PublishOutput) {
	if req.Response.Data == nil {
		return
	}

	if req.Response.Data.Type() != model.DataTypeHeapDump {
		return
	}

	heapFmt := &formatter.HeapFormatter{}
	detailed := heapFmt.FormatDetailedRetainers(req.Response)
	if detailed == nil {
		return
	}

	data, err := json.MarshalIndent(detailed, "", "  ")
	if err != nil {
		p.logger.Warn("Failed to marshal retainer analysis for task %s: %v", req.TaskUUID, err)
		return
	}

	localPath := filepath.Join(req.TaskDir, "retainer_analysis.json")
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		p.logger.Warn("Failed to write retainer_analysis.json for task %s: %v", req.TaskUUID, err)
		return
	}

	key := fmt.Sprintf("%s/retainer_analysis.json", req.TaskUUID)
	if err := p.storage.UploadFile(ctx, key, localPath); err != nil {
		p.logger.Warn("Failed to upload retainer_analysis.json for task %s: %v", req.TaskUUID, err)
		return
	}

	output.UploadedFiles["Retainer Analysis"] = key
}
