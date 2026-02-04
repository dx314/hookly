package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubAPIURL       = "https://api.github.com"
)

// GitHubClient handles GitHub OAuth operations.
type GitHubClient struct {
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
}

// GitHubUser represents a GitHub user.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	Name      string `json:"name"`
	Email     string `json:"email"`
}

// GitHubToken represents an OAuth access token response.
type GitHubToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// NewGitHubClient creates a new GitHub OAuth client.
func NewGitHubClient(clientID, clientSecret, redirectURI string) *GitHubClient {
	return &GitHubClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetAuthURL returns the GitHub authorization URL with the given state.
func (c *GitHubClient) GetAuthURL(state string) string {
	params := url.Values{
		"client_id":    {c.clientID},
		"redirect_uri": {c.redirectURI},
		"scope":        {"read:org"}, // Need read:org for org membership check
		"state":        {state},
	}
	return githubAuthorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for an access token.
func (c *GitHubClient) ExchangeCode(ctx context.Context, code string) (*GitHubToken, error) {
	data := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var token GitHubToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	if token.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	return &token, nil
}

// GetUser retrieves the authenticated user's information.
func (c *GitHubClient) GetUser(ctx context.Context, accessToken string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIURL+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user failed: %s", string(body))
	}

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}

	return &user, nil
}

// CheckOrgMembership checks if the user is a member of the specified organization.
func (c *GitHubClient) CheckOrgMembership(ctx context.Context, accessToken, org string) (bool, error) {
	// GET /user/memberships/orgs/{org} returns 200 if member, 404 if not
	reqURL := fmt.Sprintf("%s/user/memberships/orgs/%s", githubAPIURL, url.PathEscape(org))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("check org membership: %w", err)
	}
	defer resp.Body.Close()

	// Drain body
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound, http.StatusForbidden:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
}
