package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

func NewGCSClient(ctx context.Context, googleServiceAccountCredentialFile, googleOAuthClientCredentialFile string) (*storage.Client, error) {
	if len(googleServiceAccountCredentialFile) > 0 {
		return storage.NewClient(ctx,
			option.WithCredentialsFile(googleServiceAccountCredentialFile),
		)
	}

	b, err := os.ReadFile(googleOAuthClientCredentialFile)
	if err != nil {
		return nil, err
	}

	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/bigquery")
	if err != nil {
		return nil, err
	}
	token := getToken(config)

	return storage.NewClient(ctx,
		option.WithTokenSource(oauth2.StaticTokenSource(token)),
	)

}

// Retrieve a token, saves the token, then returns the generated client.
func getToken(config *oauth2.Config) *oauth2.Token {
	tokenDir := os.Getenv("HOME")
	if len(tokenDir) == 0 {
		tokenDir = "./"
	}
	tokFile := filepath.Join(tokenDir, "gcp-token.json")
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return tok
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Errorf("Unable to read authorization code: %v", err)
		return nil
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Errorf("Unable to retrieve token from web: %v", err)
		return nil
	}

	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		log.Errorf("Unable to cache oauth token: %v", err)
		return
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(token); err != nil {
		log.Error(err.Error())
	}
}
