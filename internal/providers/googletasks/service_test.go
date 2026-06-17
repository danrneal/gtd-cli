package googletasks

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/oauth2"
)

func TestTokenFromFile(t *testing.T) {
	t.Parallel()
	validToken := &oauth2.Token{
		AccessToken: "access-abc",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour).Round(time.Second),
	}

	tests := []struct {
		name          string
		setupFile     func(t *testing.T, dir string) string
		wantToken     *oauth2.Token
		wantErr       bool
		wantErrTarget error
	}{
		{
			name: "valid token file",
			setupFile: func(t *testing.T, dir string) string {
				validTokenJSON, err := json.Marshal(validToken)
				if err != nil {
					t.Fatalf("failed to marshal validToken: %v", err)
				}

				path := filepath.Join(dir, "token.json")
				if err := os.WriteFile(path, validTokenJSON, 0o600); err != nil {
					t.Fatalf("failed to write setup file: %v", err)
				}

				return path
			},
			wantToken: validToken,
			wantErr:   false,
		},
		{
			name: "file not found",
			setupFile: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			wantToken:     nil,
			wantErr:       true,
			wantErrTarget: fs.ErrNotExist,
		},
		{
			name: "invalid json",
			setupFile: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "bad.json")
				if err := os.WriteFile(path, []byte("{bad-json"), 0o600); err != nil {
					t.Fatalf("failed to write setup file: %v", err)
				}

				return path
			},
			wantToken: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			tokenFile := tt.setupFile(t, tmpDir)
			gotToken, err := tokenFromFile(tokenFile)

			if tt.wantErr {
				if err == nil {
					t.Error("tokenFromFile() expected error, got nil")
				} else if tt.wantErrTarget != nil && !errors.Is(err, tt.wantErrTarget) {
					t.Errorf("tokenFromFile() expected error target %v, got: %v", tt.wantErrTarget, err)
				}

				return
			}

			if err != nil {
				t.Errorf("tokenFromFile() unexpected error: %v", err)
				return
			}

			opts := []cmp.Option{
				cmpopts.IgnoreUnexported(oauth2.Token{}),
			}

			if diff := cmp.Diff(tt.wantToken, gotToken, opts...); diff != "" {
				t.Errorf("tokenFromFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSaveToken(t *testing.T) {
	t.Parallel()
	validToken := &oauth2.Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		RefreshToken: "refresh-456",
		Expiry:       time.Now().Add(time.Hour).Round(time.Second),
	}

	tests := []struct {
		name          string
		setupFile     func(t *testing.T, dir string) string
		token         *oauth2.Token
		wantErr       bool
		wantErrTarget error
	}{
		{
			name: "valid save",
			setupFile: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "token.json")
			},
			token:   validToken,
			wantErr: false,
		},
		{
			name: "invalid path",
			setupFile: func(t *testing.T, dir string) string {
				return "/invalid/path/that/does/not/exist/token.json"
			},
			token:         validToken,
			wantErr:       true,
			wantErrTarget: fs.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			tokenFile := tt.setupFile(t, tmpDir)
			err := saveToken(tokenFile, tt.token)

			if tt.wantErr {
				if err == nil {
					t.Error("saveToken() expected error, got nil")
				} else if tt.wantErrTarget != nil && !errors.Is(err, tt.wantErrTarget) {
					t.Errorf("saveToken() expected error target %v, got: %v", tt.wantErrTarget, err)
				}

				return
			}

			if err != nil {
				t.Errorf("saveToken() unexpected error: %v", err)
				return
			}

			if _, err = os.Stat(tokenFile); errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("token file was not created at %s", tokenFile)
			}

			content, err := os.ReadFile(tokenFile)
			if err != nil {
				t.Fatalf("failed to read token file: %v", err)
			}

			var savedToken oauth2.Token
			if err := json.Unmarshal(content, &savedToken); err != nil {
				t.Fatalf("failed to unmarshal saved token: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreUnexported(oauth2.Token{}),
			}

			if diff := cmp.Diff(*tt.token, savedToken, opts...); diff != "" {
				t.Errorf("saveToken() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// mockTokenSource implements oauth2.TokenSource to provide a static token for testing.
type mockTokenSource struct {
	token *oauth2.Token
	err   error
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	return m.token, m.err
}

func TestFileTokenSource_Token(t *testing.T) {
	t.Parallel()
	oldToken := &oauth2.Token{
		AccessToken: "old",
	}
	newToken := &oauth2.Token{
		AccessToken: "new",
	}

	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		tokenSource oauth2.TokenSource
		token       *oauth2.Token
		wantToken   *oauth2.Token
		wantSave    bool
		wantLog     bool
		wantErr     bool
	}{
		{
			name: "token unchanged",
			setupFile: func(t *testing.T) string {
				tokenFile := filepath.Join(t.TempDir(), "token.json")
				return tokenFile
			},
			tokenSource: &mockTokenSource{
				token: oldToken,
			},
			token:     oldToken,
			wantToken: oldToken,
			wantSave:  false,
			wantLog:   false,
			wantErr:   false,
		},
		{
			name: "token refreshed",
			setupFile: func(t *testing.T) string {
				tokenFile := filepath.Join(t.TempDir(), "token.json")
				return tokenFile
			},
			tokenSource: &mockTokenSource{
				token: newToken,
			},
			token:     oldToken,
			wantToken: newToken,
			wantSave:  true,
			wantLog:   false,
			wantErr:   false,
		},
		{
			name: "source error",
			setupFile: func(t *testing.T) string {
				tokenFile := filepath.Join(t.TempDir(), "token.json")
				return tokenFile
			},
			tokenSource: &mockTokenSource{
				err: os.ErrNotExist,
			},
			token:     oldToken,
			wantToken: nil,
			wantSave:  false,
			wantLog:   false,
			wantErr:   true,
		},
		{
			name: "token refreshed but save fails",
			setupFile: func(t *testing.T) string {
				tokenFile := "/invalid/path/that/does/not/exist/token.json"
				return tokenFile
			},
			tokenSource: &mockTokenSource{
				token: newToken,
			},
			token:     nil,
			wantToken: newToken,
			wantSave:  false,
			wantLog:   true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tokenFile := tt.setupFile(t)
			if tt.token != nil {
				if err := saveToken(tokenFile, tt.token); err != nil {
					t.Fatalf("failed to setup initial token: %v", err)
				}
			}

			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

			fts := &fileTokenSource{
				tokenSource: tt.tokenSource,
				tokenFile:   tokenFile,
				token:       tt.token,
				logger:      logger,
			}

			got, err := fts.Token()

			if tt.wantErr {
				if err == nil {
					t.Error("Token() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("Token() unexpected error: %v", err)
				return
			}

			opts := []cmp.Option{
				cmpopts.IgnoreUnexported(oauth2.Token{}),
			}

			if diff := cmp.Diff(tt.wantToken, got, opts...); diff != "" {
				t.Errorf("Token() mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantToken, fts.token, opts...); diff != "" {
				t.Errorf("Internal cache mismatch (-want +got):\n%s", diff)
			}

			var savedToken oauth2.Token
			content, _ := os.ReadFile(tokenFile)
			_ = json.Unmarshal(content, &savedToken)

			var wantToken *oauth2.Token
			if tt.wantSave {
				wantToken = tt.wantToken
			} else if tt.token != nil {
				wantToken = tt.token
			}

			if tt.wantLog && logBuffer.Len() == 0 {
				t.Error("Token() expected to log a warning, but buffer was empty")
			} else if !tt.wantLog && logBuffer.Len() > 0 {
				t.Errorf("Token() expected to remain silent, but logged:\n%s", logBuffer.String())
			}

			if !tt.wantSave && tt.token == nil {
				return
			}

			if diff := cmp.Diff(*wantToken, savedToken, opts...); diff != "" {
				t.Errorf("File save mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
