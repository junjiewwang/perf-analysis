package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/junjiewwang/perf-analysis/internal/advisor"
	"github.com/junjiewwang/perf-analysis/internal/analyzer"
	"github.com/junjiewwang/perf-analysis/internal/publisher"
	"github.com/junjiewwang/perf-analysis/internal/repository"
	"github.com/junjiewwang/perf-analysis/internal/storage"
	"github.com/junjiewwang/perf-analysis/pkg/config"
	"github.com/junjiewwang/perf-analysis/pkg/model"
	"github.com/junjiewwang/perf-analysis/pkg/utils"
)

// DefaultTaskProcessor implements TaskProcessor using the analyzer components.
type DefaultTaskProcessor struct {
	config          *config.Config
	storage         storage.Storage
	rawDataStorage  storage.Storage // Optional separate storage for raw data
	repos           *repository.Repositories
	analyzerFactory *analyzer.Factory
	publisher       publisher.ResultPublisher
	notifier        *CallbackNotifier
	logger          utils.Logger
}

// ProcessorConfig holds processor configuration.
type ProcessorConfig struct {
	Config         *config.Config
	Storage        storage.Storage
	RawDataStorage storage.Storage
	Repos          *repository.Repositories
	Logger         utils.Logger
}

// NewDefaultTaskProcessor creates a new DefaultTaskProcessor.
func NewDefaultTaskProcessor(cfg *ProcessorConfig) *DefaultTaskProcessor {
	if cfg.Logger == nil {
		cfg.Logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	rawDataStorage := cfg.RawDataStorage
	if rawDataStorage == nil {
		rawDataStorage = cfg.Storage
	}

	analyzerConfig := analyzer.DefaultBaseAnalyzerConfig()

	return &DefaultTaskProcessor{
		config:          cfg.Config,
		storage:         cfg.Storage,
		rawDataStorage:  rawDataStorage,
		repos:           cfg.Repos,
		analyzerFactory: analyzer.NewFactory(analyzerConfig),
		publisher:       publisher.New(cfg.Storage, cfg.Logger),
		notifier:        NewCallbackNotifier(cfg.Config, cfg.Logger),
		logger:          cfg.Logger,
	}
}

// Process processes a single analysis task.
func (p *DefaultTaskProcessor) Process(ctx context.Context, task *Task, rules []model.SuggestionRule) error {
	p.logger.Info("Starting analysis for task %s (Mode: %s)",
		task.UUID, task.Mode)

	// Resolve callback URL once, used for both success and failure notifications
	callbackURL := ""
	if p.notifier != nil {
		callbackURL = p.notifier.ResolveCallbackURL(task.CallbackURL, task.SourceCallbackURL)
	}

	// Create task directory
	taskDir := filepath.Join(p.config.Analysis.DataDir, task.UUID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		processErr := fmt.Errorf("failed to create task directory: %w", err)
		p.notifyFailure(ctx, callbackURL, task, processErr)
		return processErr
	}

	// Clean up task directory after processing
	defer func() {
		if err := os.RemoveAll(taskDir); err != nil {
			p.logger.Warn("Failed to clean up task directory %s: %v", taskDir, err)
		}
	}()

	// Download result file
	localFile := filepath.Join(taskDir, filepath.Base(task.ResultFile))
	if err := p.downloadResultFile(ctx, task, localFile); err != nil {
		processErr := fmt.Errorf("failed to download result file: %w", err)
		p.notifyFailure(ctx, callbackURL, task, processErr)
		return processErr
	}

	// Create the appropriate analyzer
	a, err := p.analyzerFactory.CreateAnalyzerForMode(analyzer.AnalysisMode(task.Mode))
	if err != nil {
		processErr := fmt.Errorf("failed to create analyzer: %w", err)
		p.notifyFailure(ctx, callbackURL, task, processErr)
		return processErr
	}

	// Create analysis context
	analysisCtx := &AnalysisContext{
		Task:      task,
		TaskDir:   taskDir,
		LocalFile: localFile,
		Rules:     rules,
		Storage:   p.storage,
		Repos:     p.repos,
		Logger:    p.logger,
	}

	// Execute analysis
	startTime := time.Now()
	result, err := p.executeAnalysis(ctx, a, analysisCtx)
	analysisTimeMs := time.Since(startTime).Milliseconds()
	if err != nil {
		processErr := fmt.Errorf("analysis failed: %w", err)
		p.notifyFailure(ctx, callbackURL, task, processErr)
		return processErr
	}

	// Save results
	if err := p.saveResults(ctx, task, result, analysisCtx, analysisTimeMs); err != nil {
		processErr := fmt.Errorf("failed to save results: %w", err)
		p.notifyFailure(ctx, callbackURL, task, processErr)
		return processErr
	}

	// Generate and save suggestions
	if err := p.generateSuggestions(ctx, task, result, rules); err != nil {
		p.logger.Warn("Failed to generate suggestions: %v", err)
		// Don't fail the task for suggestion errors
	}

	// Handle master task updates
	if task.MasterTaskTID != nil {
		if err := p.updateMasterTask(ctx, task, result); err != nil {
			p.logger.Warn("Failed to update master task: %v", err)
		}
	}

	// Update task status to completed
	if err := p.repos.Task.UpdateAnalysisStatus(ctx, task.ID, model.AnalysisStatusCompleted); err != nil {
		processErr := fmt.Errorf("failed to update task status: %w", err)
		p.notifyFailure(ctx, callbackURL, task, processErr)
		return processErr
	}

	// Send success callback notification
	if callbackURL != "" && p.notifier != nil {
		opts := &CallbackOptions{
			Mode:            task.Mode,
			Metadata:        task.Metadata,
			TotalRecords:    result.TotalRecords,
			SuggestionCount: len(result.Suggestions),
		}
		if notifyErr := p.notifier.NotifySuccess(ctx, callbackURL, task.UUID, opts); notifyErr != nil {
			p.logger.Warn("Failed to send success callback for task %s: %v", task.UUID, notifyErr)
		}
	}

	p.logger.Info("Task %s analysis completed successfully", task.UUID)
	return nil
}

// notifyFailure sends a failure callback and updates the task status to failed.
func (p *DefaultTaskProcessor) notifyFailure(ctx context.Context, callbackURL string, task *Task, taskErr error) {
	// Update task status to failed
	if updateErr := p.repos.Task.UpdateAnalysisStatusWithInfo(ctx, task.ID, model.AnalysisStatusFailed, taskErr.Error()); updateErr != nil {
		p.logger.Error("Failed to update task %s status to failed: %v", task.UUID, updateErr)
	}

	// Send failure callback
	if callbackURL != "" && p.notifier != nil {
		opts := &CallbackOptions{
			Mode:     task.Mode,
			Metadata: task.Metadata,
		}
		if notifyErr := p.notifier.NotifyFailure(ctx, callbackURL, task.UUID, taskErr, opts); notifyErr != nil {
			p.logger.Warn("Failed to send failure callback for task %s: %v", task.UUID, notifyErr)
		}
	}
}

// downloadResultFile downloads the result file from storage.
func (p *DefaultTaskProcessor) downloadResultFile(ctx context.Context, task *Task, localPath string) error {
	return p.rawDataStorage.DownloadFile(ctx, task.ResultFile, localPath)
}

// executeAnalysis runs the analyzer on the input file.
func (p *DefaultTaskProcessor) executeAnalysis(ctx context.Context, a analyzer.Analyzer, analysisCtx *AnalysisContext) (*AnalysisResult, error) {
	// Read and parse the input file
	file, err := os.Open(analysisCtx.LocalFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	// Check for empty file
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat input file: %w", err)
	}
	if stat.Size() == 0 {
		return nil, fmt.Errorf("empty input file")
	}

	// Create analysis request
	req := &model.AnalysisRequest{
		TaskUUID:      analysisCtx.Task.UUID,
		Mode:          analysisCtx.Task.Mode,
		InputFile:     analysisCtx.LocalFile,
		OutputDir:     analysisCtx.TaskDir,
		RequestParams: analysisCtx.Task.RequestParams,
	}

	// Run analysis
	resp, err := a.Analyze(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AnalysisResult{
		Response:     resp,
		TotalRecords: resp.TotalRecords,
		Suggestions:  resp.Suggestions,
	}, nil
}

// saveResults publishes generated files to storage and saves results to database.
func (p *DefaultTaskProcessor) saveResults(ctx context.Context, task *Task, result *AnalysisResult, analysisCtx *AnalysisContext, analysisTimeMs int64) error {
	// Resolve mode description from mode registry
	var modeDescription string
	if modeInfo := analyzer.AnalysisMode(task.Mode).Info(); modeInfo != nil {
		modeDescription = modeInfo.Description
	}

	// Publish all result files (output files + summary.json + detail files) via publisher
	pubOutput, err := p.publisher.Publish(ctx, &publisher.PublishRequest{
		TaskUUID:        task.UUID,
		Mode:            task.Mode,
		TaskDir:         analysisCtx.TaskDir,
		Response:        result.Response,
		Suggestions:     result.Suggestions,
		AnalysisVersion: p.config.Analysis.Version,
		ModeDescription: modeDescription,
		Profile:         "standard",
		InputFile:       filepath.Base(task.ResultFile),
		AnalysisTimeMs:  analysisTimeMs,
	})
	if err != nil {
		return fmt.Errorf("failed to publish results: %w", err)
	}

	// Convert SuggestionItem to Suggestion for DB persistence
	suggestions := make([]model.Suggestion, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		suggestions = append(suggestions, model.Suggestion{
			Suggestion: item.Suggestion,
			FuncName:   item.FuncName,
			Namespace:  item.Namespace,
		})
	}

	// Extract DB-specific fields from analysis data and uploaded file keys
	fields := publisher.ExtractDBFields(result.Response.Data, pubOutput.UploadedFiles)

	namespaceResult := model.NamespaceResult{
		TopFuncs:               fields.TopFuncs,
		TotalRecords:           int64(result.TotalRecords),
		FlameGraphFile:         fields.FlameGraphKey,
		ExtendedFlameGraphFile: fields.FlameGraphKey,
		CallGraphFile:          fields.CallGraphKey,
		ActiveThreadsJSON:      fields.ActiveThreadsJSON,
		Suggestions:            suggestions,
	}

	analysisResult := &model.AnalysisResult{
		TaskUUID:       task.UUID,
		ContainersInfo: make(map[string]model.ContainerInfo),
		Result: map[string]model.NamespaceResult{
			"": namespaceResult,
		},
		Version: p.config.Analysis.Version,
	}

	// Save to database
	return p.repos.Result.SaveResult(ctx, analysisResult)
}

// generateSuggestions generates and saves analysis suggestions.
func (p *DefaultTaskProcessor) generateSuggestions(ctx context.Context, task *Task, result *AnalysisResult, rules []model.SuggestionRule) error {
	// Create advisor
	adv := advisor.NewAdvisor()

	// Generate suggestions using advisor
	ruleCtx := &advisor.RuleContext{
		Mode: task.Mode,
	}
	suggestions := adv.Advise(ruleCtx)

	// Add existing suggestions from analysis
	for _, sug := range result.Suggestions {
		suggestions = append(suggestions, model.Suggestion{
			TaskUUID:   task.UUID,
			Suggestion: sug.Suggestion,
			FuncName:   sug.FuncName,
			Namespace:  sug.Namespace,
		})
	}

	// Save suggestions
	if len(suggestions) > 0 {
		// Set TaskUUID for all suggestions
		for i := range suggestions {
			suggestions[i].TaskUUID = task.UUID
		}
		return p.repos.Suggestion.SaveSuggestions(ctx, suggestions)
	}

	return nil
}

// updateMasterTask updates the master task status and suggestions.
func (p *DefaultTaskProcessor) updateMasterTask(ctx context.Context, task *Task, result *AnalysisResult) error {
	if task.MasterTaskTID == nil {
		return nil
	}

	masterTID := *task.MasterTaskTID

	// Create suggestion group
	groupSuggestions := make([]model.Suggestion, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		groupSuggestions = append(groupSuggestions, model.Suggestion{
			Suggestion: item.Suggestion,
			FuncName:   item.FuncName,
			Namespace:  item.Namespace,
		})
	}
	suggestionGroup := &model.SuggestionGroup{
		Suggestion: groupSuggestions,
	}

	// Get resource type based on mode
	resourceType := string(analyzer.AnalysisMode(task.Mode).ResourceType())

	// Update master task suggestions
	if err := p.repos.MasterTask.UpdateMasterTaskSuggestions(ctx, masterTID, resourceType, suggestionGroup); err != nil {
		return err
	}

	// Check if all sub-tasks are complete and update master task status
	return p.repos.MasterTask.CheckAndCompleteIfReady(ctx, masterTID)
}

// AnalysisContext holds context for a single analysis.
type AnalysisContext struct {
	Task      *Task
	TaskDir   string
	LocalFile string
	Rules     []model.SuggestionRule
	Storage   storage.Storage
	Repos     *repository.Repositories
	Logger    utils.Logger
}

// AnalysisResult holds the result of an analysis.
type AnalysisResult struct {
	Response     *model.AnalysisResponse
	TotalRecords int
	Suggestions  []model.SuggestionItem
}
