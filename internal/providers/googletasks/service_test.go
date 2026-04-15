package googletasks

import (
	"encoding/json"
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
		name      string
		setupFile func(t *testing.T, dir string) string
		wantToken *oauth2.Token
		wantErr   bool
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
			wantToken: nil,
			wantErr:   true,
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
		name      string
		setupFile func(t *testing.T, dir string) string
		token     *oauth2.Token
		wantErr   bool
	}{
		{
			name: "valid save",
			setupFile: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "token.json")
			},
			token:   validToken,
			wantErr: false,
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
				}

				return
			}

			if err != nil {
				t.Errorf("saveToken() unexpected error: %v", err)
				return
			}

			if _, err = os.Stat(tokenFile); os.IsNotExist(err) {
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
		tokenSource oauth2.TokenSource
		token       *oauth2.Token
		wantToken   *oauth2.Token
		wantSave    bool
		wantErr     bool
	}{
		{
			name: "token unchanged",
			tokenSource: &mockTokenSource{
				token: oldToken,
			},
			token:     oldToken,
			wantToken: oldToken,
			wantSave:  false,
			wantErr:   false,
		},
		{
			name: "token refreshed",
			tokenSource: &mockTokenSource{
				token: newToken,
			},
			token:     oldToken,
			wantToken: newToken,
			wantSave:  true,
			wantErr:   false,
		},
		{
			name: "source error",
			tokenSource: &mockTokenSource{
				err: os.ErrNotExist,
			},
			token:     oldToken,
			wantToken: nil,
			wantSave:  false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			tokenFile := filepath.Join(tmpDir, "token.json")
			if tt.token != nil {
				if err := saveToken(tokenFile, tt.token); err != nil {
					t.Fatalf("failed to setup initial token: %v", err)
				}
			}

			fts := &fileTokenSource{
				tokenSource: tt.tokenSource,
				tokenFile:   tokenFile,
				token:       tt.token,
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

			var savedToken oauth2.Token
			content, _ := os.ReadFile(tokenFile)
			_ = json.Unmarshal(content, &savedToken)

			var wantToken *oauth2.Token
			if tt.wantSave {
				wantToken = tt.wantToken
			} else if tt.token != nil {
				wantToken = tt.token
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
