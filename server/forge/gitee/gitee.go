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

	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/woodpecker/v3/server"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/common"
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
			// user_info + projects are needed to log in and list repos; hook and
			// pull_requests are required for the Stage 2 webhook and PR features.
			Scopes: []string{"user_info", "projects", "hook", "pull_requests"},
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

// Teams returns the organizations the user is a member of.
func (c *Gitee) Teams(ctx context.Context, u *model.User, p *model.ListOptions) ([]*model.Team, error) {
	token := common.UserToken(ctx, nil, u)

	var orgs []apiOrg
	if err := c.get(ctx, token, "/user/orgs", nil, &orgs); err != nil {
		return nil, err
	}

	teams := make([]*model.Team, 0, len(orgs))
	for i := range orgs {
		teams = append(teams, &model.Team{
			Login:  orgs[i].Login,
			Avatar: "",
		})
	}
	return teams, nil
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

// Dir fetches every file of a directory at the pipeline commit and returns their
// pipeline configuration content.
func (c *Gitee) Dir(ctx context.Context, u *model.User, r *model.Repo, b *model.Pipeline, dirName string) ([]*forge_types.FileMeta, error) {
	if u == nil {
		return nil, fmt.Errorf("no user for repository: %s", r.FullName)
	}
	if r == nil {
		return nil, fmt.Errorf("no repository for directory fetch")
	}

	path := fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(r.Owner), url.PathEscape(r.Name), url.PathEscape(dirName))
	query := url.Values{}
	query.Set("ref", b.Commit)

	var entries []FileEntry
	if err := c.get(ctx, u.AccessToken, path, query, &entries); err != nil {
		if isNotFound(err) {
			return nil, errors.Join(err, &forge_types.ErrConfigNotFound{Configs: []string{dirName}})
		}
		return nil, err
	}

	files := make([]*forge_types.FileMeta, 0, len(entries))
	for i := range entries {
		if entries[i].Type != "file" {
			continue
		}
		content, err := c.File(ctx, u, r, b, entries[i].Path)
		if err != nil {
			return nil, err
		}
		files = append(files, &forge_types.FileMeta{
			Name: entries[i].Path,
			Data: content,
		})
	}
	return files, nil
}

// Status reports the pipeline status back to Gitee as a commit status. Failures
// are logged only and never block the pipeline.
func (c *Gitee) Status(ctx context.Context, u *model.User, r *model.Repo, b *model.Pipeline, w *model.Workflow) error {
	token := common.UserToken(ctx, r, u)

	body := map[string]string{
		"context":     common.GetPipelineStatusContext(r, b, w),
		"description": common.GetPipelineStatusDescription(w.State),
		"state":       getStatus(w.State),
		"target_url":  common.GetPipelineStatusURL(r, b, w),
	}

	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", url.PathEscape(r.Owner), url.PathEscape(r.Name), url.PathEscape(b.Commit))
	if err := c.post(ctx, token, path, body, nil); err != nil {
		log.Error().Err(err).Msgf("could not update status for %s#%s", r.FullName, b.Commit)
		return nil
	}
	return nil
}

// Netrc builds the .netrc credentials used to clone repositories.
// A nil user is passed for public repos, in that case an empty credential is
// returned so the agent can still clone without authentication.
func (c *Gitee) Netrc(u *model.User, r *model.Repo) (*model.Netrc, error) {
	if r == nil {
		return nil, fmt.Errorf("no repository for netrc generation")
	}

	login := ""
	token := ""
	if u != nil {
		login = u.Login
		token = u.AccessToken
	}

	host, err := common.ExtractHostFromCloneURL(r.Clone)
	if err != nil {
		return nil, err
	}

	return &model.Netrc{
		Login:    login,
		Password: token,
		Machine:  host,
		Type:     model.ForgeTypeGitee,
	}, nil
}

// Activate creates a webhook pointing at Woodpecker so Gitee can deliver events.
// The webhook secret is taken from the repository hash.
func (c *Gitee) Activate(ctx context.Context, u *model.User, r *model.Repo, link string) error {
	hook := map[string]any{
		"url":      link,
		"password": r.Hash,
		"events":   []string{"push", "tag_push", "pull_request"},
	}

	created := new(Hook)
	path := fmt.Sprintf("/repos/%s/%s/hooks", url.PathEscape(r.Owner), url.PathEscape(r.Name))
	if err := c.post(ctx, u.AccessToken, path, hook, created); err != nil {
		return err
	}
	return nil
}

// Deactivate removes the Woodpecker webhook. A missing webhook is ignored, not
// treated as an error, so deactivation succeeds even after a manual removal.
// Only webhooks that point back at this Woodpecker instance are removed.
func (c *Gitee) Deactivate(ctx context.Context, u *model.User, r *model.Repo, link string) error {
	var hooks []Hook
	path := fmt.Sprintf("/repos/%s/%s/hooks", url.PathEscape(r.Owner), url.PathEscape(r.Name))
	if err := c.get(ctx, u.AccessToken, path, nil, &hooks); err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}

	for _, hook := range hooks {
		if hook.URL == "" || !hookBelongsToInstance(hook.URL, link) {
			continue
		}
		delPath := fmt.Sprintf("/repos/%s/%s/hooks/%d", url.PathEscape(r.Owner), url.PathEscape(r.Name), hook.ID)
		if err := c.delete(ctx, u.AccessToken, delPath); err != nil {
			if isNotFound(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// hookBelongsToInstance reports whether the given webhook url points back at the
// Woodpecker instance identified by link (the webhook url registered at
// activation). The comparison is host based so it tolerates differences in the
// query string (e.g. the access_token).
func hookBelongsToInstance(hookURL, link string) bool {
	linkURL, err := url.Parse(link)
	if err != nil || linkURL.Host == "" {
		return hookURL == link
	}
	return strings.Contains(hookURL, linkURL.Host)
}

// Branches returns the names of all branches of the repository.
func (c *Gitee) Branches(ctx context.Context, u *model.User, r *model.Repo, p *model.ListOptions) ([]string, error) {
	token := common.UserToken(ctx, r, u)

	path := fmt.Sprintf("/repos/%s/%s/branches", url.PathEscape(r.Owner), url.PathEscape(r.Name))
	var branches []Branch
	if err := c.get(ctx, token, path, nil, &branches); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(branches))
	for i := range branches {
		names = append(names, branches[i].Name)
	}
	return names, nil
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

// PullRequests returns the open pull requests of the repository.
func (c *Gitee) PullRequests(ctx context.Context, u *model.User, r *model.Repo, p *model.ListOptions) ([]*model.PullRequest, error) {
	token := common.UserToken(ctx, r, u)

	query := url.Values{}
	query.Set("state", "open")
	path := fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(r.Owner), url.PathEscape(r.Name))

	var prs []apiPullRequest
	if err := c.get(ctx, token, path, query, &prs); err != nil {
		return nil, err
	}

	result := make([]*model.PullRequest, 0, len(prs))
	for i := range prs {
		result = append(result, &model.PullRequest{
			Index: model.ForgeRemoteID(fmt.Sprint(prs[i].Number)),
			Title: prs[i].Title,
		})
	}
	return result, nil
}

// OrgMembership checks whether the user is a member of the given organization.
func (c *Gitee) OrgMembership(ctx context.Context, u *model.User, org string) (*model.OrgPerm, error) {
	path := fmt.Sprintf("/orgs/%s/members/%s", url.PathEscape(org), url.PathEscape(u.Login))

	member := new(apiOrgMember)
	if err := c.get(ctx, u.AccessToken, path, nil, member); err != nil {
		if isNotFound(err) {
			return &model.OrgPerm{}, nil
		}
		return nil, err
	}

	// Gitee does not expose the admin role through the membership endpoint in a
	// structured way, so treat every member as a non-admin by default.
	return &model.OrgPerm{Member: true, Admin: false}, nil
}

// Org fetches the details of an organization or user.
func (c *Gitee) Org(ctx context.Context, u *model.User, org string) (*model.Org, error) {
	path := fmt.Sprintf("/orgs/%s", url.PathEscape(org))

	apiOrgData := new(apiOrg)
	if err := c.get(ctx, u.AccessToken, path, nil, apiOrgData); err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		// The identifier might be a user instead of an organization.
		userData := new(User)
		if err := c.get(ctx, u.AccessToken, "/users/"+url.PathEscape(org), nil, userData); err != nil {
			if isNotFound(err) {
				return nil, fmt.Errorf("could not find organization or user %q", org)
			}
			return nil, err
		}
		return &model.Org{
			Name:   userData.Login,
			IsUser: true,
		}, nil
	}

	return &model.Org{
		ForgeID: apiOrgData.ID,
		Name:    apiOrgData.Login,
		IsUser:  false,
	}, nil
}

// getStatus maps a Woodpecker workflow state to a Gitee commit status state.
func getStatus(state model.StatusValue) string {
	switch state {
	case model.StatusSuccess, model.StatusSkipped, model.StatusBlocked:
		return "success"
	case model.StatusFailure, model.StatusCanceled, model.StatusDeclined:
		return "failure"
	case model.StatusError, model.StatusKilled:
		return "error"
	default:
		return "pending"
	}
}
