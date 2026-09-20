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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestGitee returns a Gitee forge pointing at a test server serving handler.
func newTestGitee(t *testing.T, handler http.HandlerFunc) *Gitee {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &Gitee{url: server.URL}
}

func TestGetSendsAccessTokenAsQueryParam(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, r *http.Request) {
		// Gitee authenticates via query param, never via a bearer token.
		assert.Equal(t, "secret-token", r.URL.Query().Get("access_token"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Equal(t, "/api/v5/user", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"woodpecker"}`))
	})

	var user User
	err := client.get(t.Context(), "secret-token", "/user", nil, &user)

	require.NoError(t, err)
	assert.Equal(t, "woodpecker", user.Login)
}

func TestGetOmitsEmptyAccessToken(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("access_token"))
		assert.Equal(t, "main", r.URL.Query().Get("ref"))
		_, _ = w.Write([]byte(`{}`))
	})

	err := client.get(t.Context(), "", "/repos/foo/bar", url.Values{"ref": []string{"main"}}, nil)

	require.NoError(t, err)
}

func TestGetKeepsExistingParams(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token", r.URL.Query().Get("access_token"))
		assert.Equal(t, "full_name", r.URL.Query().Get("sort"))
		_, _ = w.Write([]byte(`{}`))
	})

	err := client.get(t.Context(), "token", "/user/repos", url.Values{"sort": []string{"full_name"}}, nil)

	require.NoError(t, err)
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	err := client.get(t.Context(), "token", "/repos/foo/bar", nil, nil)

	require.Error(t, err)
	assert.True(t, isNotFound(err))
	assert.Contains(t, err.Error(), "Not Found")
}

func TestGetOtherErrorIsNotNotFound(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})

	err := client.get(t.Context(), "token", "/user", nil, nil)

	require.Error(t, err)
	assert.False(t, isNotFound(err))
	assert.Contains(t, err.Error(), strconv.Itoa(http.StatusInternalServerError))
}

func TestGetDoesNotLeakAccessTokenOnTransportError(t *testing.T) {
	t.Parallel()

	client := &Gitee{url: "http://127.0.0.1:1"}

	err := client.get(t.Context(), "super-secret-token", "/user", nil, nil)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "super-secret-token")
	assert.Contains(t, err.Error(), "/user")
}

func TestGetRedirectIsAnError(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently)
		_, _ = w.Write([]byte(`{"message":"moved"}`))
	})

	err := client.get(t.Context(), "token", "/user", nil, nil)

	require.Error(t, err)
	assert.False(t, isNotFound(err))
	assert.Contains(t, err.Error(), strconv.Itoa(http.StatusMovedPermanently))
}

func TestGetEmptyBodySkipsDecoding(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	var user User
	err := client.get(t.Context(), "token", "/user", nil, &user)

	require.NoError(t, err)
	assert.Empty(t, user.Login)
}

func TestGetDecodeError(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})

	var user User
	err := client.get(t.Context(), "token", "/user", nil, &user)

	require.Error(t, err)
	assert.False(t, isNotFound(err))
}

func TestFetchAllPagesStopsOnPartialPage(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	client := newTestGitee(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, strconv.Itoa(defaultPageSize), r.URL.Query().Get("per_page"))

		items := make([]Repository, defaultPageSize)
		if r.URL.Query().Get("page") == "2" {
			items = items[:1]
		}
		body, err := json.Marshal(items)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		requests.Add(1)
		_, _ = w.Write(body)
	})

	repos, err := fetchAllPages[Repository](t.Context(), client, "token", "/user/repos", nil)

	require.NoError(t, err)
	assert.Equal(t, defaultPageSize+1, len(repos))
	assert.Equal(t, int64(2), requests.Load(), "paging must stop after a partial page")
}

func TestFetchAllPagesStopsOnEmptyPage(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	repos, err := fetchAllPages[Repository](t.Context(), client, "token", "/user/repos", nil)

	require.NoError(t, err)
	assert.Empty(t, repos)
}

func TestFetchAllPagesPropagatesError(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	})

	repos, err := fetchAllPages[Repository](t.Context(), client, "bad", "/user/repos", nil)

	require.Error(t, err)
	assert.Nil(t, repos)
	assert.False(t, isNotFound(err))
}

func TestFetchAllPagesStopsAtMaxPages(t *testing.T) {
	t.Parallel()

	client := newTestGitee(t, func(w http.ResponseWriter, _ *http.Request) {
		// always return a full page, the guard has to kick in
		body, err := json.Marshal(make([]Repository, defaultPageSize))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})

	repos, err := fetchAllPages[Repository](t.Context(), client, "token", "/user/repos", nil)

	require.Error(t, err)
	assert.Nil(t, repos)
	assert.Contains(t, err.Error(), "paging")
}

func TestHTTPClientIsCached(t *testing.T) {
	t.Parallel()

	client := &Gitee{url: "https://gitee.com", skipVerify: true}

	// building a client per request would leak idle connections
	assert.Same(t, client.httpClient(), client.httpClient())
	assert.NotNil(t, client.httpClient().Transport)
}

func TestApiURLTrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://gitee.com/api/v5", (&Gitee{url: "https://gitee.com"}).apiURL())
	assert.Equal(t, "https://gitee.com/api/v5", (&Gitee{url: "https://gitee.com/"}).apiURL())
}

func TestErrorMessageOfInvalidBody(t *testing.T) {
	t.Parallel()

	assert.Empty(t, errorMessage([]byte("not json")))
	assert.Equal(t, "boom", errorMessage([]byte(`{"message":"boom"}`)))
}

func TestSanitizeErrorKeepsUnderlyingError(t *testing.T) {
	t.Parallel()

	wrapped := &url.Error{Op: "Get", URL: "https://gitee.com/api/v5/user?access_token=secret", Err: context.DeadlineExceeded}

	err := sanitizeError(wrapped, "/user")

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "access_token")
}
