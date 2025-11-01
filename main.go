package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	manifestFormatHLS  = "hls"
	manifestFormatDASH = "dash"
)

type application struct {
	client *http.Client
}

type Token struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type Sessions struct {
	Events []Session `json:"sessions"`
}

type Session struct {
	ID         string `json:"id"`
	ResourceID string `json:"resource_id"`
	AccountID  string `json:"account_id"`
	StartTime  int    `json:"start_time"`
	EndTime    int    `json:"end_time"`
}

type PlaybackToken struct {
	Token string `json:"token"`
}

type URL struct {
	URL string `json:"url"`
}

func (app *application) generateToken(clientID, clientSecret string) (*Token, error) {
	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", clientID, clientSecret)))

	const url = "https://oauth.brightcove.com/v4/access_token"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("grant_type=client_credentials")))
	if err != nil {
		return nil, fmt.Errorf("error framing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+encodedCredentials)

	resp, err := app.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting response: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("received error from API with status %d and error %v", resp.StatusCode, string(body))
	}

	var token Token
	if err = json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("error decoding body: %w", err)
	}

	return &token, nil
}

func (app *application) getSessions(token, playbackURL string) (*Sessions, string, error) {
	// playbackURL should be of format https://fastly.live.brightcove.com/6384185469112/ap-south-1/6415518627001/eyJyui.../playlist-hls.m3u8
	// parsedURL.Path would be would be /6384185469112/ap-south-1/6415518627001/eyJyui.../playlist-hls.m3u8
	// pathParts[1] = VideoID/JobID/ResourceID pathParts[3] = AccountID
	parsedURL, err := url.Parse(playbackURL)
	if err != nil {
		return nil, "", fmt.Errorf("error parsing playbackURL: %w", err)
	}
	pathParts := strings.Split(parsedURL.Path, "/")
	if len(pathParts) < 6 {
		return nil, "", errors.New("malformed playback URL provided")
	}
	var resourceID = pathParts[1]

	url := fmt.Sprintf("https://api.live.brightcove.com/v2/accounts/%s/sessions/resource/%s", pathParts[3], pathParts[1])
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error framing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error getting response: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("received error from API with status %d and error %v", resp.StatusCode, string(body))
	}

	var sessions Sessions
	err = json.NewDecoder(resp.Body).Decode(&sessions)
	if err != nil {
		return nil, "", fmt.Errorf("error decoding body: %w", err)
	}

	return &sessions, resourceID, nil
}

func (app *application) generatePlaybackToken(sessions *Sessions, token string) ([]PlaybackToken, error) {
	var url string
	var playbackTokens []PlaybackToken
	if len(sessions.Events) > 0 {
		session := sessions.Events[0]
		url = fmt.Sprintf("https://api.live.brightcove.com/v2/accounts/%s/playback/%s/token", session.AccountID, session.ResourceID)
	} else {
		return nil, errors.New("no events in session, quitting")
	}

	for _, session := range sessions.Events {
		data := struct {
			StartTime      string `json:"start_time"`
			EndTime        string `json:"end_time"`
			ManifestFormat string `json:"manifest_format"`
		}{
			StartTime:      strconv.Itoa(session.StartTime),
			EndTime:        strconv.Itoa(session.EndTime),
			ManifestFormat: manifestFormatHLS,
		}
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(data)
		if err != nil {
			return nil, fmt.Errorf("error encoding JSON: %w", err)
		}

		req, err := http.NewRequest(http.MethodPost, url, &buf)
		if err != nil {
			return nil, fmt.Errorf("error framing request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := app.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error getting response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("received error from API with status %d and error %v", resp.StatusCode, string(body))
		}

		var token PlaybackToken
		err = json.NewDecoder(resp.Body).Decode(&token)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("error decoding body: %w", err)
		}

		playbackTokens = append(playbackTokens, token)
	}

	return playbackTokens, nil
}

func (app *application) generatePlaybackURL(tokens []PlaybackToken, resourceID string) ([]URL, error) {
	var playbackURLs []URL
	for _, token := range tokens {
		url := fmt.Sprintf("https://api.live.brightcove.com/v2/playback/%s?pt=%s", resourceID, token.Token)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("error framing request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error getting response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("received error from API with status %d and error %v", resp.StatusCode, string(body))
		}

		var playbackURL URL
		err = json.NewDecoder(resp.Body).Decode(&playbackURL)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("error decoding body: %w", err)
		}

		playbackURLs = append(playbackURLs, playbackURL)
	}

	return playbackURLs, nil
}

func main() {
	args := os.Args
	if len(args) == 1 {
		fmt.Println("Usage: ./timeshifturls <PLAYBACK_URL>")
		os.Exit(0)
	}
	playbackURL := args[1]

	if err := godotenv.Load(); err != nil {
		log.Println("error loading .env", err)
		os.Exit(1)
	}
	clientID := os.Getenv("CLIENT_ID")
	clientSecret := os.Getenv("CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Println("client credentials missing")
		os.Exit(1)
	}

	app := application{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	token, err := app.generateToken(clientID, clientSecret)
	if err != nil {
		log.Println("error generating access token:", err)
		os.Exit(1)
	}

	sessions, resourceID, err := app.getSessions(token.AccessToken, playbackURL)
	if err != nil {
		log.Println("error getting sessions:", err)
		os.Exit(1)
	}

	playbackTokens, err := app.generatePlaybackToken(sessions, token.AccessToken)
	if err != nil {
		log.Println("error creating playback token:", err)
		os.Exit(1)
	}

	playbackURLs, err := app.generatePlaybackURL(playbackTokens, resourceID)
	if err != nil {
		log.Println("error generating playback urls:", err)
		os.Exit(1)
	}

	for i, url := range playbackURLs {
		fmt.Printf("\nVOD URL[%d]: %s\n", i, url.URL)
	}
	fmt.Println()
}
