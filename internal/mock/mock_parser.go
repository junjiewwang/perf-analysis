// Package mock provides mock implementations for testing.
package mock

import (
	"context"
	"io"

	"github.com/stretchr/testify/mock"

	"github.com/perf-analysis/pkg/model"
)

// MockParser is a mock implementation of the Parser interface.
type MockParser struct {
	mock.Mock
}

// Parse mocks the Parse method.
func (m *MockParser) Parse(ctx context.Context, reader io.Reader) (*model.ParseResult, error) {
	args := m.Called(ctx, reader)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ParseResult), args.Error(1)
}

// SupportedFormats mocks the SupportedFormats method.
func (m *MockParser) SupportedFormats() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

// Name mocks the Name method.
func (m *MockParser) Name() string {
	args := m.Called()
	return args.String(0)
}

// ExpectParse sets up an expectation for Parse.
func (m *MockParser) ExpectParse(result *model.ParseResult, err error) *mock.Call {
	return m.On("Parse", mock.Anything, mock.Anything).Return(result, err)
}

// ExpectSupportedFormats sets up an expectation for SupportedFormats.
func (m *MockParser) ExpectSupportedFormats(formats []string) *mock.Call {
	return m.On("SupportedFormats").Return(formats)
}

// ExpectName sets up an expectation for Name.
func (m *MockParser) ExpectName(name string) *mock.Call {
	return m.On("Name").Return(name)
}
