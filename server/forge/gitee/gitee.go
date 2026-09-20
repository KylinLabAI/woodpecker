// Copyright 2026 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gitee

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/woodpecker/v3/server"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge"
	forge_types "go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

const (
	authorizeTokenURL = "%s/oauth/authorize"
	accessTokenURL    = "%s/oauth/token"
)

// Gitee implements the Forge and the Refresher interface.
// The Refresher assertion matters as Gitee access tokens only live one day.
var (
	_ forge.Forge     = (*Gitee)(nil)
	_ forge.Refresher = (*Gitee)(nil)
)

// Gitee is the Forge implementation for https://gitee.com.
type Gitee struct {
	id                int64
	url               string
	oAuthClientID     string
	oAuthClientSecret string
	oAuthHost         string
	skipVerify        bool

	client     *http.Client
	clientOnce sync.Once
}

// Opts defines configuration options.
type Opts struct {
	URL               string // Gitee server url.
	OAuthClientID     string // OAuth2 Client ID
	OAuthClientSecret string // OAuth2 Client Secret
	OAuthHost         string // OAuth2 Host
	SkipVerify        bool   // Skip ssl verification.
}

// New returns a Forge implementation that integrates with Gitee.
// See https://gitee.com/api/v5/swagger.
func New(id int64, opts Opts) (forge.Forge, error) {
	return &Gitee{
		id:                id,
		url:               opts.URL,
		oAuthClientID:     opts.OAuthClientID,
		oAuthClientSecret: opts.OAuthClientSecret,
		oAuthHost:         opts.OAuthHost,
		skipVerify:        opts.SkipVerify,
	}, nil
}

// Name returns the string name of this driver.
func (c *Gitee) Name() string {
	return "gitee"
}

// URL returns the root url of a configured forge.
func (c *Gitee) URL() string {
	return c.url
}

// oauth2Config builds the oauth2 config of the Gitee endpoints.
func (c *Gitee) oauth2Config(ctx context.Context) (*oauth2.Config, context.Context) {
	publicOAuthURL := c.oAuthHost
	if publicOAuthURL == "" {
		publicOAuthURL = c.url
	}
	return &oauth2.Config{
			ClientID:     c.oAuthClientID,
			ClientSecret: c.oAuthClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  fmt.Sprintf(authorizeTokenURL, publicOAuthURL),
				TokenURL: fmt.Sprintf(accessTokenURL, c.url),
				// Gitee expects the credentials in the request body and does
				// not support basic auth on the token endpoint.
				AuthStyle: oauth2.AuthStyleInParams,
			},
			RedirectURL: fmt.Sprintf("%s/authorize", server.Config.Server.OAuthHost),
		},
		context.WithValue(ctx, oauth2.HTTPClient, c.httpClient())
}

// Login authenticates the user against Gitee.
// The first call has no code and only yields the url the user has to be
// redirected to, the second call exchanges the code for a token.
func (c *Gitee) Login(ctx context.Context, req *forge_types.OAuthRequest) (*model.User, string, error) {
	config, oauth2Ctx := c.oauth2Config(ctx)
	redirectURL := config.AuthCodeURL(req.State)

	if len(req.Code) == 0 {
		return nil, redirectURL, nil
	}

	token, err := config.Exchange(oauth2Ctx, req.Code)
	if err != nil {
		return nil, redirectURL, fmt.Errorf("oauth2 config exchange failed: %w", err)
	}

	account := new(User)
	if err := c.get(ctx, token.AccessToken, "/user", nil, account); err != nil {
		return nil, redirectURL, fmt.Errorf("fetching user info failed: %w", err)
	}
	if account.Login == "" {
		return nil, redirectURL, errors.New("gitee account has no login")
	}

	user := &model.User{
		AccessToken:   token.AccessToken,
		RefreshToken:  token.RefreshToken,
		Login:         account.Login,
		Email:         account.Email,
		ForgeRemoteID: model.ForgeRemoteID(fmt.Sprint(account.ID)),
		Avatar:        account.AvatarURL,
	}
	// Gitee always reports expires_in, keep the zero value otherwise so the
	// token is not treated as long expired.
	if !token.Expiry.IsZero() {
		user.Expiry = token.Expiry.UTC().Unix()
	}
	return user, redirectURL, nil
}

// Refresh refreshes the oauth2 token of the user.
// Gitee access tokens are only valid for one day, so this is not optional.
func (c *Gitee) Refresh(ctx context.Context, user *model.User) (bool, error) {
	if user.RefreshToken == "" {
		return false, nil
	}

	config, oauth2Ctx := c.oauth2Config(ctx)
	config.RedirectURL = ""

	source := config.TokenSource(oauth2Ctx, &oauth2.Token{
		AccessToken:  user.AccessToken,
		RefreshToken: user.RefreshToken,
		// Mark the token as expired, otherwise the oauth2 package hands back
		// the token it was given instead of refreshing it.
		Expiry: time.Now().Add(-time.Minute),
	})

	token, err := source.Token()
	if err != nil || len(token.AccessToken) == 0 {
		return false, err
	}

	user.AccessToken = token.AccessToken
	user.RefreshToken = token.RefreshToken
	if !token.Expiry.IsZero() {
		user.Expiry = token.Expiry.UTC().Unix()
	}
	return true, nil
}

// TODO(T4): fetch the teams of the user from the Gitee API.
func (c *Gitee) Teams(context.Context, *model.User, *model.ListOptions) ([]*model.Team, error) {
	return nil, forge_types.ErrNotImplemented
}

// Repo fetches a single repository of the Gitee API.
func (c *Gitee) Repo(ctx context.Context, u *model.User, remoteID model.ForgeRemoteID, owner, name string) (*model.Repo, error) {
	token := u.AccessToken

	// The remote id survives a rename, but Gitee may not resolve it, so fall
	// back to owner/name in that case.
	if remoteID.IsValid() && !strings.ContainsAny(string(remoteID), "/") && !strings.Contains(string(remoteID), "..") {
		repo := new(Repository)
		err := c.get(ctx, token, "/repos/"+url.PathEscape(string(remoteID)), nil, repo)
		if err == nil {
			return toRepo(repo), nil
		}
		if !isNotFound(err) {
			return nil, err
		}
		// Without owner/name we cannot look the repo up a second time.
		if owner == "" || name == "" {
			return nil, errors.Join(err, forge_types.ErrRepoNotFound)
		}
	}

	repo := new(Repository)
	// owner/name are single path segments; reject anything that could break
	// out of the /repos/ path (the Go http client unescapes %2F again).
	if strings.ContainsAny(owner+name, "/") || strings.Contains(owner+name, "..") {
		return nil, errors.Join(fmt.Errorf("invalid repository owner or name"), forge_types.ErrRepoNotFound)
	}
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(name))
	if err := c.get(ctx, token, path, nil, repo); err != nil {
		if isNotFound(err) {
			return nil, errors.Join(err, forge_types.ErrRepoNotFound)
		}
		return nil, err
	}
	return toRepo(repo), nil
}

// Repos fetches every repository the user has access to.
func (c *Gitee) Repos(ctx context.Context, u *model.User, p *model.ListOptions) ([]*model.Repo, error) {
	// Gitee is paged internally.
	if p != nil && p.Page != 1 {
		return nil, nil
	}

	repos, err := fetchAllPages[Repository](ctx, c, u.AccessToken, "/user/repos", nil)
	if err != nil {
		return nil, err
	}

	result := make([]*model.Repo, 0, len(repos))
	for i := range repos {
		if repos[i].Archived {
			continue
		}
		result = append(result, toRepo(&repos[i]))
	}
	return result, nil
}

// File fetches a single pipeline configuration file.
// It is fetched at the exact commit of the pipeline, not at a branch head.
func (c *Gitee) File(ctx context.Context, u *model.User, r *model.Repo, b *model.Pipeline, f string) ([]byte, error) {
	if u == nil {
		return nil, fmt.Errorf("no user for repository: %s", r.FullName)
	}
	if r == nil {
		return nil, fmt.Errorf("no repository for file fetch")
	}

	path := fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(r.Owner), url.PathEscape(r.Name), url.PathEscape(f))
	query := url.Values{}
	query.Set("ref", b.Commit)

	var raw json.RawMessage
	if err := c.get(ctx, u.AccessToken, path, query, &raw); err != nil {
		if isNotFound(err) {
			return nil, errors.Join(err, &forge_types.ErrConfigNotFound{Configs: []string{f}})
		}
		return nil, err
	}
	// Gitee returns a JSON array for a directory and an object for a file.
	if trimmed := strings.TrimSpace(string(raw)); len(trimmed) > 0 && trimmed[0] == '[' {
		return nil, errors.Join(errors.New("requested path is a directory"), &forge_types.ErrConfigNotFound{Configs: []string{f}})
	}

	content := new(Content)
	if err := json.Unmarshal(raw, content); err != nil {
		return nil, fmt.Errorf("could not decode content of %s in %s: %w", f, r.FullName, err)
	}

	decoded, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		return nil, fmt.Errorf("could not decode file %s of %s: %w", f, r.FullName, err)
	}
	return decoded, nil
}

// TODO(T17): fetch all files of a directory from the Gitee API.
func (c *Gitee) Dir(context.Context, *model.User, *model.Repo, *model.Pipeline, string) ([]*forge_types.FileMeta, error) {
	return nil, forge_types.ErrNotImplemented
}

// TODO(T15): report the pipeline status back to Gitee.
func (c *Gitee) Status(context.Context, *model.User, *model.Repo, *model.Pipeline, *model.Workflow) error {
	return nil
}

// TODO(T6): build the netrc credentials used to clone repos.
func (c *Gitee) Netrc(*model.User, *model.Repo) (*model.Netrc, error) {
	return nil, forge_types.ErrNotImplemented
}

// Activate is a no-op until WebHook support is implemented (T14).
// It must never return an error, otherwise repo activation fails with HTTP 500.
func (c *Gitee) Activate(context.Context, *model.User, *model.Repo, string) error {
	return nil
}

// Deactivate is a no-op until WebHook support is implemented (T14).
// It must never return an error, otherwise repo deactivation fails.
func (c *Gitee) Deactivate(context.Context, *model.User, *model.Repo, string) error {
	return nil
}

// TODO(T17): fetch the branches of a repo from the Gitee API.
func (c *Gitee) Branches(context.Context, *model.User, *model.Repo, *model.ListOptions) ([]string, error) {
	return nil, forge_types.ErrNotImplemented
}

// BranchHead returns the latest commit of a branch.
// It is required for the cron feature to work.
func (c *Gitee) BranchHead(ctx context.Context, u *model.User, r *model.Repo, branch string) (*model.Commit, error) {
	if u == nil {
		return nil, fmt.Errorf("no user for repository: %s", r.FullName)
	}
	if r == nil {
		return nil, fmt.Errorf("no repository for branch lookup")
	}

	path := fmt.Sprintf("/repos/%s/%s/branches/%s", url.PathEscape(r.Owner), url.PathEscape(r.Name), url.PathEscape(branch))

	branchData := new(Branch)
	if err := c.get(ctx, u.AccessToken, path, nil, branchData); err != nil {
		return nil, err
	}
	if branchData.Commit == nil {
		return nil, fmt.Errorf("branch %s of %s has no commit", branch, r.FullName)
	}

	return &model.Commit{
		SHA:      branchData.Commit.SHA,
		ForgeURL: branchData.Commit.URL,
	}, nil
}

// TODO(T16): fetch the open pull requests of a repo from the Gitee API.
func (c *Gitee) PullRequests(context.Context, *model.User, *model.Repo, *model.ListOptions) ([]*model.PullRequest, error) {
	return nil, forge_types.ErrNotImplemented
}

// TODO(T13): parse incoming Gitee WebHook requests.
func (c *Gitee) Hook(context.Context, *http.Request) (*model.Repo, *model.Pipeline, error) {
	return nil, nil, fmt.Errorf("gitee webhook not implemented")
}

// TODO(T17): check the membership of a user in an organization.
func (c *Gitee) OrgMembership(context.Context, *model.User, string) (*model.OrgPerm, error) {
	return nil, forge_types.ErrNotImplemented
}

// TODO(T17): fetch the details of an organization.
func (c *Gitee) Org(context.Context, *model.User, string) (*model.Org, error) {
	return nil, forge_types.ErrNotImplemented
}
