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

func TestNetrcForPrivateRepo(t *testing.T) {
	t.Parallel()

	repo := &model.Repo{
		Clone: "https://gitee.com/kylin/woodpecker.git",
	}
	user := &model.User{Login: "kylin", AccessToken: "secret-token"}

	netrc, err := (&Gitee{}).Netrc(user, repo)

	require.NoError(t, err)
	assert.Equal(t, "gitee.com", netrc.Machine)
	assert.Equal(t, "kylin", netrc.Login)
	assert.Equal(t, "secret-token", netrc.Password)
	assert.Equal(t, model.ForgeTypeGitee, netrc.Type)
}

func TestNetrcForPublicRepoWithoutUser(t *testing.T) {
	t.Parallel()

	repo := &model.Repo{Clone: "https://gitee.com/kylin/docs.git"}

	netrc, err := (&Gitee{}).Netrc(nil, repo)

	require.NoError(t, err)
	assert.Equal(t, "gitee.com", netrc.Machine)
	assert.Empty(t, netrc.Login)
	assert.Empty(t, netrc.Password)
}

func TestNetrcRequiresRepo(t *testing.T) {
	t.Parallel()

	_, err := (&Gitee{}).Netrc(&model.User{}, nil)

	require.Error(t, err)
}
