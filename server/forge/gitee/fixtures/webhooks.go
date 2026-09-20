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

package fixtures

// PushHookPayload is a Gitee "Push Hook" webhook payload for a branch push.
const PushHookPayload = `{
	"ref": "refs/heads/main",
	"before": "before-sha",
	"after": "after-sha",
	"total_commits_count": 1,
	"commits": [
		{
			"id": "after-sha",
			"message": "first commit",
			"url": "https://gitee.com/kylin/woodpecker/commit/after-sha",
			"author": {
				"name": "Kylin",
				"email": "kylin@example.com",
				"username": "kylin",
				"avatar_url": "https://gitee.com/avatars/42.png"
			},
			"added": ["a.txt"],
			"removed": [],
			"modified": ["b.txt"]
		}
	],
	"head_commit": {
		"id": "after-sha",
		"message": "first commit",
		"url": "https://gitee.com/kylin/woodpecker/commit/after-sha",
		"author": {
			"name": "Kylin",
			"email": "kylin@example.com",
			"username": "kylin",
			"avatar_url": "https://gitee.com/avatars/42.png"
		},
		"added": ["a.txt"],
		"removed": [],
		"modified": ["b.txt"]
	},
	"pusher": {
		"id": 42,
		"login": "kylin",
		"name": "Kylin",
		"email": "kylin@example.com",
		"avatar_url": "https://gitee.com/avatars/42.png"
	},
	"repository": {
		"id": 123456,
		"name": "woodpecker",
		"full_name": "kylin/woodpecker",
		"html_url": "https://gitee.com/kylin/woodpecker",
		"clone_url": "https://gitee.com/kylin/woodpecker.git",
		"ssh_url": "git@gitee.com:kylin/woodpecker.git",
		"private": true,
		"default_branch": "main",
		"owner": {
			"id": 42,
			"login": "kylin",
			"name": "Kylin",
			"avatar_url": "https://gitee.com/avatars/42.png"
		}
	}
}`

// TagPushHookPayload is a Gitee "Tag Push Hook" webhook payload.
const TagPushHookPayload = `{
	"ref": "refs/tags/v1.0",
	"before": "0000000000000000000000000000000000000000",
	"after": "tag-sha",
	"total_commits_count": 0,
	"commits": [],
	"pusher": {
		"id": 42,
		"login": "kylin",
		"name": "Kylin",
		"email": "kylin@example.com",
		"avatar_url": "https://gitee.com/avatars/42.png"
	},
	"repository": {
		"id": 123456,
		"name": "woodpecker",
		"full_name": "kylin/woodpecker",
		"html_url": "https://gitee.com/kylin/woodpecker",
		"clone_url": "https://gitee.com/kylin/woodpecker.git",
		"ssh_url": "git@gitee.com:kylin/woodpecker.git",
		"private": true,
		"default_branch": "main",
		"owner": {
			"id": 42,
			"login": "kylin",
			"name": "Kylin",
			"avatar_url": "https://gitee.com/avatars/42.png"
		}
	}
}`

// PullRequestHookPayload is a Gitee "Pull Request Hook" webhook payload for an
// opened pull request.
const PullRequestHookPayload = `{
	"action": "open",
	"number": 1,
	"pull_request": {
		"id": 1,
		"number": 1,
		"title": "Add feature",
		"state": "open",
		"html_url": "https://gitee.com/kylin/woodpecker/pulls/1",
		"draft": false,
		"mergeable": true,
		"user": {
			"id": 42,
			"login": "kylin",
			"name": "Kylin",
			"email": "kylin@example.com",
			"avatar_url": "https://gitee.com/avatars/42.png"
		},
		"head": {
			"ref": "feature",
			"sha": "head-sha",
			"label": "kylin:feature",
			"repo": {
				"id": 123456,
				"name": "woodpecker",
				"full_name": "kylin/woodpecker",
				"owner": {
					"id": 42,
					"login": "kylin",
					"name": "Kylin",
					"avatar_url": "https://gitee.com/avatars/42.png"
				},
				"html_url": "https://gitee.com/kylin/woodpecker",
				"clone_url": "https://gitee.com/kylin/woodpecker.git"
			}
		},
		"base": {
			"ref": "main",
			"sha": "base-sha",
			"label": "kylin:main",
			"repo": {
				"id": 123456,
				"name": "woodpecker",
				"full_name": "kylin/woodpecker",
				"owner": {
					"id": 42,
					"login": "kylin",
					"name": "Kylin",
					"avatar_url": "https://gitee.com/avatars/42.png"
				},
				"html_url": "https://gitee.com/kylin/woodpecker",
				"clone_url": "https://gitee.com/kylin/woodpecker.git"
			}
		}
	},
	"repository": {
		"id": 123456,
		"name": "woodpecker",
		"full_name": "kylin/woodpecker",
		"html_url": "https://gitee.com/kylin/woodpecker",
		"clone_url": "https://gitee.com/kylin/woodpecker.git",
		"ssh_url": "git@gitee.com:kylin/woodpecker.git",
		"private": true,
		"default_branch": "main",
		"owner": {
			"id": 42,
			"login": "kylin",
			"name": "Kylin",
			"avatar_url": "https://gitee.com/avatars/42.png"
		}
	},
	"sender": {
		"id": 42,
		"login": "kylin",
		"name": "Kylin",
		"email": "kylin@example.com",
		"avatar_url": "https://gitee.com/avatars/42.png"
	}
}`
