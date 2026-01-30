package googletasks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestTokenFromFile(t *testing.T) {
	validToken := &oauth2.Token{
		AccessToken: "access-abc",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour).Round(time.Second),
	}
	validTokenJSON, _ := json.Marshal(validToken)

	tests := []struct {
		name      string
		setupFile func(t *testing.T, dir string) string
		wantToken *oauth2.Token
		wantErr   bool
	}{
		{
			name: "valid token file",
			setupFile: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "token.json")
				if err := os.WriteFile(path, validTokenJSON, 0600); err != nil {
					t.Fatalf("failed to write setup file: %v", err)
				}

				return path
			},
			wantToken: validToken,
			wantErr:   false,
		},
		{
			name: "file not found",
			setupFile: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			wantToken: nil,
			wantErr:   true,
		},
		{
			name: "invalid json",
			setupFile: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "bad.json")
				if err := os.WriteFile(path, []byte("{bad-json"), 0600); err != nil {
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
			tmpDir := t.TempDir()
			tokenFile := tt.setupFile(t, tmpDir)

			gotToken, err := tokenFromFile(tokenFile)

			if tt.wantErr {
				if err == nil {
					t.Error("tokenFromFile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("tokenFromFile() unexpected error: %v", err)
				}

				if gotToken.AccessToken != tt.wantToken.AccessToken {
					t.Errorf("got AccessToken %q, want %q", gotToken.AccessToken, tt.wantToken.AccessToken)
				}
				if !gotToken.Expiry.Equal(tt.wantToken.Expiry) {
					t.Errorf("got Expiry %v, want %v", gotToken.Expiry, tt.wantToken.Expiry)
				}
			}
		})
	}
}

func TestSaveToken(t *testing.T) {
	validToken := &oauth2.Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		RefreshToken: "refresh-456",
		Expiry:       time.Now().Add(1 * time.Hour).Round(time.Second),
	}

	tests := []struct {
		name      string
		setupFile func(t *testing.T, dir string) string
		token     *oauth2.Token
		wantErr   bool
	}{
		{
			name: "valid save",
			setupFile: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "token.json")
			},
			token:   validToken,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenFile := tt.setupFile(t, tmpDir)

			err := saveToken(tokenFile, tt.token)

			if tt.wantErr {
				if err == nil {
					t.Error("saveToken() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("saveToken() unexpected error: %v", err)
				}

				if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
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

				if savedToken.AccessToken != tt.token.AccessToken {
					t.Errorf("got AccessToken %q, want %q", savedToken.AccessToken, tt.token.AccessToken)
				}

				if savedToken.RefreshToken != tt.token.RefreshToken {
					t.Errorf("got RefreshToken %q, want %q", savedToken.RefreshToken, tt.token.RefreshToken)
				}

				if !savedToken.Expiry.Equal(tt.token.Expiry) {
					t.Errorf("got Expiry %v, want %v", savedToken.Expiry, tt.token.Expiry)
				}
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
			tmpDir := t.TempDir()
			tokenFile := filepath.Join(tmpDir, "token.json")

			if tt.token != nil {
				saveToken(tokenFile, tt.token)
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
			} else {
				if err != nil {
					t.Errorf("Token() unexpected error: %v", err)
				}

				if got.AccessToken != tt.wantToken.AccessToken {
					t.Errorf("got token %v, want %v", got, tt.wantToken)
				}

				content, _ := os.ReadFile(tokenFile)
				var savedToken oauth2.Token
				_ = json.Unmarshal(content, &savedToken)

				if tt.wantSave {
					if savedToken.AccessToken != tt.wantToken.AccessToken {
						t.Errorf("saved token %q, want %q", savedToken.AccessToken, tt.wantToken.AccessToken)
					}
				} else {
					if savedToken.AccessToken != tt.token.AccessToken {
						t.Errorf("unexpected save: file has %q, expected old %q", savedToken.AccessToken, tt.token.AccessToken)
					}
				}
			}
		})
	}
}
