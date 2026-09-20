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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/server/forge/gitee/fixtures"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

// newHookRequest builds an http.Request carrying the given Gitee webhook payload
// and event type header.
func newHookRequest(event, payload string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/hook", strings.NewReader(payload))
	req.Header.Set("X-Gitee-Event", event)
	return req
}

func TestHookPush(t *testing.T) {
	repo, pipeline, err := (&Gitee{}).Hook(context.Background(), newHookRequest("Push Hook", fixtures.PushHookPayload))
	require.NoError(t, err)
	require.NotNil(t, repo)
	require.NotNil(t, pipeline)

	assert.Equal(t, "kylin/woodpecker", repo.FullName)
	assert.Equal(t, model.ForgeRemoteID("123456"), repo.ForgeRemoteID)
	assert.Equal(t, model.EventPush, pipeline.Event)
	assert.Equal(t, "after-sha", pipeline.Commit)
	assert.Equal(t, "refs/heads/main", pipeline.Ref)
	assert.Equal(t, "main", pipeline.Branch)
	assert.Equal(t, "kylin", pipeline.Author)
	assert.Equal(t, "kylin@example.com", pipeline.Email)
	assert.Equal(t, "kylin", pipeline.Sender)
	assert.Equal(t, "https://gitee.com/kylin/woodpecker/commit/after-sha", pipeline.ForgeURL)
	assert.Contains(t, pipeline.ChangedFiles, "a.txt")
	assert.Contains(t, pipeline.ChangedFiles, "b.txt")
}

func TestHookTagPush(t *testing.T) {
	repo, pipeline, err := (&Gitee{}).Hook(context.Background(), newHookRequest("Tag Push Hook", fixtures.TagPushHookPayload))
	require.NoError(t, err)
	require.NotNil(t, repo)
	require.NotNil(t, pipeline)

	assert.Equal(t, model.EventTag, pipeline.Event)
	assert.Equal(t, "v1.0", pipeline.TagTitle)
	assert.Equal(t, "refs/tags/v1.0", pipeline.Ref)
	assert.Equal(t, "tag-sha", pipeline.Commit)
	assert.Equal(t, "kylin", pipeline.Author)
	assert.Equal(t, "kylin", pipeline.Sender)
}

func TestHookPullRequest(t *testing.T) {
	repo, pipeline, err := (&Gitee{}).Hook(context.Background(), newHookRequest("Pull Request Hook", fixtures.PullRequestHookPayload))
	require.NoError(t, err)
	require.NotNil(t, repo)
	require.NotNil(t, pipeline)

	assert.Equal(t, model.EventPull, pipeline.Event)
	assert.Equal(t, "head-sha", pipeline.Commit)
	assert.Equal(t, "refs/pull/1/head", pipeline.Ref)
	assert.Equal(t, "main", pipeline.Branch)
	assert.Equal(t, "Add feature", pipeline.Title)
	assert.Equal(t, "kylin", pipeline.Author)
	assert.Equal(t, "feature:main", pipeline.Refspec)
	assert.False(t, pipeline.FromFork)
	assert.Equal(t, "https://gitee.com/kylin/woodpecker/pulls/1", pipeline.ForgeURL)
}

func TestHookPullRequestClosed(t *testing.T) {
	payload := strings.Replace(fixtures.PullRequestHookPayload, `"action": "open"`, `"action": "close"`, 1)
	_, pipeline, err := (&Gitee{}).Hook(context.Background(), newHookRequest("Pull Request Hook", payload))
	require.NoError(t, err)
	require.NotNil(t, pipeline)
	assert.Equal(t, model.EventPullClosed, pipeline.Event)
}

func TestHookIgnoredEvent(t *testing.T) {
	repo, pipeline, err := (&Gitee{}).Hook(context.Background(), newHookRequest("Note Hook", "{}"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, &types.ErrIgnoreEvent{}))
	assert.Nil(t, repo)
	assert.Nil(t, pipeline)
}

func TestHookUnsupportedAction(t *testing.T) {
	// A pull request "comment" (note) action must not schedule a pipeline but is
	// a valid webhook, so it returns (repo, nil, nil).
	payload := strings.Replace(fixtures.PullRequestHookPayload, `"action": "open"`, `"action": "comment"`, 1)
	repo, pipeline, err := (&Gitee{}).Hook(context.Background(), newHookRequest("Pull Request Hook", payload))
	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Nil(t, pipeline)
}
