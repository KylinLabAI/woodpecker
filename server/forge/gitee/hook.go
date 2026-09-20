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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/woodpecker/v3/server/forge/common"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

const (
	headerGiteeEvent = "X-Gitee-Event"

	hookPush        = "Push Hook"
	hookTagPush     = "Tag Push Hook"
	hookPullRequest = "Pull Request Hook"
	hookNote        = "Note Hook"
	hookIssue       = "Issue Hook"
	hookRepoChange  = "Repository Hook"
)

// Hook parses an incoming Gitee webhook and returns the repository and pipeline
// data. The webhook is authenticated by Woodpecker's hook token (a JWT signed
// with the repository hash) at the HTTP layer in server/api/hook.go; this method
// only decodes the payload. Gitee verifies the same secret when calling the hook
// url, so no additional signature check is required here.
//
// Return semantics follow the Forge contract:
//   - (repo, pipeline, nil): execute a pipeline for this event
//   - (repo, nil, nil): valid webhook, no pipeline should run
//   - (nil, nil, types.ErrIgnoreEvent): event ignored (logged)
//   - (nil, nil, error): invalid webhook or parse error
func (c *Gitee) Hook(_ context.Context, r *http.Request) (*model.Repo, *model.Pipeline, error) {
	hookType := r.Header.Get(headerGiteeEvent)

	repo, pipeline, err := parseHook(r)
	if err != nil {
		return nil, nil, err
	}
	if pipeline != nil {
		switch pipeline.Event {
		case model.EventTag:
			if pipeline.TagTitle == "" {
				pipeline.TagTitle = strings.TrimPrefix(pipeline.Ref, "refs/tags/")
			}
			if pipeline.Commit == "" {
				return nil, nil, fmt.Errorf("could not determine commit for tag %s", pipeline.TagTitle)
			}
		case model.EventPull, model.EventPullClosed, model.EventPullMetadata:
			if pipeline.Commit == "" {
				return nil, nil, fmt.Errorf("could not determine commit for pull request %s", pipeline.Ref)
			}
		}
		return repo, pipeline, nil
	}

	// No pipeline: either a valid webhook that needs no action (repo set) or an
	// ignored event (repo nil). The server treats both as a successful no-op.
	if repo != nil {
		return repo, nil, nil
	}
	log.Debug().Msgf("unsupported or ignored gitee hook type: '%s'", hookType)
	return nil, nil, &types.ErrIgnoreEvent{Event: hookType}
}

// parseHook dispatches the webhook to the matching parser based on the
// X-Gitee-Event header.
func parseHook(r *http.Request) (*model.Repo, *model.Pipeline, error) {
	switch r.Header.Get(headerGiteeEvent) {
	case hookPush, hookTagPush:
		return parsePushHook(r.Body)
	case hookPullRequest:
		return parsePullRequestHook(r.Body)
	}
	return nil, nil, &types.ErrIgnoreEvent{Event: r.Header.Get(headerGiteeEvent)}
}

// pushHook is the payload of a Gitee push or tag-push webhook.
type pushHook struct {
	Ref        string        `json:"ref"`
	Before     string        `json:"before"`
	After      string        `json:"after"`
	Commits    []*pushCommit `json:"commits"`
	HeadCommit *pushCommit   `json:"head_commit"`
	TotalCount int           `json:"total_commits_count"`
	Pusher     *sender       `json:"pusher"`
	Sender     *sender       `json:"sender"`
	Repository *Repository   `json:"repository"`
	Password   string        `json:"password"`
}

// pushCommit is a single commit inside a pushHook.
type pushCommit struct {
	ID        string     `json:"id"`
	Message   string     `json:"message"`
	URL       string     `json:"url"`
	Timestamp string     `json:"timestamp"`
	Author    *committer `json:"author"`
	Added     []string   `json:"added"`
	Removed   []string   `json:"removed"`
	Modified  []string   `json:"modified"`
}

// committer is the author of a commit reported by Gitee.
type committer struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

// sender is the user that triggered an event.
type sender struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// pullRequestHook is the payload of a Gitee pull request webhook.
type pullRequestHook struct {
	Action      string       `json:"action"`
	Number      int          `json:"number"`
	PullRequest *pullRequest `json:"pull_request"`
	Repository  *Repository  `json:"repository"`
	Sender      *sender      `json:"sender"`
}

// pullRequest is the pull_request object inside a pullRequestHook.
type pullRequest struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	Body      string     `json:"body"`
	Draft     bool       `json:"draft"`
	Mergeable bool       `json:"mergeable"`
	User      *sender    `json:"user"`
	Assignees []*sender  `json:"assignees"`
	Labels    []*label   `json:"labels"`
	Milestone *milestone `json:"milestone"`
	Head      *prRef     `json:"head"`
	Base      *prRef     `json:"base"`
}

// prRef is the head or base of a pull request.
type prRef struct {
	Ref   string      `json:"ref"`
	Sha   string      `json:"sha"`
	Label string      `json:"label"`
	Repo  *Repository `json:"repo"`
}

// label is a pull request label.
type label struct {
	Name string `json:"name"`
}

// milestone is a pull request milestone.
type milestone struct {
	Title string `json:"title"`
}

// parsePushHook parses a push or tag push webhook and returns the repo and
// pipeline details. Unsupported commits return nil values.
func parsePushHook(payload io.Reader) (repo *model.Repo, pipeline *model.Pipeline, err error) {
	push := new(pushHook)
	if err := json.NewDecoder(payload).Decode(push); err != nil {
		return nil, nil, fmt.Errorf("could not decode push hook: %w", err)
	}
	if push.Repository == nil {
		return nil, nil, fmt.Errorf("push hook does not contain repository info")
	}

	// Gitee sends tag pushes with ref "refs/tags/..."; treat them as tag events.
	if strings.HasPrefix(push.Ref, "refs/tags/") {
		repo = toRepo(push.Repository)
		pipeline = pipelineFromTag(push)
		return repo, pipeline, nil
	}

	if !strings.HasPrefix(push.Ref, "refs/heads/") {
		// Ignore anything that is not a branch or tag push (e.g. notes).
		return nil, nil, nil
	}

	repo = toRepo(push.Repository)
	pipeline = pipelineFromPush(push)
	return repo, pipeline, nil
}

// pipelineFromPush extracts the Pipeline data from a Gitee branch push hook.
func pipelineFromPush(hook *pushHook) *model.Pipeline {
	avatar := ""
	author := ""
	email := ""
	if hook.Pusher != nil {
		author = hook.Pusher.Login
		email = hook.Pusher.Email
		avatar = hook.Pusher.AvatarURL
	}

	var message string
	link := ""
	if len(hook.Commits) > 0 {
		message = hook.Commits[0].Message
		link = hook.Commits[0].URL
	}

	authorC := commitAuthorOf(hook)
	if authorC != nil {
		if authorC.Username != "" {
			author = authorC.Username
		}
		if authorC.Email != "" {
			email = authorC.Email
		}
		if authorC.AvatarURL != "" {
			avatar = authorC.AvatarURL
		}
	}
	if len(hook.Commits) == 1 {
		link = hook.Commits[0].URL
	}

	return &model.Pipeline{
		Event:        model.EventPush,
		Commit:       hook.After,
		Ref:          hook.Ref,
		ForgeURL:     link,
		Branch:       strings.TrimPrefix(hook.Ref, "refs/heads/"),
		Message:      message,
		Avatar:       expandAvatar(hook.Repository.HTMLURL, avatar),
		Author:       author,
		Email:        email,
		Timestamp:    time.Now().UTC().Unix(),
		Sender:       senderLogin(hook.Pusher),
		ChangedFiles: changedFilesFromPush(hook),
	}
}

// pipelineFromTag extracts the Pipeline data from a Gitee tag push hook.
func pipelineFromTag(hook *pushHook) *model.Pipeline {
	ref := strings.TrimPrefix(hook.Ref, "refs/tags/")
	author := ""
	email := ""
	avatar := ""
	if hook.Pusher != nil {
		author = hook.Pusher.Login
		email = hook.Pusher.Email
		avatar = hook.Pusher.AvatarURL
	}
	authorC := commitAuthorOf(hook)
	if authorC != nil {
		if authorC.Username != "" {
			author = authorC.Username
		}
		if authorC.Email != "" {
			email = authorC.Email
		}
		if authorC.AvatarURL != "" {
			avatar = authorC.AvatarURL
		}
	}

	return &model.Pipeline{
		Event:     model.EventTag,
		TagTitle:  ref,
		Commit:    hook.After,
		Ref:       fmt.Sprintf("refs/tags/%s", ref),
		ForgeURL:  fmt.Sprintf("%s/src/tag/%s", hook.Repository.HTMLURL, ref),
		Avatar:    expandAvatar(hook.Repository.HTMLURL, avatar),
		Author:    author,
		Sender:    senderLogin(hook.Pusher),
		Email:     email,
		Timestamp: time.Now().UTC().Unix(),
	}
}

// parsePullRequestHook parses a pull request webhook and returns the repo and
// pipeline details.
func parsePullRequestHook(payload io.Reader) (repo *model.Repo, pipeline *model.Pipeline, err error) {
	pr := new(pullRequestHook)
	if err := json.NewDecoder(payload).Decode(pr); err != nil {
		return nil, nil, fmt.Errorf("could not decode pull request hook: %w", err)
	}
	if pr.Repository == nil {
		return nil, nil, fmt.Errorf("pull request hook does not contain repository info")
	}
	if pr.PullRequest == nil {
		return nil, nil, fmt.Errorf("pull request hook does not contain pull_request info")
	}

	event, ok := pullRequestEvent(pr.Action)
	if !ok {
		// Unsupported action (comment, review, etc.) - a valid webhook that
		// does not require a pipeline.
		return toRepo(pr.Repository), nil, nil
	}

	repo = toRepo(pr.Repository)
	pipeline = pipelineFromPullRequest(pr, event)
	return repo, pipeline, nil
}

// pullRequestEvent maps a Gitee pull request action to a Woodpecker event.
// The second return value is false if the action should be ignored.
func pullRequestEvent(action string) (model.WebhookEvent, bool) {
	switch action {
	case "open", "synchronize", "reopen":
		return model.EventPull, true
	case "close", "merge":
		return model.EventPullClosed, true
	case "update":
		// A force-push to the PR branch is equivalent to synchronize.
		return model.EventPull, true
	case "edit", "label", "assign", "milestone", "unassign", "untitled":
		return model.EventPullMetadata, true
	default:
		return "", false
	}
}

// pipelineFromPullRequest extracts the Pipeline data from a Gitee pull request hook.
func pipelineFromPullRequest(hook *pullRequestHook, event model.WebhookEvent) *model.Pipeline {
	pr := hook.PullRequest

	author := ""
	email := ""
	if pr.User != nil {
		author = pr.User.Login
		email = pr.User.Email
	}
	sender := senderLogin(hook.Sender)

	avatar := ""
	if hook.Sender != nil {
		avatar = hook.Sender.AvatarURL
	}

	fromFork := false
	if pr.Head != nil && pr.Base != nil && pr.Head.Repo != nil && pr.Base.Repo != nil {
		fromFork = pr.Head.Repo.ID != pr.Base.Repo.ID
	}

	branch := ""
	if pr.Base != nil {
		branch = pr.Base.Ref
	}
	commit := ""
	if pr.Head != nil {
		commit = pr.Head.Sha
	}
	headRef := ""
	if pr.Head != nil {
		headRef = pr.Head.Ref
	}
	baseRef := ""
	if pr.Base != nil {
		baseRef = pr.Base.Ref
	}

	pipeline := &model.Pipeline{
		Event:                event,
		Commit:               commit,
		ForgeURL:             pr.HTMLURL,
		Ref:                  fmt.Sprintf("refs/pull/%d/head", pr.Number),
		Branch:               branch,
		Message:              pr.Title,
		Author:               author,
		Avatar:               expandAvatar(hook.Repository.HTMLURL, avatar),
		Sender:               sender,
		Email:                email,
		Title:                pr.Title,
		Refspec:              fmt.Sprintf("%s:%s", headRef, baseRef),
		PullRequestLabels:    convertLabels(pr.Labels),
		PullRequestMilestone: convertMilestone(pr.Milestone),
		PullRequestDraft:     pr.Draft,
		FromFork:             fromFork,
	}

	if event == model.EventPullMetadata {
		pipeline.EventReason = []string{hook.Action}
		for i := range pipeline.EventReason {
			pipeline.EventReason[i] = common.NormalizeEventReason(pipeline.EventReason[i])
		}
	}

	return pipeline
}

// commitAuthorOf returns the head commit author of a push hook, preferring the
// head commit and falling back to the first commit.
func commitAuthorOf(hook *pushHook) *committer {
	if hook.HeadCommit != nil && hook.HeadCommit.Author != nil {
		return hook.HeadCommit.Author
	}
	for _, c := range hook.Commits {
		if c.Author != nil {
			return c.Author
		}
	}
	return nil
}

// senderLogin returns the login of a sender, or an empty string.
func senderLogin(s *sender) string {
	if s == nil {
		return ""
	}
	return s.Login
}

// changedFilesFromPush collects the changed files from all commits and the head
// commit of a push hook.
func changedFilesFromPush(hook *pushHook) []string {
	files := make([]string, 0, len(hook.Commits)*3)
	for _, c := range hook.Commits {
		files = append(files, c.Added...)
		files = append(files, c.Removed...)
		files = append(files, c.Modified...)
	}
	if hook.HeadCommit != nil {
		files = append(files, hook.HeadCommit.Added...)
		files = append(files, hook.HeadCommit.Removed...)
		files = append(files, hook.HeadCommit.Modified...)
	}
	return files
}

// convertLabels returns the names of the given labels.
func convertLabels(from []*label) []string {
	labels := make([]string, 0, len(from))
	for _, l := range from {
		labels = append(labels, l.Name)
	}
	return labels
}

// convertMilestone returns the title of the given milestone.
func convertMilestone(from *milestone) string {
	if from == nil {
		return ""
	}
	return from.Title
}

// expandAvatar converts a relative avatar url into an absolute one using the
// repository url as a base. Already absolute urls are returned unchanged.
func expandAvatar(repo, rawURL string) string {
	aURL, err := url.Parse(rawURL)
	if err != nil || rawURL == "" {
		return rawURL
	}
	if aURL.IsAbs() {
		return aURL.String()
	}
	bURL, err := url.Parse(repo)
	if err != nil {
		return rawURL
	}
	return bURL.ResolveReference(aURL).String()
}
