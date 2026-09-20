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

// Package fixtures holds sample responses of the Gitee API.
package fixtures

// RepoPayload is a private repository as returned by GET /api/v5/repos/{owner}/{repo}.
const RepoPayload = `{
	"id": 123456,
	"name": "woodpecker",
	"path": "woodpecker",
	"full_name": "kylin/woodpecker",
	"human_name": "Woodpecker CI",
	"owner": {
		"id": 42,
		"login": "kylin",
		"name": "Kylin",
		"avatar_url": "https://gitee.com/avatars/42.png"
	},
	"html_url": "https://gitee.com/kylin/woodpecker",
	"clone_url": "https://gitee.com/kylin/woodpecker.git",
	"ssh_url": "git@gitee.com:kylin/woodpecker.git",
	"private": true,
	"public": false,
	"archived": false,
	"default_branch": "main",
	"permissions": {
		"admin": true,
		"push": true,
		"pull": true
	}
}`

// PublicRepoPayload is a public repository without permissions and owner details.
const PublicRepoPayload = `{
	"id": 654321,
	"name": "docs",
	"full_name": "kylin/docs",
	"html_url": "https://gitee.com/kylin/docs",
	"clone_url": "https://gitee.com/kylin/docs.git",
	"ssh_url": "git@gitee.com:kylin/docs.git",
	"private": false,
	"public": true,
	"archived": false,
	"default_branch": "master"
}`

// ContentPayload is the response of GET /api/v5/repos/{owner}/{repo}/contents/{path}.
const ContentPayload = `{
	"type": "file",
	"name": ".woodpecker.yaml",
	"encoding": "base64",
	"size": 36,
	"content": "cGlwZWxpbmU6CiAgYnVpbGQ6CiAgICBpbWFnZTogZ29sYW5n",
	"sha": "a1b2c3",
	"url": "https://gitee.com/kylin/woodpecker/contents/.woodpecker.yaml",
	"html_url": "https://gitee.com/kylin/woodpecker/blob/main/.woodpecker.yaml",
	"download_url": "https://gitee.com/kylin/woodpecker/raw/main/.woodpecker.yaml"
}`

// DirContentPayload is returned when the path points at a directory.
const DirContentPayload = `[
	{
		"type": "file",
		"name": ".woodpecker.yaml",
		"encoding": "base64",
		"size": 38,
		"content": "cGlwZWxpbmU6CiAgYnVpbGQ6CiAgICBpbWFnZTogZ29sYW5n"
	}
]`

// BranchPayload is the response of GET /api/v5/repos/{owner}/{repo}/branches/{branch}.
const BranchPayload = `{
	"name": "main",
	"commit": {
		"sha": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b",
		"url": "https://gitee.com/kylin/woodpecker/commit/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b",
		"html_url": "https://gitee.com/kylin/woodpecker/commit/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b"
	}
}`

// UserReposPayload is the first page of GET /api/v5/user/repos.
const UserReposPayload = `[
	{
		"id": 123456,
		"name": "woodpecker",
		"full_name": "kylin/woodpecker",
		"owner": {
			"id": 42,
			"login": "kylin",
			"avatar_url": "https://gitee.com/avatars/42.png"
		},
		"html_url": "https://gitee.com/kylin/woodpecker",
		"clone_url": "https://gitee.com/kylin/woodpecker.git",
		"ssh_url": "git@gitee.com:kylin/woodpecker.git",
		"private": true,
		"archived": false,
		"default_branch": "main",
		"permissions": {
			"admin": true,
			"push": true,
			"pull": true
		}
	},
	{
		"id": 654321,
		"name": "docs",
		"full_name": "kylin/docs",
		"owner": {
			"id": 42,
			"login": "kylin",
			"avatar_url": "https://gitee.com/avatars/42.png"
		},
		"html_url": "https://gitee.com/kylin/docs",
		"clone_url": "https://gitee.com/kylin/docs.git",
		"ssh_url": "git@gitee.com:kylin/docs.git",
		"private": false,
		"archived": false,
		"default_branch": "master",
		"permissions": {
			"admin": false,
			"push": true,
			"pull": true
		}
	},
	{
		"id": 999,
		"name": "old",
		"full_name": "kylin/old",
		"owner": {
			"id": 42,
			"login": "kylin"
		},
		"html_url": "https://gitee.com/kylin/old",
		"clone_url": "https://gitee.com/kylin/old.git",
		"private": false,
		"archived": true
	}
]`
