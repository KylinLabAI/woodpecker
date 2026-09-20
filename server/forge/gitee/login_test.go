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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/server"
	forge_types "go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

// setOAuthHost points the global oauth host at the test server for the
// duration of the test. Tests using it must not run in parallel as
// server.Config is a package global.
func setOAuthHost(t *testing.T) {
	t.Helper()

	const oauthHost = "http://woodpecker.test"
	previous := server.Config.Server.OAuthHost
	server.Config.Server.OAuthHost = oauthHost
	t.Cleanup(func() {
		server.Config.Server.OAuthHost = previous
	})
}

func TestOAuth2Config(t *testing.T) {
	setOAuthHost(t)

	client := &Gitee{
		url:               "https://gitee.com",
		oAuthClientID:     "id",
		oAuthClientSecret: "secret",
	}

	config, _ := client.oauth2Config(t.Context())

	assert.Equal(t, "https://gitee.com/oauth/authorize", config.Endpoint.AuthURL)
	assert.Equal(t, "https://gitee.com/oauth/token", config.Endpoint.TokenURL)
	assert.Equal(t, "http://woodpecker.test/authorize", config.RedirectURL)
	assert.Equal(t, "id", config.ClientID)
	assert.Equal(t, "secret", config.ClientSecret)
}

func TestOAuth2ConfigUsesPublicOAuthHost(t *testing.T) {
	setOAuthHost(t)

	client := &Gitee{url: "http://internal.gitee", oAuthHost: "https://gitee.com"}

	config, _ := client.oauth2Config(t.Context())

	assert.Equal(t, "https://gitee.com/oauth/authorize", config.Endpoint.AuthURL,
		"the browser has to be sent to the publicly reachable host")
	assert.Equal(t, "http://internal.gitee/oauth/token", config.Endpoint.TokenURL,
		"the token is exchanged server side")
}

func TestLoginWithoutCodeOnlyReturnsRedirectURL(t *testing.T) {
	setOAuthHost(t)

	client := &Gitee{url: "https://gitee.com", oAuthClientID: "id"}

	user, redirectURL, err := client.Login(t.Context(), &forge_types.OAuthRequest{State: "state"})

	require.NoError(t, err)
	assert.Nil(t, user)
	assert.Contains(t, redirectURL, "https://gitee.com/oauth/authorize")
	assert.Contains(t, redirectURL, "state=state")
	assert.Contains(t, redirectURL, "redirect_uri=http%3A%2F%2Fwoodpecker.test%2Fauthorize")
}

func TestLoginExchangesCodeForUser(t *testing.T) {
	setOAuthHost(t)

	tokenCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			assert.NoError(t, r.ParseForm())
			// Gitee expects the credentials in the body
			assert.Equal(t, "client-id", r.Form.Get("client_id"))
			assert.Equal(t, "client-secret", r.Form.Get("client_secret"))
			assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
			assert.Equal(t, "the-code", r.Form.Get("code"))
			tokenCalls++
			_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","token_type":"bearer","expires_in":86400}`))
		case "/api/v5/user":
			assert.Equal(t, "at", r.URL.Query().Get("access_token"))
			_, _ = w.Write([]byte(`{"id":4242,"login":"woodpecker","name":"Woodpecker CI","email":"ci@example.com","avatar_url":"https://gitee.com/a.png"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client := &Gitee{
		url:               srv.URL,
		oAuthClientID:     "client-id",
		oAuthClientSecret: "client-secret",
	}

	user, redirectURL, err := client.Login(t.Context(), &forge_types.OAuthRequest{Code: "the-code"})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, 1, tokenCalls)
	assert.NotEmpty(t, redirectURL)

	assert.Equal(t, "woodpecker", user.Login)
	assert.Equal(t, "ci@example.com", user.Email)
	assert.Equal(t, "https://gitee.com/a.png", user.Avatar)
	assert.Equal(t, model.ForgeRemoteID("4242"), user.ForgeRemoteID)
	assert.Equal(t, "at", user.AccessToken)
	assert.Equal(t, "rt", user.RefreshToken)
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), user.Expiry, 60)
}

func TestLoginFailsOnTokenError(t *testing.T) {
	setOAuthHost(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	t.Cleanup(srv.Close)

	client := &Gitee{url: srv.URL, oAuthClientID: "id", oAuthClientSecret: "secret"}

	user, _, err := client.Login(t.Context(), &forge_types.OAuthRequest{Code: "bad"})

	require.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "exchange")
}

func TestLoginFailsOnUserInfoError(t *testing.T) {
	setOAuthHost(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"at","token_type":"bearer","expires_in":86400}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	client := &Gitee{url: srv.URL, oAuthClientID: "id", oAuthClientSecret: "secret"}

	user, _, err := client.Login(t.Context(), &forge_types.OAuthRequest{Code: "the-code"})

	require.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "user info")
}

func TestRefreshUpdatesBothTokens(t *testing.T) {
	setOAuthHost(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "old-refresh", r.Form.Get("refresh_token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","token_type":"bearer","expires_in":86400}`))
	}))
	t.Cleanup(srv.Close)

	client := &Gitee{url: srv.URL, oAuthClientID: "id", oAuthClientSecret: "secret"}
	user := &model.User{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(time.Hour).Unix(),
	}

	updated, err := client.Refresh(t.Context(), user)

	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, "new-at", user.AccessToken)
	// Gitee rotates the refresh token, it has to be persisted as well
	assert.Equal(t, "new-rt", user.RefreshToken)
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), user.Expiry, 60)
}

func TestRefreshWithoutRefreshToken(t *testing.T) {
	user := &model.User{AccessToken: "at"}

	updated, err := (&Gitee{url: "https://gitee.com"}).Refresh(t.Context(), user)

	require.NoError(t, err)
	assert.False(t, updated)
	assert.Equal(t, "at", user.AccessToken)
}

func TestRefreshReturnsFalseOnError(t *testing.T) {
	setOAuthHost(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(srv.Close)

	client := &Gitee{url: srv.URL, oAuthClientID: "id", oAuthClientSecret: "secret"}
	user := &model.User{AccessToken: "at", RefreshToken: "rt"}

	updated, err := client.Refresh(t.Context(), user)

	require.Error(t, err)
	assert.False(t, updated)
	assert.Equal(t, "at", user.AccessToken)
}
