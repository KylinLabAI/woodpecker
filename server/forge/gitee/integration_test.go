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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/server"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/gitee/fixtures"
	forge_types "go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

// mockGitee is a stand-in for the Gitee API v5 plus its OAuth endpoints. It
// serves exactly the paths that the Gitee forge talks to, so the acceptance
// journey can be exercised without a real Gitee account. The returned string
// pointer captures the ref that the File request was made with, so it can be
// asserted on the test goroutine instead of inside the handler.
func mockGitee(t *testing.T) (*httptest.Server, *string) {
	t.Helper()

	var fileRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			_ = r.ParseForm()
			if r.FormValue("grant_type") == "refresh_token" {
				_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","token_type":"bearer","expires_in":86400}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","token_type":"bearer","expires_in":86400}`))

		case r.URL.Path == "/api/v5/user":
			_, _ = w.Write([]byte(`{"id":4242,"login":"woodpecker","name":"Woodpecker CI","email":"ci@example.com","avatar_url":"https://gitee.com/a.png"}`))

		case r.URL.Path == "/api/v5/user/repos":
			_, _ = w.Write([]byte(fixtures.UserReposPayload))

		case r.URL.Path == "/api/v5/repos/123456":
			_, _ = w.Write([]byte(fixtures.RepoPayload))

		case r.URL.Path == "/api/v5/repos/kylin/woodpecker":
			_, _ = w.Write([]byte(fixtures.RepoPayload))

		case r.URL.Path == "/api/v5/repos/kylin/woodpecker/contents/.woodpecker.yaml":
			fileRef = r.URL.Query().Get("ref")
			_, _ = w.Write([]byte(fixtures.ContentPayload))

		case r.URL.Path == "/api/v5/repos/kylin/woodpecker/branches/main":
			_, _ = w.Write([]byte(fixtures.BranchPayload))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &fileRef
}

// NewJourneyClient wires a Gitee forge against the mock server. Server.Config
// is a package global, so the test is not marked parallel.
func newJourneyClient(t *testing.T, srv *httptest.Server) *Gitee {
	t.Helper()

	previous := server.Config.Server.OAuthHost
	server.Config.Server.OAuthHost = srv.URL
	t.Cleanup(func() { server.Config.Server.OAuthHost = previous })

	return &Gitee{
		url:               srv.URL,
		oAuthClientID:     "client-id",
		oAuthClientSecret: "client-secret",
	}
}

// Case 1: the forge reports its identity and the configured url; the login page
// renders its button and favicon from these.
func TestAcceptanceCase1ForgeIdentity(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	assert.Equal(t, "gitee", client.Name())
	assert.Equal(t, srv.URL, client.URL())
}

// Case 2: without a code, Login only yields the redirect url pointing at Gitee's
// authorize endpoint with the correct redirect_uri.
func TestAcceptanceCase2LoginRedirect(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	_, redirectURL, err := client.Login(t.Context(), &forge_types.OAuthRequest{State: "state"})
	require.NoError(t, err)

	assert.Contains(t, redirectURL, "/oauth/authorize")
	assert.Contains(t, redirectURL, "state=state")
	assert.Contains(t, redirectURL, "redirect_uri="+url.QueryEscape(srv.URL+"/authorize"))
}

// Case 3: with a code, Login exchanges it and returns a populated user.
func TestAcceptanceCase3LoginExchangesCode(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	user, redirectURL, err := client.Login(t.Context(), &forge_types.OAuthRequest{Code: "the-code"})
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.NotEmpty(t, redirectURL)

	assert.Equal(t, "woodpecker", user.Login)
	assert.Equal(t, "ci@example.com", user.Email)
	assert.Equal(t, model.ForgeRemoteID("4242"), user.ForgeRemoteID)
	assert.Equal(t, "at", user.AccessToken)
	assert.Equal(t, "rt", user.RefreshToken)
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), user.Expiry, 60)
}

// Case 4: repo sync returns the accessible repositories (public and private) and
// skips archived ones.
func TestAcceptanceCase4RepoSync(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	repos, err := client.Repos(t.Context(), &model.User{AccessToken: "at"}, &model.ListOptions{Page: 1})
	require.NoError(t, err)
	require.Len(t, repos, 2)

	assert.Equal(t, "kylin/woodpecker", repos[0].FullName)
	assert.True(t, repos[0].IsSCMPrivate)
	assert.Equal(t, "kylin/docs", repos[1].FullName)
	assert.False(t, repos[1].IsSCMPrivate)

	// The archived "kylin/old" must be excluded.
	for _, r := range repos {
		assert.NotEqual(t, "kylin/old", r.FullName)
	}
}

// Case 4: a single repo can be fetched by its remote id and by owner/name.
func TestAcceptanceCase4RepoFetch(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	byID, err := client.Repo(t.Context(), &model.User{AccessToken: "at"}, model.ForgeRemoteID("123456"), "kylin", "woodpecker")
	require.NoError(t, err)
	assert.Equal(t, "kylin/woodpecker", byID.FullName)

	byName, err := client.Repo(t.Context(), &model.User{AccessToken: "at"}, model.ForgeRemoteID(""), "kylin", "woodpecker")
	require.NoError(t, err)
	assert.Equal(t, "kylin/woodpecker", byName.FullName)
}

// Case 5: activating a repository must never error (a 500 would make the forge
// unusable).
func TestAcceptanceCase5Activate(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	err := client.Activate(context.Background(), &model.User{}, &model.Repo{FullName: "kylin/woodpecker"}, "https://hook")
	assert.NoError(t, err)
}

// Case 6/7: the pipeline configuration is fetched at the exact commit, the
// branch head is resolved (cron), and the netrc for cloning is produced.
func TestAcceptanceCase6FileBranchAndNetrc(t *testing.T) {
	srv, fileRef := mockGitee(t)
	client := newJourneyClient(t, srv)

	repo := &model.Repo{
		Owner:    "kylin",
		Name:     "woodpecker",
		FullName: "kylin/woodpecker",
		Clone:    "https://gitee.com/kylin/woodpecker.git",
	}
	user := &model.User{Login: "kylin", AccessToken: "at"}

	config, err := client.File(t.Context(), user, repo, &model.Pipeline{Commit: "deadbeef"}, ".woodpecker.yaml")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef", *fileRef, "File must be fetched at the pipeline commit")
	assert.Equal(t, "pipeline:\n  build:\n    image: golang", string(config))

	commit, err := client.BranchHead(t.Context(), user, repo, "main")
	require.NoError(t, err)
	assert.Equal(t, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b", commit.SHA)
	assert.Contains(t, commit.ForgeURL, "gitee.com")

	netrc, err := client.Netrc(user, repo)
	require.NoError(t, err)
	assert.Equal(t, "gitee.com", netrc.Machine)
	assert.Equal(t, "kylin", netrc.Login)
	assert.Equal(t, "at", netrc.Password)
}

// Case 8: a day later the token has expired, but Refresh obtains a fresh pair so
// sync and builds keep working without a new login.
func TestAcceptanceCase8Refresh(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	user := &model.User{
		AccessToken:  "old-at",
		RefreshToken: "old-rt",
		Expiry:       time.Now().Add(time.Hour).Unix(),
	}

	updated, err := client.Refresh(t.Context(), user)
	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, "new-at", user.AccessToken)
	assert.Equal(t, "new-rt", user.RefreshToken)
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), user.Expiry, 60)
}

// Full journey: login -> sync -> fetch -> activate -> build inputs -> refresh,
// matching the end-to-end acceptance checklist end to end.
func TestAcceptanceFullJourney(t *testing.T) {
	srv, _ := mockGitee(t)
	client := newJourneyClient(t, srv)

	user, _, err := client.Login(t.Context(), &forge_types.OAuthRequest{Code: "the-code"})
	require.NoError(t, err)
	require.NotNil(t, user)

	repos, err := client.Repos(t.Context(), user, &model.ListOptions{Page: 1})
	require.NoError(t, err)
	require.Len(t, repos, 2)

	repo := repos[0]
	require.NoError(t, client.Activate(t.Context(), user, repo, "https://hook"))

	config, err := client.File(t.Context(), user, repo, &model.Pipeline{Commit: "deadbeef"}, ".woodpecker.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, config)

	commit, err := client.BranchHead(t.Context(), user, repo, "main")
	require.NoError(t, err)
	assert.NotEmpty(t, commit.SHA)

	netrc, err := client.Netrc(user, repo)
	require.NoError(t, err)
	assert.NotEmpty(t, netrc.Machine)

	updated, err := client.Refresh(t.Context(), user)
	require.NoError(t, err)
	assert.True(t, updated)
}
