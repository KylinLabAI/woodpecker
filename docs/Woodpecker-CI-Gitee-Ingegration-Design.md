# Woodpecker CI Gitee OAuth & Forge Integration Design

> This document has been verified against the `woodpecker` mainline source (module `go.woodpecker-ci.org/woodpecker/v3`) and can be used directly as the implementation reference.
> For the execution steps, see `Woodpecker-CI-Gitee-Ingegration-Plan.md`.

## 1. Overview

### 1.1 Goal

Add an official **Gitee.com OAuth2 + Forge adapter** to Woodpecker CI:

- Log in to Woodpecker with a Gitee account
- Automatically sync the user's Gitee repository list
- Fetch pipeline configuration and create builds
- Trigger builds manually (UI / CLI)
- (Stage 2) Trigger CI automatically via Gitee WebHooks

**Core constraints: zero changes to the Agent; the Server requires Go code changes and a rebuild; the Web frontend needs small type/icon changes.**

### 1.2 Target Version

Woodpecker **v3.x** mainline (`go.mod` → `go.woodpecker-ci.org/woodpecker/v3`).

### 1.3 Architecture Prerequisites

- Server-Master / Agent-Worker architecture
- All Git platform integration, OAuth, repo sync and WebHook logic lives in `woodpecker-server`
- `woodpecker-agent` only executes pipelines and contains no Git platform logic — **no changes required**

---

## 2. Solution Options

### 2.1 Option A: Native Gitee Forge (recommended)

Add a `gitee` adapter to the v3 native Forge system, at the same level as GitHub / Gitea / Forgejo / GitLab / Bitbucket.

### 2.2 Option B: Addon Forge (zero-intrusion alternative)

v3 ships with a built-in **addon forge** mechanism that implements a forge through an external executable, requiring **no server code changes at all**:

```
WOODPECKER_ADDON_FORGE=/usr/local/bin/woodpecker-gitee-forge
```

See `server/forge/addon/` (`server.go` / `client.go` / `plugin.go` / `args.go`); registration entry is `setupAddon()` in `server/forge/setup/setup.go`, using the type constant `model.ForgeTypeAddon`.

Trade-off: addons communicate over go-plugin RPC, which is harder to debug and package, and the full Forge interface must still be implemented. **Choose A if you want to upstream it; choose B if you want to keep the mainline untouched.**

### 2.3 Capability Tiers

**Stage 1 (required)**

- Gitee OAuth2 login + user binding
- **Access Token auto-refresh** (Gitee tokens only live 1 day — see 3.4, cannot be deferred)
- Repository list sync and single-repo lookup
- **Fetch pipeline configuration via `File()`** (without it no build can be created)
- **`Netrc()` clone credentials**
- **`Activate()` / `Deactivate()`** (an error here makes repo activation return HTTP 500)
- **`BranchHead()`** (used by cron and manual builds to resolve the branch head)
- Manual build triggering (fully working in UI / CLI)

**Stage 2 (optional)**

- Gitee WebHook creation / deletion / signature verification / event parsing (`Hook()`)
- Build status reporting back to Gitee (`Status()`)
- Pull request event triggering (`PullRequests()`)

---

## 3. Gitee Official OAuth2 Specification

### 3.1 Official Endpoints

| Purpose | URL | Method |
|---|---|---|
| Authorization page | `https://gitee.com/oauth/authorize` | GET |
| Exchange token | `https://gitee.com/oauth/token` | POST |
| Refresh token | `https://gitee.com/oauth/token` (`grant_type=refresh_token`) | POST |
| Get user info | `https://gitee.com/api/v5/user` | GET |
| List user repos | `https://gitee.com/api/v5/user/repos` | GET |

Additional APIs needed later:

| Purpose | URL |
|---|---|
| Single repo | `GET /api/v5/repos/{owner}/{repo}` |
| File content | `GET /api/v5/repos/{owner}/{repo}/contents/{path}?ref={sha}` (returns base64 `content`) |
| Branch list | `GET /api/v5/repos/{owner}/{repo}/branches` |
| Single branch | `GET /api/v5/repos/{owner}/{repo}/branches/{branch}` |
| PR list | `GET /api/v5/repos/{owner}/{repo}/pulls?state=open` |
| Commit status | `POST /api/v5/repos/{owner}/{repo}/statuses/{sha}` |
| WebHook | `POST /api/v5/repos/{owner}/{repo}/hooks` |

### 3.2 OAuth Flow (browser-side, no public IP required)

1. The user's browser opens the internal Woodpecker instance
2. It redirects to the public Gitee authorization page
3. After the user authorizes, Gitee 302-redirects **the user's browser** back to the internal callback address
4. The browser requests the internal Woodpecker `/authorize` endpoint to finish login

✅ OAuth login works on a purely internal network
⚠️ Only WebHook auto-triggering needs a public IP / tunnel (Stage 2)

### 3.3 OAuth Scopes

- `user_info`: read user profile and log in
- `projects`: read the user's repositories

Stage 2 additionally requires: `hook` (manage WebHooks), `pull_requests` (PR events)

### 3.4 Token Lifetime and Refresh (critical)

Gitee OAuth `access_token` is valid for **1 day**; `refresh_token` is valid for **30 days**.

Woodpecker v3 provides the optional `forge.Refresher` interface (`server/forge/refresh.go`): when a token has **less than 30 minutes** of remaining lifetime, `Refresh()` is called automatically and the result is written back to the database.

**The Gitee adapter must implement `Refresh()` in Stage 1**, otherwise every repo sync / build fails for the user the next day and they have to log in again.

Refresh request:

```
POST https://gitee.com/oauth/token
grant_type=refresh_token&refresh_token={refresh_token}
```

The response returns **new** values for both `access_token` and `refresh_token`; both must be persisted.

### 3.5 API Authentication (easy to get wrong)

- Gitee authenticates via the `?access_token=xxx` query parameter or the `Authorization: token <token>` header
- It is **not** the `Bearer` scheme used by the Gitea SDK — do not copy `newClientToken()` from `server/forge/gitea/gitea.go`
- Pagination uses `page` / `per_page`; the response has **no Link header**, so terminate when "returned items < per_page"

---

## 4. Code Architecture Changes

### 4.1 New Directory

Follow the existing layout of `server/forge/gitea/` (it is not a single file):

```
server/forge/gitee/
  ├── gitee.go        # Forge implementation (Opts / New / Name / URL / Login / Refresh / Repo / Repos / File / Netrc / Activate ...)
  ├── types.go        # Gitee API response structs
  ├── helper.go       # HTTP client, authentication, pagination
  ├── parse.go        # Gitee response -> model.* conversion
  ├── parse_test.go   # Required by CI
  └── fixtures/       # Sample API response JSON
```

### 4.2 Files to Modify (verified, paths are accurate)

| File | Change |
|---|---|
| `server/model/forge.go` | Add `ForgeTypeGitee ForgeType = "gitee"` |
| `server/services/setup.go` | Add a `case c.Bool("gitee")` branch to `setupForgeService()`, default URL `https://gitee.com` |
| `server/forge/setup/setup.go` | Add `case model.ForgeTypeGitee: return setupGitee(forge)` to the `Forge()` switch, plus a new `setupGitee()` |
| `cmd/server/flags.go` | Add a `gitee` BoolFlag; hook `WOODPECKER_GITEE_URL` / `_CLIENT` / `_SECRET` / `_SKIP_VERIFY` into the generic forge flag `Sources` |
| `web/src/lib/api/types/forge.ts` | Add `'gitee'` to the `ForgeType` union |
| `web/src/components/atomic/Icon.vue` | Add the Gitee icon and the `IconName` union entry |
| `docs/docs/30-administration/10-configuration/12-forges/` | Add `xx-gitee.md` |

> ❌ Note: `server/config/config.go` does **not** exist. All env vars / CLI flags are defined in `cmd/server/flags.go`;
> `server/forge/forge.go` contains **only the interface definition** — no factory or environment dispatch logic.

### 4.3 Reference Anchors

```go
// server/services/setup.go — where to add the new branch
case c.Bool("gitea"):
    _forge.Type = model.ForgeTypeGitea
    if _forge.URL == "" {
        _forge.URL = "https://try.gitea.com"
    }
// add:
case c.Bool("gitee"):
    _forge.Type = model.ForgeTypeGitee
    if _forge.URL == "" {
        _forge.URL = "https://gitee.com"
    }
```

```go
// server/forge/setup/setup.go — new setupGitee, same signature as setupGitea
func setupGitee(forge *model.Forge) (forge.Forge, error) {
    opts := gitee.Opts{
        URL:               strings.TrimRight(forge.URL, "/"),
        OAuthClientID:     forge.OAuthClientID,
        OAuthClientSecret: forge.OAuthClientSecret,
        SkipVerify:        forge.SkipVerify,
        OAuthHost:         forge.OAuthHost,
    }
    if len(opts.URL) == 0 {
        return nil, fmt.Errorf("WOODPECKER_GITEE_URL must be set")
    }
    return gitee.New(forge.ID, opts)
}
```

```go
// cmd/server/flags.go — append after the Gitea section
//
// Gitee
//
&cli.BoolFlag{
    Sources: cli.EnvVars("WOODPECKER_GITEE"),
    Name:    "gitee",
    Usage:   "gitee driver is enabled",
},
```

Also add `WOODPECKER_GITEE_URL` to the `Sources` of `forge-url`,
add `WOODPECKER_GITEE_CLIENT_FILE` / `WOODPECKER_GITEE_CLIENT` to the `forge-oauth-client` chain,
add `WOODPECKER_GITEE_SECRET_FILE` / `WOODPECKER_GITEE_SECRET` to the `forge-oauth-secret` chain,
and add `WOODPECKER_GITEE_SKIP_VERIFY` to the `Sources` of `forge-skip-verify`.

---

## 5. Forge Interface Implementation Checklist

The `Forge` interface in `server/forge/forge.go` has **18 methods**, and Go interfaces are enforced at compile time — **all of them must be implemented**.
Methods that are not functionally supported yet must return `types.ErrNotImplemented` or `nil` (see the per-method requirements below).

| # | Method | Stage | Requirement |
|---|---|---|---|
| 1 | `Name() string` | 1 | Return `"gitee"` |
| 2 | `URL() string` | 1 | Return `https://gitee.com` |
| 3 | `Login(ctx, *types.OAuthRequest) (*model.User, string, error)` | 1 | **Two-phase**, see 5.1 |
| 4 | `Teams(ctx, u, p) ([]*model.Team, error)` | 2 | May return `types.ErrNotImplemented` |
| 5 | `Repo(ctx, u, remoteID, owner, name) (*model.Repo, error)` | 1 | Prefer lookup by `remoteID`, fall back to owner/name |
| 6 | `Repos(ctx, u, p) ([]*model.Repo, error)` | 1 | Must fill `Perm`, support pagination |
| 7 | `File(ctx, u, r, b, fileName) ([]byte, error)` | 1 | **Required**, otherwise no pipeline config at all |
| 8 | `Dir(ctx, u, r, b, dirName) ([]*types.FileMeta, error)` | 2 | May return `types.ErrNotImplemented` |
| 9 | `Status(ctx, u, r, b, wf) error` | 2 | Failures are only logged, must not block |
| 10 | `Netrc(u, r) (*model.Netrc, error)` | 1 | **Required**, clone credentials for private repos |
| 11 | `Activate(ctx, u, r, link) error` | 1 | **Must not return ErrNotImplemented**, see 5.2 |
| 12 | `Deactivate(ctx, u, r, link) error` | 1 | Same as above; ignore a missing webhook instead of erroring |
| 13 | `Branches(ctx, u, r, p) ([]string, error)` | 2 | May return `types.ErrNotImplemented` |
| 14 | `BranchHead(ctx, u, r, branch) (*model.Commit, error)` | 1 | **Required** for cron and resolving build branch heads |
| 15 | `PullRequests(ctx, u, r, p) ([]*model.PullRequest, error)` | 2 | May return `types.ErrNotImplemented` |
| 16 | `Hook(ctx, r) (*model.Repo, *model.Pipeline, error)` | 2 | Return an error placeholder in Stage 1 |
| 17 | `OrgMembership(ctx, u, org) (*model.OrgPerm, error)` | 2 | May return `types.ErrNotImplemented` |
| 18 | `Org(ctx, u, org) (*model.Org, error)` | 2 | May return `types.ErrNotImplemented` |

Additional optional interface (**must be implemented in Stage 1**):

```go
// server/forge/refresh.go
type Refresher interface {
    Refresh(ctx context.Context, u *model.User) (bool, error)
}
```

### 5.1 Two-Phase `Login()` Semantics

```go
func (c *Gitee) Login(ctx context.Context, req *forge_types.OAuthRequest) (*model.User, string, error) {
    config, oauth2Ctx := c.oauth2Config(ctx)
    redirectURL := config.AuthCodeURL(req.State)

    // Phase 1: empty code -> return only the redirect URL
    if len(req.Code) == 0 {
        return nil, redirectURL, nil
    }

    // Phase 2: with code -> exchange token + fetch user info
    token, err := config.Exchange(oauth2Ctx, req.Code)
    if err != nil {
        return nil, redirectURL, fmt.Errorf("oauth2 config exchange failed: %w", err)
    }
    account, err := c.getUserInfo(ctx, token.AccessToken)
    if err != nil {
        return nil, redirectURL, fmt.Errorf("fetching user info failed: %w", err)
    }

    return &model.User{
        AccessToken:   token.AccessToken,
        RefreshToken:  token.RefreshToken,
        Expiry:        token.Expiry.UTC().Unix(),
        Login:         account.Login,
        Email:         account.Email,
        ForgeRemoteID: model.ForgeRemoteID(fmt.Sprint(account.ID)),
        Avatar:        account.AvatarURL,
    }, redirectURL, nil
}
```

`oauth2Config` should mirror `server/forge/gitea/gitea.go`, but with the endpoint constants replaced:

```go
const (
    authorizeTokenURL = "%s/oauth/authorize"
    accessTokenURL    = "%s/oauth/token"
)
```

`RedirectURL` must be `fmt.Sprintf("%s/authorize", server.Config.Server.OAuthHost)`.

### 5.2 `Activate()` / `Deactivate()` Must Not Return ErrNotImplemented

Only the callers of `Teams` / `Dir` / `Branches` / `PullRequests` / `OrgMembership` handle `ErrNotImplemented` gracefully.
The caller of `Activate` returns HTTP 500 directly:

```go
// server/api/repo.go
err = _forge.Activate(c, user, repo, hookURL)
if err != nil {
    msg := "could not create webhook in forge."
    log.Error().Err(err).Msg(msg)
    c.String(http.StatusInternalServerError, msg)
    return
}
```

**Stage 1 approach**: implement `Activate()` / `Deactivate()` as no-op placeholders that return `nil` (the cost is no WebHook auto-triggering, only manual builds — which matches the Stage 1 goal). They must never return an error.

---

## 6. Environment Variable Design

```bash
# Enable the Gitee adapter
WOODPECKER_GITEE=true

# Gitee OAuth application credentials
# (the generic names WOODPECKER_FORGE_CLIENT / WOODPECKER_FORGE_SECRET work equally well)
WOODPECKER_GITEE_CLIENT=your-gitee-client-id
WOODPECKER_GITEE_SECRET=your-gitee-client-secret

# Gitee base URL, defaults to https://gitee.com — usually not needed
WOODPECKER_GITEE_URL=https://gitee.com

# Public-facing address, used to build the callback $WOODPECKER_HOST/authorize
WOODPECKER_HOST=http://192.168.x.x:8000

# Agent communication port (unchanged)
WOODPECKER_GRPC_ADDR=:9000

# Admin Gitee username
WOODPECKER_ADMIN=your-gitee-username
```

Notes:

- v3 introduced the generic prefix `WOODPECKER_FORGE_URL` / `WOODPECKER_FORGE_CLIENT` / `WOODPECKER_FORGE_SECRET` / `WOODPECKER_FORGE_SKIP_VERIFY`; the per-forge names are just aliases and are equivalent
- `WOODPECKER_HOST` → `cmd/server/setup.go` sets `server.Config.Server.OAuthHost = serverHost`, and the callback is built as `%s/authorize`
- `WOODPECKER_GRPC_ADDR`, `WOODPECKER_ADMIN` and `WOODPECKER_BACKEND` already exist — nothing new is needed

---

## 7. Gitee OAuth Application Registration

### 7.1 Registration URL

https://gitee.com/oauth/applications

### 7.2 Required Values

- Application name: Woodpecker CI
- Homepage URL: `$WOODPECKER_HOST`
- **Callback URL (strictly matched)**: `$WOODPECKER_HOST/authorize`
- Permissions: `user_info`, `projects` (Stage 2 adds `hook`, `pull_requests`)

❗ The callback URL must exactly match the redirect URL built by the code — no extra slash, no port change, no protocol change.

---

## 8. Field Mapping (Gitee → Woodpecker)

### 8.1 User Mapping

| Gitee API field | Woodpecker model field | Note |
|---|---|---|
| `login` | `User.Login` | Must match `^[a-zA-Z0-9-_.]+$`, length ≤ 250 |
| `id` | `User.ForgeRemoteID` | Cast to `model.ForgeRemoteID(string)` |
| `email` | `User.Email` | Gitee may return empty — add a fallback |
| `avatar_url` | `User.Avatar` | |
| `access_token` | `User.AccessToken` | |
| `refresh_token` | `User.RefreshToken` | |
| `expires_in` | `User.Expiry` | `token.Expiry.UTC().Unix()` |
| `name` | **no matching field** | `model.User` has no `Name` — do not map it |

The `User` returned by `Login()` must contain: `Login`, `Email`, `Avatar`, `AccessToken`, `RefreshToken`, `Expiry`, `ForgeRemoteID`.

### 8.2 Repository Mapping

| Gitee API field | Woodpecker model field | Note |
|---|---|---|
| `id` | `Repo.ForgeRemoteID` | Required so repos can still be located after a rename |
| `full_name` | `Repo.FullName` | |
| `name` | `Repo.Name` | |
| `owner.login` | `Repo.Owner` | |
| `html_url` | `Repo.ForgeURL` | **Not** `Repo.Link` (does not exist) |
| `clone_url` | `Repo.Clone` | HTTPS clone URL |
| `ssh_url` | `Repo.CloneSSH` | |
| `private` | `Repo.IsSCMPrivate` | **Not** `Repo.IsPrivate` (does not exist) |
| `private` | `Repo.Visibility` | `model.VisibilityPrivate` / `VisibilityPublic`; you can also just call `repo.ResetVisibility()` |
| `default_branch` | `Repo.Branch` | |
| - | `Repo.Perm` | `&model.Perm{Pull: true, Push: <can write>, Admin: <is admin>}` |

`model.Netrc` (returned by `Netrc()`):

| Field | Value |
|---|---|
| `Machine` | `gitee.com` (or `common.ExtractHostFromCloneURL(repo.Clone)`) |
| `Login` | user login |
| `Password` | `user.AccessToken` |
| `Type` | `model.ForgeTypeGitee` |

`model.Commit` (returned by `BranchHead()`): `SHA`, `ForgeURL`.

You can reuse `UserToken()`, `RepoUser()` and `ExtractHostFromCloneURL()` from `server/forge/common/`.

---

## 9. Build and Deployment

### 9.1 Build

Only the server needs to be rebuilt; the agent can stay on the official binary:

```bash
go build -o woodpecker-server ./cmd/server
```

If the frontend was changed (`forge.ts` / `Icon.vue`), the web assets must be rebuilt too:

```bash
cd web && pnpm install && pnpm build
```

### 9.2 Deployment Topology

- Linux 4G: custom-built **Gitee-enabled woodpecker-server** (Master; scheduling, packaging, publishing only)
- Windows 16G: official woodpecker-agent (build/compile)
- Mac 16G: official woodpecker-agent (build/compile)

All agents use `WOODPECKER_BACKEND=local` (supported by `pipeline/backend/local` + `cmd/agent/core/flags.go`), no Docker, native compilation.

---

## 10. Functional Acceptance Tests

1. Start the server and open `/login`; the login button shows the forge URL hostname (`gitee.com`) plus the favicon
   > Note: the login page renders generically (`web/src/views/Login.vue` takes the hostname from `forge.url`), so there is **no fixed "Login with Gitee" label**
2. Clicking it redirects to `https://gitee.com/oauth/authorize?...&redirect_uri=http://192.168.x.x:8000/authorize`
3. After authorizing, it redirects back to `/authorize` and lands on the Woodpecker home page
4. Click "sync repositories" and all public / private Gitee repos are fetched
5. Activating a repository succeeds (the Stage 1 `Activate()` placeholder must not error)
6. Add `.woodpecker.yaml`, trigger a build manually, and it is dispatched to the Windows / Mac / Linux agents
7. The build completes, logs are correct, artifacts can be packaged
8. **A day later, without logging in again, repo sync / builds still succeed** (verifies `Refresh()`)

---

## 11. Known Limitations & Follow-up TODOs

### 11.1 Stage 1 Limitations

- No WebHook auto-triggering; manual builds only
- No build status reported back to Gitee
- No PR list / PR events
- No organization (Org) or team (Team) permission model

### 11.2 Stage 2 Tasks

| Task | Methods involved |
|---|---|
| WebHook create / delete / event parsing / signature verification | `Activate`, `Deactivate`, `Hook` |
| Build status reporting | `Status` |
| PR list and PR events | `PullRequests`, `Hook` |
| Org and team permissions | `Org`, `OrgMembership`, `Teams` |
| Directory-level pipeline configuration | `Dir` |
