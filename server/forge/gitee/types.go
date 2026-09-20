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

// User is the user as returned by the Gitee API.
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// Owner is the owner of a repository as returned by the Gitee API.
// Gitee reports a user or an organization here.
type Owner struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// Permissions are the effective permissions of the requesting user on a repository.
type Permissions struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

// Repository is a repository as returned by the Gitee API.
type Repository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	FullName      string `json:"full_name"`
	HumanName     string `json:"human_name"`
	Owner         *Owner `json:"owner"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	Private       bool   `json:"private"`
	Public        bool   `json:"public"`
	Archived      bool   `json:"archived"`
	DefaultBranch string `json:"default_branch"`
	// The Gitee API returns the permission object under the singular key
	// "permission" (both for /user/repos and /repos/{owner}/{repo}). Using the
	// plural key never matched and silently dropped the permissions, which made
	// every repo look inaccessible (empty repo list).
	Permissions *Permissions `json:"permission"`
}

// CommitRef is the commit a branch or tag points to.
type CommitRef struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}

// Branch is a branch of a repository as returned by the Gitee API.
type Branch struct {
	Name   string     `json:"name"`
	Commit *CommitRef `json:"commit"`
}

// Content is a file of a repository as returned by the Gitee API.
// The content is base64 encoded when the encoding is "base64".
type Content struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// FileEntry is an entry of a directory as returned by the Gitee API contents
// endpoint for a directory. Only entries of type "file" are fetched as pipeline
// configuration by Dir().
type FileEntry struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Hook is a webhook of a repository as returned by the Gitee API.
type Hook struct {
	ID       int64    `json:"id"`
	URL      string   `json:"url"`
	Password string   `json:"password"`
	Events   []string `json:"events"`
}

// apiPullRequest is a pull request as returned by the Gitee API.
type apiPullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// apiOrg is an organization or user as returned by the Gitee API.
type apiOrg struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
	Path  string `json:"path"`
}

// apiOrgMember is a membership of a user in an organization.
type apiOrgMember struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	State string `json:"state"`
}
