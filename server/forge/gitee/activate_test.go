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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

func TestActivateNeverErrors(t *testing.T) {
	t.Parallel()

	repo := &model.Repo{FullName: "kylin/woodpecker"}
	user := &model.User{Login: "kylin"}

	err := (&Gitee{}).Activate(t.Context(), user, repo, "https://woodpecker.test/hook")
	// A non-nil error would make repo activation fail with HTTP 500.
	require.NoError(t, err)
}

func TestDeactivateNeverErrors(t *testing.T) {
	t.Parallel()

	repo := &model.Repo{FullName: "kylin/woodpecker"}

	err := (&Gitee{}).Deactivate(t.Context(), nil, repo, "https://woodpecker.test/hook")
	require.NoError(t, err)
}

func TestStatusNeverErrors(t *testing.T) {
	t.Parallel()

	repo := &model.Repo{FullName: "kylin/woodpecker"}
	user := &model.User{Login: "kylin"}

	// Status failures are logged but must not block the pipeline.
	err := (&Gitee{}).Status(t.Context(), user, repo, &model.Pipeline{}, &model.Workflow{})
	assert.NoError(t, err)
}
