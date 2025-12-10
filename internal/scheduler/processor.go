package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/perf-analysis/internal/advisor"
	"github.com/perf-analysis/internal/analyzer"
	"github.com/perf-analysis/internal/repository"
	"github.com/perf-analysis/internal/storage"
	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// DefaultTaskProcessor implements TaskProcessor using the analyzer components.
type DefaultTaskProcessor struct {
	config          *config.Config
	storage         storage.Storage
	rawDataStorage  storage.Storage // Optional separate storage for raw data
	repos           *repository.Repositories
	analyzerFactory *analyzer.Factory
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
		logger:          cfg.Logger,
	}
}

// Process processes a single analysis task.
func (p *DefaultTaskProcessor) Process(ctx context.Context, task *Task, rules []model.SuggestionRule) error {
	p.logger.Info("Starting analysis for task %s (Type: %d, Profiler: %d)",
		task.UUID, task.Type, task.ProfilerType)

	// Create task directory
	taskDir := p.config.GetTaskDir(task.UUID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return fmt.Errorf("failed to create task directory: %w", err)
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
		return fmt.Errorf("failed to download result file: %w", err)
	}

	// Create the appropriate analyzer
	a, err := p.analyzerFactory.CreateAnalyzer(task.Type, task.ProfilerType)
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
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
	result, err := p.executeAnalysis(ctx, a, analysisCtx)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Save results
	if err := p.saveResults(ctx, task, result, analysisCtx); err != nil {
		return fmt.Errorf("failed to save results: %w", err)
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
		return fmt.Errorf("failed to update task status: %w", err)
	}

	p.logger.Info("Task %s analysis completed successfully", task.UUID)
	return nil
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
		TaskType:      analysisCtx.Task.Type,
		ProfilerType:  analysisCtx.Task.ProfilerType,
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
		Response:       resp,
		FlameGraphFile: resp.FlameGraphFile,
		CallGraphFile:  resp.CallGraphFile,
		TopFuncs:       resp.TopFuncs,
		TotalRecords:   resp.TotalRecords,
		Suggestions:    resp.Suggestions,
	}, nil
}

// saveResults uploads generated files and saves results to database.
func (p *DefaultTaskProcessor) saveResults(ctx context.Context, task *Task, result *AnalysisResult, analysisCtx *AnalysisContext) error {
	// Upload generated files
	filesToUpload := map[string]string{
		"flamegraph.json.gz": result.FlameGraphFile,
		"callgraph.json":     result.CallGraphFile,
	}

	uploadedFiles := make(map[string]string)
	for name, localPath := range filesToUpload {
		if localPath == "" {
			continue
		}

		// Check if file exists
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			continue
		}

		cosKey := fmt.Sprintf("%s/%s", task.UUID, name)
		if err := p.storage.UploadFile(ctx, cosKey, localPath); err != nil {
			p.logger.Error("Failed to upload %s: %v", name, err)
			continue
		}
		uploadedFiles[name] = cosKey
	}

	// Build result data
	// Convert SuggestionItem to Suggestion
	suggestions := make([]model.Suggestion, 0, len(result.Suggestions))
	for _, item := range result.Suggestions {
		suggestions = append(suggestions, model.Suggestion{
			Suggestion: item.Suggestion,
			FuncName:   item.FuncName,
			Namespace:  item.Namespace,
		})
	}

	namespaceResult := model.NamespaceResult{
		TopFuncs:               result.TopFuncs,
		TotalRecords:           int64(result.TotalRecords),
		FlameGraphFile:         uploadedFiles["flamegraph.json.gz"],
		ExtendedFlameGraphFile: uploadedFiles["flamegraph.json.gz"],
		CallGraphFile:          uploadedFiles["callgraph.json"],
		ActiveThreadsJSON:      result.Response.ActiveThreadsJSON,
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
		TaskType:     task.Type,
		ProfilerType: task.ProfilerType,
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

	// Get resource type based on task type
	resourceType := getResourceType(task.Type)

	// Update master task suggestions
	if err := p.repos.MasterTask.UpdateMasterTaskSuggestions(ctx, masterTID, resourceType, suggestionGroup); err != nil {
		return err
	}

	// Check if all sub-tasks are complete and update master task status
	return p.repos.MasterTask.CheckAndCompleteIfReady(ctx, masterTID)
}

// getResourceType returns the resource type string for a task type.
func getResourceType(taskType model.TaskType) string {
	switch taskType {
	case model.TaskTypeGeneric, model.TaskTypeTiming:
		return "CPU"
	case model.TaskTypeJava:
		return "App"
	case model.TaskTypeTracing:
		return "Disk"
	case model.TaskTypeMemLeak:
		return "Memory"
	default:
		return "CPU"
	}
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
	Response       *model.AnalysisResponse
	FlameGraphFile string
	CallGraphFile  string
	TopFuncs       string
	TotalRecords   int
	Suggestions    []model.SuggestionItem
}
