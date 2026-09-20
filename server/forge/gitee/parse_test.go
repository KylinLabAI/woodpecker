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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/server/forge/gitee/fixtures"
	forge_types "go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

func TestToRepo(t *testing.T) {
	t.Parallel()

	from := new(Repository)
	require.NoError(t, json.Unmarshal([]byte(fixtures.RepoPayload), from))

	repo := toRepo(from)

	assert.Equal(t, model.ForgeRemoteID("123456"), repo.ForgeRemoteID)
	assert.Equal(t, "kylin/woodpecker", repo.FullName)
	assert.Equal(t, "woodpecker", repo.Name)
	assert.Equal(t, "kylin", repo.Owner)
	assert.Equal(t, "https://gitee.com/avatars/42.png", repo.Avatar)
	// Gitee has no Repo.Link, the url belongs into ForgeURL
	assert.Equal(t, "https://gitee.com/kylin/woodpecker", repo.ForgeURL)
	assert.Equal(t, "https://gitee.com/kylin/woodpecker.git", repo.Clone)
	assert.Equal(t, "git@gitee.com:kylin/woodpecker.git", repo.CloneSSH)
	assert.Equal(t, "main", repo.Branch)
	// Gitee has no Repo.IsPrivate
	assert.True(t, repo.IsSCMPrivate)
	assert.Equal(t, model.VisibilityPrivate, repo.Visibility)
	assert.True(t, repo.PREnabled)

	require.NotNil(t, repo.Perm)
	assert.True(t, repo.Perm.Pull)
	assert.True(t, repo.Perm.Push)
	assert.True(t, repo.Perm.Admin)
}

func TestToRepoPublicWithoutOwnerAndPermissions(t *testing.T) {
	t.Parallel()

	from := new(Repository)
	require.NoError(t, json.Unmarshal([]byte(fixtures.PublicRepoPayload), from))

	repo := toRepo(from)

	assert.Equal(t, "kylin", repo.Owner, "the owner falls back to the full name owner part")
	assert.Equal(t, model.VisibilityPublic, repo.Visibility)
	assert.False(t, repo.IsSCMPrivate)

	require.NotNil(t, repo.Perm)
	assert.True(t, repo.Perm.Pull, "a repo that can be seen can always be pulled")
	assert.False(t, repo.Perm.Push)
	assert.False(t, repo.Perm.Admin)
}

func TestToPermPushImpliesPull(t *testing.T) {
	t.Parallel()

	perm := toPerm(&Permissions{Push: true})

	assert.True(t, perm.Pull)
	assert.True(t, perm.Push)
	assert.False(t, perm.Admin)
}

func TestRepos(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v5/user/repos", r.URL.Path)
		assert.Equal(t, "token", r.URL.Query().Get("access_token"))

		if r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(fixtures.UserReposPayload))
	}))
	t.Cleanup(srv.Close)

	repos, err := (&Gitee{url: srv.URL}).Repos(t.Context(), &model.User{AccessToken: "token"}, &model.ListOptions{Page: 1})

	require.NoError(t, err)
	require.Len(t, repos, 2, "archived repos have to be skipped")

	assert.Equal(t, "kylin/woodpecker", repos[0].FullName)
	assert.True(t, repos[0].IsSCMPrivate)
	assert.Equal(t, "kylin/docs", repos[1].FullName)
	assert.False(t, repos[1].IsSCMPrivate)
	assert.False(t, repos[1].Perm.Admin)
}

func TestReposSkipsOtherPages(t *testing.T) {
	t.Parallel()

	repos, err := (&Gitee{url: "https://gitee.com"}).Repos(t.Context(), &model.User{}, &model.ListOptions{Page: 2})

	require.NoError(t, err)
	assert.Empty(t, repos)
}

func TestReposPropagatesError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	repos, err := (&Gitee{url: srv.URL}).Repos(t.Context(), &model.User{AccessToken: "bad"}, &model.ListOptions{Page: 1})

	require.Error(t, err)
	assert.Nil(t, repos)
}

func TestRepoByRemoteID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v5/repos/123456", r.URL.Path)
		_, _ = w.Write([]byte(fixtures.RepoPayload))
	}))
	t.Cleanup(srv.Close)

	repo, err := (&Gitee{url: srv.URL}).Repo(t.Context(), &model.User{AccessToken: "token"}, "123456", "kylin", "renamed")

	require.NoError(t, err)
	assert.Equal(t, "kylin/woodpecker", repo.FullName)
}

func TestRepoFallsBackToOwnerAndName(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gitee does not resolve the remote id, the owner/name lookup has to run
		if r.URL.Path == "/api/v5/repos/123456" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.Equal(t, "/api/v5/repos/kylin/woodpecker", r.URL.Path)
		_, _ = w.Write([]byte(fixtures.RepoPayload))
	}))
	t.Cleanup(srv.Close)

	repo, err := (&Gitee{url: srv.URL}).Repo(t.Context(), &model.User{AccessToken: "token"}, "123456", "kylin", "woodpecker")

	require.NoError(t, err)
	assert.Equal(t, model.ForgeRemoteID("123456"), repo.ForgeRemoteID)
}

func TestRepoNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)

	repo, err := (&Gitee{url: srv.URL}).Repo(t.Context(), &model.User{AccessToken: "token"}, "", "kylin", "missing")

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.True(t, errors.Is(err, forge_types.ErrRepoNotFound))
}

func TestRepoPropagatesOtherErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	repo, err := (&Gitee{url: srv.URL}).Repo(t.Context(), &model.User{AccessToken: "token"}, "", "kylin", "boom")

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.False(t, errors.Is(err, forge_types.ErrRepoNotFound))
}

func TestRepoRejectsTraversalInOwnerOrName(t *testing.T) {
	t.Parallel()

	// The handler must never be hit: a path traversal attempt is rejected
	// before a request is built.
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	t.Cleanup(srv.Close)

	_, err := (&Gitee{url: srv.URL}).Repo(t.Context(), &model.User{AccessToken: "token"}, "", "o/evil", "../x")

	require.Error(t, err)
	assert.False(t, hit)
	assert.True(t, errors.Is(err, forge_types.ErrRepoNotFound))
}

func TestRepoRemoteIDNotFoundWithoutOwner(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)

	repo, err := (&Gitee{url: srv.URL}).Repo(t.Context(), &model.User{AccessToken: "token"}, "123456", "", "")

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.True(t, errors.Is(err, forge_types.ErrRepoNotFound))
}
