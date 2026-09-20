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
	"fmt"
	"strings"

	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

// toRepo converts a Gitee repository to a Woodpecker repository.
func toRepo(from *Repository) *model.Repo {
	repo := &model.Repo{
		ForgeRemoteID: model.ForgeRemoteID(fmt.Sprint(from.ID)),
		Name:          from.Name,
		FullName:      from.FullName,
		ForgeURL:      from.HTMLURL,
		Clone:         from.CloneURL,
		CloneSSH:      from.SSHURL,
		Branch:        from.DefaultBranch,
		IsSCMPrivate:  from.Private,
		PREnabled:     true,
		Perm:          toPerm(from.Permissions),
	}
	if from.Owner != nil {
		repo.Owner = from.Owner.Login
		repo.Avatar = from.Owner.AvatarURL
	}
	// The owner is needed to address the repo in later API calls, so derive
	// it from the full name if Gitee did not report it.
	if repo.Owner == "" {
		repo.Owner, _, _ = strings.Cut(from.FullName, "/")
	}
	repo.ResetVisibility()

	return repo
}

// toPerm converts the Gitee permissions to Woodpecker permissions.
// A repo that can be seen can always be pulled.
func toPerm(from *Permissions) *model.Perm {
	if from == nil {
		return &model.Perm{Pull: true}
	}
	return &model.Perm{
		Pull:  true,
		Push:  from.Push || from.Admin,
		Admin: from.Admin,
	}
}
