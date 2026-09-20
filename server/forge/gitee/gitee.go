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
	"fmt"
	"net/http"
	"sync"

	"go.woodpecker-ci.org/woodpecker/v3/server/forge"
	forge_types "go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
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

// TODO(T3): implement OAuth2 login.
func (c *Gitee) Login(context.Context, *forge_types.OAuthRequest) (*model.User, string, error) {
	return nil, "", forge_types.ErrNotImplemented
}

// TODO(T3): implement OAuth2 token refresh.
func (c *Gitee) Refresh(context.Context, *model.User) (bool, error) {
	return false, forge_types.ErrNotImplemented
}

// TODO(T4): fetch the teams of the user from the Gitee API.
func (c *Gitee) Teams(context.Context, *model.User, *model.ListOptions) ([]*model.Team, error) {
	return nil, forge_types.ErrNotImplemented
}

// TODO(T4): fetch a single repo from the Gitee API.
func (c *Gitee) Repo(context.Context, *model.User, model.ForgeRemoteID, string, string) (*model.Repo, error) {
	return nil, forge_types.ErrNotImplemented
}

// TODO(T4): fetch all repos of the user from the Gitee API.
func (c *Gitee) Repos(context.Context, *model.User, *model.ListOptions) ([]*model.Repo, error) {
	return nil, forge_types.ErrNotImplemented
}

// TODO(T5): fetch a single file of a repo from the Gitee API.
func (c *Gitee) File(context.Context, *model.User, *model.Repo, *model.Pipeline, string) ([]byte, error) {
	return nil, forge_types.ErrNotImplemented
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

// TODO(T5): fetch the head commit of a branch from the Gitee API.
func (c *Gitee) BranchHead(context.Context, *model.User, *model.Repo, string) (*model.Commit, error) {
	return nil, forge_types.ErrNotImplemented
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
