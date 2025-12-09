# Perf Analysis Service

A Go-based performance analysis service that supports Java async-profiler CPU and memory profiling analysis, flame graph generation, call graph generation, and statistical analysis.

## Features

- **Java CPU Profiling Analysis**: Analyze async-profiler CPU profiling data
- **Java Memory Profiling Analysis**: Analyze async-profiler memory allocation data
- **Flame Graph Generation**: Generate interactive JSON flame graphs
- **Call Graph Generation**: Generate call graph data for visualization
- **Top Functions Statistics**: Calculate and report hot functions
- **Thread Analysis**: Analyze thread activity and statistics
- **Analysis Suggestions**: Generate optimization suggestions based on analysis results

## Project Structure

```
perf-analysis/
├── cmd/
│   └── analyzer/           # Main application entry point
├── internal/
│   ├── analyzer/           # Core analyzer implementations
│   ├── parser/             # Data parsing modules
│   ├── flamegraph/         # Flame graph generation (TODO)
│   ├── callgraph/          # Call graph generation (TODO)
│   ├── statistics/         # Statistics calculation (TODO)
│   ├── advisor/            # Analysis suggestion (TODO)
│   ├── storage/            # Object storage (TODO)
│   ├── database/           # Database operations (TODO)
│   ├── scheduler/          # Task scheduling (TODO)
│   ├── mock/               # Mock implementations for testing
│   └── testutil/           # Test utilities
├── pkg/
│   ├── config/             # Configuration management
│   ├── model/              # Data models
│   ├── errors/             # Error definitions
│   └── utils/              # Utility functions
├── api/
│   └── apm/                # APM callback client (TODO)
├── configs/                # Configuration files
├── test/
│   ├── integration/        # Integration tests (TODO)
│   └── e2e/                # End-to-end tests (TODO)
├── Makefile
├── go.mod
└── README.md
```

## Requirements

- Go 1.21 or later
- PostgreSQL or MySQL database
- COS (optional, for object storage)

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/perf-analysis/perf-analysis.git
cd perf-analysis
```

### 2. Install dependencies

```bash
make deps
```

### 3. Configure the service

```bash
cp configs/config.yaml.example configs/config.yaml
# Edit configs/config.yaml with your settings
```

### 4. Build the application

```bash
make build
```

### 5. Run the service

```bash
./bin/analyzer -c configs/config.yaml
```

## Development

### Running Tests

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run tests with coverage
make test-coverage

# Run tests with race detector
make test-race

# Run benchmarks
make bench
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run vet
make vet
```

### CI Checks

```bash
make ci
```

## Configuration

See [configs/config.yaml.example](configs/config.yaml.example) for configuration options.

### Key Configuration Options

| Section | Option | Description | Default |
|---------|--------|-------------|---------|
| analysis | version | Analysis version | 1.0.0 |
| analysis | data_dir | Data directory | ./data |
| analysis | max_worker | Maximum worker count | 5 |
| database | type | Database type (postgres/mysql) | postgres |
| storage | type | Storage type (cos/local) | local |
| scheduler | poll_interval | Task polling interval (seconds) | 2 |

## Architecture

### Design Principles

- **High Cohesion, Low Coupling**: Each module has a single responsibility
- **Interface-based Design**: All major components are defined by interfaces
- **Dependency Injection**: Dependencies are injected through constructors
- **Testability**: All components can be unit tested with mocks

### Core Interfaces

```go
// Analyzer interface for profiling analysis
type Analyzer interface {
    Analyze(ctx context.Context, req *AnalysisRequest, data io.Reader) (*AnalysisResult, error)
    SupportedTypes() []TaskType
    Name() string
}

// Parser interface for data parsing
type Parser interface {
    Parse(ctx context.Context, reader io.Reader) (*ParseResult, error)
    SupportedFormats() []string
    Name() string
}
```

## License

MIT License

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
