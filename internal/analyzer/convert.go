package analyzer

import (
	"github.com/perf-analysis/perflib"
	libanalyzer "github.com/perf-analysis/perflib/analyzer"
	libmodel "github.com/perf-analysis/perflib/model"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// convertRequest converts a business AnalysisRequest to a library AnalysisRequest.
func convertRequest(req *model.AnalysisRequest) *libmodel.AnalysisRequest {
	return &libmodel.AnalysisRequest{
		Mode:      req.Mode,
		InputFile: req.InputFile,
		OutputDir: req.OutputDir,
	}
}

// convertResponse converts a library AnalysisResponse to a business AnalysisResponse,
// adding business-specific fields (TaskUUID, COSKey mapping).
func convertResponse(libResp *libmodel.AnalysisResponse, taskUUID string) *model.AnalysisResponse {
	// Map output files: RelativePath → COSKey
	outputFiles := make([]model.OutputFile, 0, len(libResp.OutputFiles))
	for _, f := range libResp.OutputFiles {
		of := model.OutputFile{
			Name:         f.Name,
			LocalPath:    f.LocalPath,
			RelativePath: f.RelativePath,
			ContentType:  f.ContentType,
		}
		// Build COSKey from TaskUUID and RelativePath
		if taskUUID != "" && f.RelativePath != "" {
			of.COSKey = taskUUID + "/" + f.RelativePath
		}
		outputFiles = append(outputFiles, of)
	}

	return &model.AnalysisResponse{
		TaskUUID:     taskUUID,
		Mode:         libResp.Mode,
		TotalRecords: libResp.TotalRecords,
		OutputFiles:  outputFiles,
		Data:         libResp.Data,
		Suggestions:  libResp.Suggestions,
		Error:        libResp.Error,
	}
}

// convertConfig converts a business BaseAnalyzerConfig to a library BaseAnalyzerConfig,
// adapting the Logger interface.
func convertConfig(cfg *BaseAnalyzerConfig) *libanalyzer.BaseAnalyzerConfig {
	if cfg == nil {
		return nil
	}
	return &libanalyzer.BaseAnalyzerConfig{
		OutputDir:         cfg.OutputDir,
		FlameGraphOptions: cfg.FlameGraphOptions,
		CallGraphOptions:  cfg.CallGraphOptions,
		TopFuncsN:         cfg.TopFuncsN,
		IncludeSwapper:    cfg.IncludeSwapper,
		Logger:            adaptLogger(cfg.Logger),
		Verbose:           cfg.Verbose,
		AnalysisProfile:   libanalyzer.AnalysisProfile(cfg.AnalysisProfile),
	}
}

// adaptLogger adapts a utils.Logger to a perflib.Logger.
// If the logger is nil, returns nil (perflib handles nil loggers internally).
// Since utils.Logger is a superset of perflib.Logger (both have Debug/Info/Warn/Error),
// utils.Logger already satisfies perflib.Logger interface.
func adaptLogger(logger utils.Logger) perflib.Logger {
	if logger == nil {
		return nil
	}
	return logger
}
