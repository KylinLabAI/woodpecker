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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	forge_types "go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

// These methods are not implemented yet (see the integration plan, Stage 2).
// The callers rely on the exact error to fall back gracefully, so it must stay
// types.ErrNotImplemented.
func TestNotImplementedStubs(t *testing.T) {
	t.Parallel()

	client := &Gitee{}
	user := &model.User{}
	repo := &model.Repo{}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "Teams", run: func() error { _, e := client.Teams(t.Context(), user, &model.ListOptions{}); return e }},
		{name: "Dir", run: func() error { _, e := client.Dir(t.Context(), user, repo, &model.Pipeline{}, "dir"); return e }},
		{name: "Branches", run: func() error { _, e := client.Branches(t.Context(), user, repo, &model.ListOptions{}); return e }},
		{name: "PullRequests", run: func() error { _, e := client.PullRequests(t.Context(), user, repo, &model.ListOptions{}); return e }},
		{name: "OrgMembership", run: func() error { _, e := client.OrgMembership(t.Context(), user, "kylin"); return e }},
		{name: "Org", run: func() error { _, e := client.Org(t.Context(), user, "kylin"); return e }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.run()
			require.Error(t, err)
			assert.True(t, errors.Is(err, forge_types.ErrNotImplemented),
				"%s should return ErrNotImplemented, got %v", tc.name, err)
		})
	}
}

func TestHookNotImplemented(t *testing.T) {
	t.Parallel()

	_, _, err := (&Gitee{}).Hook(t.Context(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}
