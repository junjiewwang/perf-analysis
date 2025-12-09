package mock

import (
	"context"
	"io"

	"github.com/stretchr/testify/mock"
)

// MockStorage is a mock implementation of the Storage interface.
type MockStorage struct {
	mock.Mock
}

// Upload mocks the Upload method.
func (m *MockStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	args := m.Called(ctx, key, reader)
	return args.Error(0)
}

// UploadFile mocks the UploadFile method.
func (m *MockStorage) UploadFile(ctx context.Context, key string, localPath string) error {
	args := m.Called(ctx, key, localPath)
	return args.Error(0)
}

// Download mocks the Download method.
func (m *MockStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

// DownloadFile mocks the DownloadFile method.
func (m *MockStorage) DownloadFile(ctx context.Context, key string, localPath string) error {
	args := m.Called(ctx, key, localPath)
	return args.Error(0)
}

// Delete mocks the Delete method.
func (m *MockStorage) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

// Exists mocks the Exists method.
func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

// ExpectUpload sets up an expectation for Upload.
func (m *MockStorage) ExpectUpload(key string, err error) *mock.Call {
	return m.On("Upload", mock.Anything, key, mock.Anything).Return(err)
}

// ExpectUploadFile sets up an expectation for UploadFile.
func (m *MockStorage) ExpectUploadFile(key, localPath string, err error) *mock.Call {
	return m.On("UploadFile", mock.Anything, key, localPath).Return(err)
}

// ExpectDownload sets up an expectation for Download.
func (m *MockStorage) ExpectDownload(key string, reader io.ReadCloser, err error) *mock.Call {
	return m.On("Download", mock.Anything, key).Return(reader, err)
}

// ExpectDownloadFile sets up an expectation for DownloadFile.
func (m *MockStorage) ExpectDownloadFile(key, localPath string, err error) *mock.Call {
	return m.On("DownloadFile", mock.Anything, key, localPath).Return(err)
}

// ExpectAnyUpload sets up an expectation for any Upload call.
func (m *MockStorage) ExpectAnyUpload(err error) *mock.Call {
	return m.On("Upload", mock.Anything, mock.Anything, mock.Anything).Return(err)
}

// ExpectAnyUploadFile sets up an expectation for any UploadFile call.
func (m *MockStorage) ExpectAnyUploadFile(err error) *mock.Call {
	return m.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(err)
}
