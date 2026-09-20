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
	"encoding/base64"
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

func TestFileFetchesAtCommit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v5/repos/kylin/woodpecker/contents/.woodpecker.yaml", r.URL.Path)
		assert.Equal(t, "deadbeef", r.URL.Query().Get("ref"))
		_, _ = w.Write([]byte(fixtures.ContentPayload))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}
	pipeline := &model.Pipeline{Commit: "deadbeef"}

	data, err := (&Gitee{url: srv.URL}).File(t.Context(), &model.User{AccessToken: "token"}, repo, pipeline, ".woodpecker.yaml")

	require.NoError(t, err)
	assert.Equal(t, "pipeline:\n  build:\n    image: golang", string(data))
}

func TestFileBase64DecodesContent(t *testing.T) {
	t.Parallel()

	payload := `{"type":"file","encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte("hello")) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	data, err := (&Gitee{url: srv.URL}).File(t.Context(), &model.User{}, repo, &model.Pipeline{Commit: "sha"}, "file")

	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestFileEscapesNestedPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v5/repos/kylin/woodpecker/contents/.woodpecker/ci.yaml", r.URL.Path)
		_, _ = w.Write([]byte(fixtures.ContentPayload))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	_, err := (&Gitee{url: srv.URL}).File(t.Context(), &model.User{}, repo, &model.Pipeline{Commit: "sha"}, ".woodpecker/ci.yaml")

	require.NoError(t, err)
}

func TestFileNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	data, err := (&Gitee{url: srv.URL}).File(t.Context(), &model.User{}, repo, &model.Pipeline{Commit: "sha"}, "missing.yaml")

	require.Error(t, err)
	assert.Nil(t, data)
	assert.True(t, errors.Is(err, &forge_types.ErrConfigNotFound{Configs: []string{"missing.yaml"}}))
}

func TestFileRejectsDirectory(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixtures.DirContentPayload))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	data, err := (&Gitee{url: srv.URL}).File(t.Context(), &model.User{}, repo, &model.Pipeline{Commit: "sha"}, ".woodpecker")

	require.Error(t, err)
	assert.Nil(t, data)
	assert.True(t, errors.Is(err, &forge_types.ErrConfigNotFound{Configs: []string{".woodpecker"}}))
}

func TestBranchHead(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v5/repos/kylin/woodpecker/branches/main", r.URL.Path)
		_, _ = w.Write([]byte(fixtures.BranchPayload))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	commit, err := (&Gitee{url: srv.URL}).BranchHead(t.Context(), &model.User{}, repo, "main")

	require.NoError(t, err)
	assert.Equal(t, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b", commit.SHA)
	assert.Equal(t, "https://gitee.com/kylin/woodpecker/commit/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b", commit.ForgeURL)
}

func TestBranchHeadNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	commit, err := (&Gitee{url: srv.URL}).BranchHead(t.Context(), &model.User{}, repo, "missing")

	require.Error(t, err)
	assert.Nil(t, commit)
}

func TestBranchHeadWithoutCommit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"main"}`))
	}))
	t.Cleanup(srv.Close)

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}

	commit, err := (&Gitee{url: srv.URL}).BranchHead(t.Context(), &model.User{}, repo, "main")

	require.Error(t, err)
	assert.Nil(t, commit)
}

func TestFileRequiresUser(t *testing.T) {
	t.Parallel()

	repo := &model.Repo{Owner: "kylin", Name: "woodpecker", FullName: "kylin/woodpecker"}
	_, err := (&Gitee{url: "https://gitee.com"}).File(t.Context(), nil, repo, &model.Pipeline{Commit: "sha"}, "file")

	require.Error(t, err)
}
