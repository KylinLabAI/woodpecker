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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/server"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/gitee/fixtures"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

// captured holds values posted to the mock server so they can be asserted on the
// test goroutine.
type captured struct {
	hookURL      string
	hookPassword string
	statusState  string
}

// mockStage2 is a mock Gitee API that serves the Stage 2 (T14-T17) endpoints.
func mockStage2(t *testing.T) (*httptest.Server, *captured) {
	t.Helper()
	cap := &captured{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path

		switch {
		case r.Method == http.MethodPost && p == "/api/v5/repos/kylin/woodpecker/hooks":
			body, _ := io.ReadAll(r.Body)
			var hook map[string]any
			_ = json.Unmarshal(body, &hook)
			cap.hookURL, _ = hook["url"].(string)
			cap.hookPassword, _ = hook["password"].(string)
			_, _ = w.Write([]byte(`{"id":1,"url":"` + cap.hookURL + `","password":"` + cap.hookPassword + `"}`))

		case r.Method == http.MethodGet && p == "/api/v5/repos/kylin/woodpecker/hooks":
			_, _ = w.Write([]byte(`[{"id":1,"url":"http://woodpecker.test/api/hook?access_token=abc","password":"secret"}]`))

		case r.Method == http.MethodDelete && strings.HasPrefix(p, "/api/v5/repos/kylin/woodpecker/hooks/"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/v5/repos/kylin/woodpecker/statuses/"):
			body, _ := io.ReadAll(r.Body)
			var status map[string]any
			_ = json.Unmarshal(body, &status)
			if s, ok := status["state"].(string); ok {
				cap.statusState = s
			}
			w.WriteHeader(http.StatusCreated)

		case r.Method == http.MethodGet && p == "/api/v5/repos/kylin/woodpecker/branches":
			_, _ = w.Write([]byte(`[{"name":"main"},{"name":"dev"}]`))

		case r.Method == http.MethodGet && p == "/api/v5/repos/kylin/woodpecker/pulls":
			_, _ = w.Write([]byte(`[{"number":1,"title":"Add feature"},{"number":2,"title":"Fix bug"}]`))

		case r.Method == http.MethodGet && p == "/api/v5/user/orgs":
			_, _ = w.Write([]byte(`[{"login":"myorg","id":99}]`))

		case r.Method == http.MethodGet && p == "/api/v5/orgs/myorg":
			_, _ = w.Write([]byte(`{"login":"myorg","id":99}`))

		case r.Method == http.MethodGet && p == "/api/v5/orgs/myorg/members/kylin":
			_, _ = w.Write([]byte(`{"login":"kylin","state":"active"}`))

		case r.Method == http.MethodGet && p == "/api/v5/orgs/empty/members/kylin":
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodGet && p == "/api/v5/orgs/unknown":
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodGet && p == "/api/v5/users/myuser":
			_, _ = w.Write([]byte(`{"id":7,"login":"myuser"}`))

		case r.Method == http.MethodGet && p == "/api/v5/repos/kylin/woodpecker/contents/.woodpecker":
			_, _ = w.Write([]byte(`[{"type":"file","name":".woodpecker.yaml","path":".woodpecker/.woodpecker.yaml"},{"type":"dir","name":"sub"}]`))

		case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/v5/repos/kylin/woodpecker/contents/"):
			_, _ = w.Write([]byte(fixtures.ContentPayload))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func stage2Client(t *testing.T, srv *httptest.Server) *Gitee {
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

func stage2Repo() *model.Repo {
	return &model.Repo{
		Owner:    "kylin",
		Name:     "woodpecker",
		FullName: "kylin/woodpecker",
		Clone:    "https://gitee.com/kylin/woodpecker.git",
		Hash:     "repo-hash",
	}
}

// T14: Activate registers a webhook carrying the repo hash as password.
func TestStage2Activate(t *testing.T) {
	srv, cap := mockStage2(t)
	client := stage2Client(t, srv)

	err := client.Activate(context.Background(), &model.User{AccessToken: "at"}, stage2Repo(), "http://woodpecker.test/api/hook?access_token=abc")
	require.NoError(t, err)
	assert.Equal(t, "http://woodpecker.test/api/hook?access_token=abc", cap.hookURL)
	assert.Equal(t, "repo-hash", cap.hookPassword)
}

// T14: Deactivate deletes the existing webhook without erroring.
func TestStage2Deactivate(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	err := client.Deactivate(context.Background(), &model.User{AccessToken: "at"}, stage2Repo(), "http://woodpecker.test")
	require.NoError(t, err)
}

// T15: Status reports the workflow state to Gitee.
func TestStage2Status(t *testing.T) {
	srv, cap := mockStage2(t)
	client := stage2Client(t, srv)

	repo := stage2Repo()
	repo.Owner = "kylin"
	repo.Name = "woodpecker"
	err := client.Status(context.Background(), &model.User{AccessToken: "at"}, repo,
		&model.Pipeline{Commit: "deadbeef"}, &model.Workflow{State: model.StatusSuccess})
	require.NoError(t, err)
	assert.Equal(t, "success", cap.statusState)
}

// T15: a failing status update is logged but never blocks the pipeline.
func TestStage2StatusDoesNotBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	client := stage2Client(t, srv)

	err := client.Status(context.Background(), &model.User{AccessToken: "at"}, stage2Repo(),
		&model.Pipeline{Commit: "deadbeef"}, &model.Workflow{State: model.StatusFailure})
	assert.NoError(t, err)
}

// T16: PullRequests returns the open pull requests.
func TestStage2PullRequests(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	prs, err := client.PullRequests(context.Background(), &model.User{AccessToken: "at"}, stage2Repo(), &model.ListOptions{Page: 1})
	require.NoError(t, err)
	require.Len(t, prs, 2)
	assert.Equal(t, "Add feature", prs[0].Title)
	assert.Equal(t, model.ForgeRemoteID("1"), prs[0].Index)
}

// T17: Teams returns the user's organizations.
func TestStage2Teams(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	teams, err := client.Teams(context.Background(), &model.User{AccessToken: "at"}, &model.ListOptions{Page: 1})
	require.NoError(t, err)
	require.Len(t, teams, 1)
	assert.Equal(t, "myorg", teams[0].Login)
}

// T17: Branches returns the branch names.
func TestStage2Branches(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	branches, err := client.Branches(context.Background(), &model.User{AccessToken: "at"}, stage2Repo(), &model.ListOptions{Page: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "dev"}, branches)
}

// T17: Org resolves an organization and a user fallback.
func TestStage2Org(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	org, err := client.Org(context.Background(), &model.User{AccessToken: "at"}, "myorg")
	require.NoError(t, err)
	assert.Equal(t, "myorg", org.Name)
	assert.False(t, org.IsUser)

	user, err := client.Org(context.Background(), &model.User{AccessToken: "at"}, "myuser")
	require.NoError(t, err)
	assert.Equal(t, "myuser", user.Name)
	assert.True(t, user.IsUser)
}

// T17: OrgMembership reports membership based on the API response.
func TestStage2OrgMembership(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	member, err := client.OrgMembership(context.Background(), &model.User{Login: "kylin", AccessToken: "at"}, "myorg")
	require.NoError(t, err)
	assert.True(t, member.Member)

	notMember, err := client.OrgMembership(context.Background(), &model.User{Login: "kylin", AccessToken: "at"}, "empty")
	require.NoError(t, err)
	assert.False(t, notMember.Member)
}

// T17: Dir fetches the files of a directory and their content.
func TestStage2Dir(t *testing.T) {
	srv, _ := mockStage2(t)
	client := stage2Client(t, srv)

	files, err := client.Dir(context.Background(), &model.User{AccessToken: "at"}, stage2Repo(), &model.Pipeline{Commit: "deadbeef"}, ".woodpecker")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, ".woodpecker/.woodpecker.yaml", files[0].Name)
	assert.Equal(t, "pipeline:\n  build:\n    image: golang", string(files[0].Data))
}
