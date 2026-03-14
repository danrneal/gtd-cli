package googletasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"
)

// NewService creates a new Google Tasks service.
// It authenticates using the provided credentials file and token file.
// If the token file does not exist, it triggers the web authentication flow.
func NewService(ctx context.Context, credsFile, tokenFile string) (*tasks.Service, error) {
	creds, err := os.ReadFile(credsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %w", err)
	}

	config, err := google.ConfigFromJSON(creds, tasks.TasksScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %w", err)
	}

	client, err := clientFromConfig(ctx, config, tokenFile)
	if err != nil {
		return nil, err
	}

	service, err := tasks.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create tasks service: %w", err)
	}

	return service, nil
}

// clientFromConfig retrieves a token, saves the token, then returns the generated client.
func clientFromConfig(ctx context.Context, config *oauth2.Config, tokenFile string) (*http.Client, error) {
	token, err := tokenFromFile(tokenFile)
	if err != nil {
		token, err = tokenFromWeb(ctx, config)
		if err != nil {
			return nil, err
		}

		if err := saveToken(tokenFile, token); err != nil {
			return nil, err
		}
	}

	tokenSource := config.TokenSource(ctx, token)
	tc := &fileTokenSource{
		tokenSource: tokenSource,
		tokenFile:   tokenFile,
		token:       token,
	}

	return oauth2.NewClient(ctx, tc), nil
}

// tokenFromFile retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("could not open token file: %w", err)
	}

	defer f.Close()

	token := &oauth2.Token{}
	if err = json.NewDecoder(f).Decode(token); err != nil {
		return nil, fmt.Errorf("could not decode token from file %q: %w", file, err)
	}

	return token, nil
}

// tokenFromWeb requests a token from the web, then returns the retrieved token.
func tokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	authCodeURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("Go to the following link in your browser then type the authorization code: \n%v\n", authCodeURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	token, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %w", err)
	}

	return token, nil
}

// saveToken saves a token to a file path.
func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("unable to cache oauth token: %w", err)
	}

	if err := json.NewEncoder(f).Encode(token); err != nil {
		_ = f.Close()
		return fmt.Errorf("could not encode token: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("could not close token file: %w", err)
	}

	return nil
}

// fileTokenSource is a wrapper around oauth2.TokenSource that saves the token to a file whenever it's refreshed.
type fileTokenSource struct {
	tokenSource oauth2.TokenSource
	tokenFile   string
	token       *oauth2.Token
}

// Token returns a token from the underlying source, saving it if it has been refreshed.
func (fts *fileTokenSource) Token() (*oauth2.Token, error) {
	token, err := fts.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token from source: %w", err)
	}

	if fts.token == nil || token.AccessToken != fts.token.AccessToken {
		fts.token = token
		if err := saveToken(fts.tokenFile, token); err != nil {
			log.Printf("failed to save new token: %v", err)
		}
	}

	return token, nil
}
