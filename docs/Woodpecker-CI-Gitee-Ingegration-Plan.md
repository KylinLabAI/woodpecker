# Woodpecker CI Gitee Integration — Implementation Plan

> Companion document: `Woodpecker-CI-Gitee-Ingegration-Design.md` (design: interface checklist, field mapping, endpoints, pitfalls)
> This is the **execution document**: follow T0 → T12 in order to complete Stage 1; T13+ is Stage 2.
> All paths and method signatures have been verified against the `go.woodpecker-ci.org/woodpecker/v3` mainline source.

## 0. How to Use This Plan

- Every task provides: **Goal / Files / Steps / Acceptance Criteria / Verification Command**
- Tasks are ordered by dependency — **follow the order** (later tasks depend on earlier ones compiling)
- Run `go build ./...` after every task to avoid a pile of compile errors at the end
- Stage 1 is done when all 8 acceptance cases in Chapter 10 of the design document pass

---

## 1. Task Overview

| ID | Task | Stage | Depends on | Estimate |
|---|---|---|---|---|
| T0 | Register a Gitee OAuth application | Prep | - | 0.5h |
| T1 | Skeleton & registration: make `WOODPECKER_GITEE=true` recognized | 1 | T0 | 2h |
| T2 | `types.go` + `helper.go`: API client, auth, pagination | 1 | T1 | 3h |
| T3 | `Login()` + `oauth2Config()` + `Refresh()` | 1 | T2 | 4h |
| T4 | `Repo()` / `Repos()` + `parse.go` repo mapping + tests | 1 | T2 | 4h |
| T5 | `File()` + `BranchHead()` | 1 | T2 | 3h |
| T6 | `Netrc()` | 1 | T2 | 1h |
| T7 | `Activate()` / `Deactivate()` placeholders | 1 | T1 | 0.5h |
| T8 | Placeholders for the remaining 8 methods | 1 | T1 | 1h |
| T9 | Web frontend: `forge.ts` + `Icon.vue` | 1 | T1 | 1h |
| T10 | Admin docs `docs/docs/.../xx-gitee.md` | 1 | T1 | 1h |
| T11 | Quality gates: license / lint / test / generate | 1 | T1-T10 | 2h |
| T12 | End-to-end acceptance (8 cases) | 1 | T11 | 2h |
| T13 | `Hook()` WebHook event parsing | 2 | T12 | 6h |
| T14 | Real WebHook create/delete in `Activate()` / `Deactivate()` | 2 | T13 | 3h |
| T15 | `Status()` build status reporting | 2 | T12 | 2h |
| T16 | `PullRequests()` + PR events | 2 | T12 | 3h |
| T17 | `Org()` / `OrgMembership()` / `Teams()` / `Branches()` / `Dir()` | 2 | T12 | 5h |

---

## T0 — Register a Gitee OAuth Application

**Goal**: obtain `client_id` / `client_secret` and fix the callback URL.

**Steps**

1. Open https://gitee.com/oauth/applications and create an application
2. Application name: `Woodpecker CI`
3. Homepage URL: `$WOODPECKER_HOST` (e.g. `http://192.168.1.10:8000`)
4. Callback URL: `$WOODPECKER_HOST/authorize` (**strict match — no extra slash, no port change, no protocol change**)
5. Select permissions: `user_info`, `projects` (Stage 2 adds `hook`, `pull_requests`)
6. Record the `Client ID` and `Client Secret`

**Acceptance Criteria**: Woodpecker starts locally and `/login` is reachable at `$WOODPECKER_HOST`.

---

## T1 — Skeleton & Registration

**Goal**: `WOODPECKER_GITEE=true` is recognized and `setup.Forge()` can construct a Gitee instance. Start with a **compilable shell** (all methods panic or return placeholders); later tasks fill them in one by one.

**Files**

1. `server/model/forge.go`
   - Add `ForgeTypeGitee ForgeType = "gitee"` to the `const` block

2. `cmd/server/flags.go`
   - Append after the `// Gitea` section:
     ```go
     //
     // Gitee
     //
     &cli.BoolFlag{
         Sources: cli.EnvVars("WOODPECKER_GITEE"),
         Name:    "gitee",
         Usage:   "gitee driver is enabled",
     },
     ```
   - Add `"WOODPECKER_GITEE_URL"` to the `Sources` of `forge-url`
   - Add `"WOODPECKER_GITEE_CLIENT_FILE"` (file source) and `cli.EnvVar("WOODPECKER_GITEE_CLIENT")` to the `forge-oauth-client` chain
   - Add `"WOODPECKER_GITEE_SECRET_FILE"` and `cli.EnvVar("WOODPECKER_GITEE_SECRET")` to the `forge-oauth-secret` chain
   - Add `"WOODPECKER_GITEE_SKIP_VERIFY"` to the `Sources` of `forge-skip-verify`

3. `server/services/setup.go` — add to the switch in `setupForgeService()`:
   ```go
   case c.Bool("gitee"):
       _forge.Type = model.ForgeTypeGitee
       if _forge.URL == "" {
           _forge.URL = "https://gitee.com"
       }
   ```
   Note: it must be placed after `bitbucket-dc` and before `default:`.

4. `server/forge/setup/setup.go`
   - import `go.woodpecker-ci.org/woodpecker/v3/server/forge/gitee`
   - add `case model.ForgeTypeGitee: return setupGitee(forge)` to the `Forge()` switch
   - add `setupGitee()` with the same signature as `setupGitea()` (see design document 4.3)

5. Create `server/forge/gitee/` (6 files, see design document 4.1)
   ```go
   // gitee.go
   package gitee

   const (
       authorizeTokenURL = "%s/oauth/authorize"
       accessTokenURL    = "%s/oauth/token"
       defaultPageSize   = 50
   )

   type Gitee struct {
       id                int64
       url               string
       oAuthClientID     string
       oAuthClientSecret string
       oAuthHost         string
       skipVerify        bool
   }

   type Opts struct {
       URL               string
       OAuthClientID     string
       OAuthClientSecret string
       OAuthHost         string
       SkipVerify        bool
   }

   func New(id int64, opts Opts) (forge.Forge, error) { ... }

   func (c *Gitee) Name() string { return "gitee" }
   func (c *Gitee) URL() string  { return c.url }
   ```

6. `server/forge/setup/setup_test.go` — add:
   ```go
   func TestForgeGiteeRequiresURL(t *testing.T) {
       t.Parallel()
       _, err := Forge(&model.Forge{Type: model.ForgeTypeGitee, URL: ""})
       assert.Error(t, err)
   }
   ```

**All new `.go` files must carry the Apache license header** (you can run `make generate-license-header` at the end).

**Acceptance Criteria**

```bash
go build ./...
WOODPECKER_GITEE=true WOODPECKER_GITEE_CLIENT=x WOODPECKER_GITEE_SECRET=y \
  WOODPECKER_HOST=http://localhost:8000 ./woodpecker-server 2>&1 | grep -i gitee
# should show "setting up forge" with type=gitee
```

---

## T2 — `types.go` + `helper.go`

**Goal**: wrap the Gitee API client and handle authentication, pagination and error mapping.

**Files**: `server/forge/gitee/types.go`, `server/forge/gitee/helper.go`

**Steps**

1. `types.go` — define response structs (field names aligned with the Gitee docs):
   - `User`: `ID`, `Login`, `Name`, `Email`, `AvatarURL`
   - `Repository`: `ID`, `FullName`, `Name`, `Owner{Login}`, `HTMLURL`, `CloneURL`, `SSHURL`, `Private`, `DefaultBranch`, `Permissions{Admin,Push,Pull}`
   - `Branch`: `Name`, `Commit{SHA,URL}`
   - `Content`: `Content` (base64), `Encoding`, `Name`, `Path`

2. `helper.go` — implement:
   - `newHTTPClient(skipVerify bool) *http.Client` (see the `&http.Transport{TLSClientConfig: ...}` in `gitea.go`'s `oauth2Config`)
   - `get(ctx, path string, accessToken string, out any) error`
     - URL: `{c.url}/api/v5{path}`
     - Auth: **`?access_token=xxx`** query parameter, or the `Authorization: token xxx` header
       > ❗ Not Bearer — do not copy `newClientToken()` from `server/forge/gitea/gitea.go`
     - 404 must be recognizable by the caller (`server/forge/types` provides `ErrRepoNotFound`, `ErrConfigNotFound`)
   - `paginate(page, perPage)` loop: `GET ...?page=N&per_page=50`
     - **No Link header**, so terminate when `len(items) < perPage`
   - Always resolve the user token via `common.UserToken(ctx, r, u)` (reuse `server/forge/common/`)

**Acceptance Criteria**: write `helper_test.go` using `httptest.Server` to fake Gitee responses, covering: correct auth parameter, pagination termination, 404 mapping.

```bash
go test ./server/forge/gitee/... -run TestHelper -v
```

---

## T3 — `Login()` + `oauth2Config()` + `Refresh()`

**Goal**: complete the OAuth login loop and automatic token refresh.

**Files**: `server/forge/gitee/gitee.go`

**Steps**

1. `oauth2Config(ctx)` — mirror `server/forge/gitea/gitea.go:92-111`, only replacing the endpoint constants with `authorizeTokenURL` / `accessTokenURL` (see T1).
   - `RedirectURL` must be `fmt.Sprintf("%s/authorize", server.Config.Server.OAuthHost)`
   - `publicOAuthURL`: fall back to `c.url` when `c.oAuthHost` is empty

2. `Login()` — implement the two phases (full code in design document 5.1):
   - empty `req.Code` → `return nil, redirectURL, nil`
   - otherwise `config.Exchange` → exchange token → `GET /api/v5/user` → build `model.User`
   - **Must set**: `Login`, `Email`, `Avatar`, `AccessToken`, `RefreshToken`, `Expiry`, `ForgeRemoteID`
   - ⚠️ `model.User` has **no `Name` field** — do not map Gitee's `name`
   - ⚠️ `Login` must satisfy `^[a-zA-Z0-9-_.]+$` (see `Validate()` in `server/model/user.go`)

3. `Refresh(ctx, u *model.User) (bool, error)` — **must be implemented in Stage 1**, Gitee tokens only live 1 day:
   ```go
   func (c *Gitee) Refresh(ctx context.Context, user *model.User) (bool, error) {
       config, oauth2Ctx := c.oauth2Config(ctx)
       config.RedirectURL = ""
       source := config.TokenSource(oauth2Ctx, &oauth2.Token{
           AccessToken:  user.AccessToken,
           RefreshToken: user.RefreshToken,
           Expiry:       time.Unix(user.Expiry, 0),
       })
       token, err := source.Token()
       if err != nil || len(token.AccessToken) == 0 {
           return false, err
       }
       user.AccessToken = token.AccessToken
       user.RefreshToken = token.RefreshToken
       user.Expiry = token.Expiry.UTC().Unix()
       return true, nil
   }
   ```
   > Gitee returns a **new** `refresh_token` on refresh as well; both must be persisted.

**Acceptance Criteria**

- Interface assertions pass:
  ```go
  var _ forge.Forge = (*Gitee)(nil)
  var _ forge.Refresher = (*Gitee)(nil)
  ```
- Completing authorization in a browser persists the user; the `users` table has `refresh_token` and `expiry` populated

---

## T4 — `Repo()` / `Repos()` + `parse.go` Repo Mapping

**Goal**: repository list sync and single-repo lookup.

**Files**: `server/forge/gitee/gitee.go`, `parse.go`, **`parse_test.go`**, **`fixtures/`**

**Steps**

1. `Repos()`:
   - `GET /api/v5/user/repos?page=N&per_page=50`, paginate through everything
   - Skip archived repos (mirroring `if repo.Archived { continue }` in `gitea.go`)
   - v3 has an internal pagination convention: `if p.Page != 1 { return nil, nil }` (see `gitea.go:242`) — keep consistent

2. `Repo()`:
   - When `remoteID.IsValid()`, prefer `GET /api/v5/repos/{id}` (Gitee supports lookup by id)
   - Otherwise `GET /api/v5/repos/{owner}/{name}`
   - On 404 → `errors.Join(err, forge_types.ErrRepoNotFound)`

3. `toRepo()` in `parse.go` — map per design document 8.2:

   | Gitee | model.Repo |
   |---|---|
   | `id` | `ForgeRemoteID` |
   | `full_name` | `FullName` |
   | `name` | `Name` |
   | `owner.login` | `Owner` |
   | `html_url` | `ForgeURL` |
   | `clone_url` | `Clone` |
   | `ssh_url` | `CloneSSH` |
   | `private` | `IsSCMPrivate` + `Visibility` (or just call `repo.ResetVisibility()`) |
   | `default_branch` | `Branch` |
   | `permissions` | `Perm{Pull, Push, Admin}` |

4. `fixtures/`: save real captured responses as JSON (follow the structure of `server/forge/gitea/fixtures/`, including the embed declarations in `helper_test.go`)

5. `parse_test.go`: table-driven assertions for the field mapping

**Acceptance Criteria**

```bash
go test ./server/forge/gitee/... -run TestParse -v
```

---

## T5 — `File()` + `BranchHead()`

**Goal**: make builds actually run (without `File()` there is no pipeline configuration at all).

**Files**: `server/forge/gitee/gitee.go`

**Steps**

1. `File(ctx, u, r, b, fileName)`:
   - `GET /api/v5/repos/{owner}/{repo}/contents/{path}?ref={b.Commit}`
   - `content` is base64 (possibly with newlines) — decode with `base64.StdEncoding.DecodeString`
   - On 404 → `errors.Join(err, &forge_types.ErrConfigNotFound{Configs: []string{fileName}})`
   - ⚠️ Must fetch at `b.Commit`, **not** at the branch head

2. `BranchHead(ctx, u, r, branch)`:
   - `GET /api/v5/repos/{owner}/{repo}/branches/{branch}`
   - Return `&model.Commit{SHA: ..., ForgeURL: ...}`
   - cron depends on this method — do not return `ErrNotImplemented`

**Acceptance Criteria**: triggering a build manually reads `.woodpecker.yaml` and generates a workflow.

---

## T6 — `Netrc()`

**Goal**: private repositories can be cloned.

**Files**: `server/forge/gitee/gitee.go`

**Steps** (see `gitea.go:345-360`)

```go
func (c *Gitee) Netrc(u *model.User, r *model.Repo) (*model.Netrc, error) {
    login, token := "", ""
    if u != nil {
        login = u.Login
        token = u.AccessToken
    }
    host, err := common.ExtractHostFromCloneURL(r.Clone)
    if err != nil {
        return nil, err
    }
    return &model.Netrc{
        Machine:  host,
        Login:    login,
        Password: token,
        Type:     model.ForgeTypeGitee,
    }, nil
}
```

> `u` may be nil (public repos) — the nil check is mandatory.

**Acceptance Criteria**: activate a **private** repo and trigger a build; the clone step succeeds.

---

## T7 — `Activate()` / `Deactivate()` Placeholders

**Goal**: repo activation must not return HTTP 500.

**Files**: `server/forge/gitee/gitee.go`

**Steps**

```go
// Stage 1: WebHooks not implemented yet — return nil (must NOT return ErrNotImplemented)
func (c *Gitee) Activate(context.Context, *model.User, *model.Repo, string) error {
    return nil
}

func (c *Gitee) Deactivate(context.Context, *model.User, *model.Repo, string) error {
    return nil
}
```

> ⚠️ **Never return an error.** `server/api/repo.go:179` turns an `Activate` error straight into HTTP 500, making the repo impossible to activate.
> The trade-off is no WebHook auto-triggering in Stage 1, which matches the design goal. See T14 for Stage 2.

**Acceptance Criteria**: activating a repo in the UI returns 200; `woodpecker-cli repo add` succeeds.

---

## T8 — Placeholders for the Remaining 8 Methods

**Goal**: satisfy the compile-time interface requirements.

**Files**: `server/forge/gitee/gitee.go`

| Method | Stage 1 implementation |
|---|---|
| `Teams()` | `return nil, forge_types.ErrNotImplemented` |
| `Dir()` | `return nil, forge_types.ErrNotImplemented` |
| `Branches()` | `return nil, forge_types.ErrNotImplemented` |
| `PullRequests()` | `return nil, forge_types.ErrNotImplemented` |
| `OrgMembership()` | `return nil, forge_types.ErrNotImplemented` |
| `Org()` | `return nil, forge_types.ErrNotImplemented` |
| `Status()` | `return nil` (no reporting yet; see T15 for Stage 2) |
| `Hook()` | `return nil, nil, fmt.Errorf("webhook not implemented")` (see T13 for Stage 2) |

> Only the callers of `Teams` / `Dir` / `Branches` / `PullRequests` / `OrgMembership` degrade gracefully on `ErrNotImplemented`,
> so those 5 are safe. For `Org()` and `Hook()`, verify the caller behaviour, implement as above and watch the logs.

**Acceptance Criteria**: `go build ./...` passes; the server starts without panicking.

---

## T9 — Web Frontend

**Files**

1. `web/src/lib/api/types/forge.ts`
   ```ts
   export type ForgeType = 'github' | 'gitlab' | 'gitea' | 'bitbucket' | 'bitbucket-dc' | 'addon' | 'forgejo' | 'gitee';
   ```

2. `web/src/components/atomic/Icon.vue`
   - Add `'gitee'` to the `IconName` union
   - Add a `v-else-if="name === 'gitee'"` branch (use `siGitee` from `simple-icons`, or a placeholder icon from `@mdi/js`)

> The login page `web/src/views/Login.vue` renders generically (hostname + favicon from `forge.url`),
> so **no dedicated Gitee login button is needed**, and no fixed "Login with Gitee" label will appear.

**Acceptance Criteria**

```bash
cd web && pnpm install && pnpm typecheck && pnpm lint && pnpm build
```

---

## T10 — Admin Documentation

**Files**: `docs/docs/30-administration/10-configuration/12-forges/xx-gitee.md`

> ⚠️ CI has a gate — `.woodpecker/check-feature-docs.sh`: a PR labelled `feature` fails if `docs/docs/` has no changes.

**Suggested content** (follow `30-gitea.md` in the same directory): overview, Gitee OAuth application registration steps, environment variable table, full configuration example, known limitations.

**Acceptance Criteria**: the file exists and is picked up by the docusaurus sidebar.

---

## T11 — Quality Gates

**Steps**

```bash
# 1. license header (required for new .go files)
make generate-license-header

# 2. code generation (mockery + go generate)
make generate

# 3. lint
make lint

# 4. unit tests
make test-server
go test ./server/forge/gitee/... -race -cover

# 5. frontend
cd web && pnpm typecheck && pnpm lint && pnpm build
```

**Notes**

- In `.mockery.yaml`, `server/forge` is `recursive: true`, but no new interface is added, so mocks normally do not change; if `make generate` produces a diff, commit it
- golangci-lint uses **v2** (see the Makefile) — match the local version

**Acceptance Criteria**: everything above passes, with no files changed beyond the intended diff.

---

## T12 — End-to-End Acceptance

Start the server:

```bash
WOODPECKER_GITEE=true \
WOODPECKER_GITEE_CLIENT=<id> \
WOODPECKER_GITEE_SECRET=<secret> \
WOODPECKER_HOST=http://192.168.1.10:8000 \
WOODPECKER_GRPC_ADDR=:9000 \
WOODPECKER_ADMIN=<your-gitee-username> \
WOODPECKER_AGENT_SECRET=<shared-secret> \
./woodpecker-server
```

Agent (official binary, same on all three machines):

```bash
WOODPECKER_SERVER=192.168.1.10:9000 \
WOODPECKER_AGENT_SECRET=<shared-secret> \
WOODPECKER_BACKEND=local \
woodpecker-agent
```

Verify each case from Chapter 10 of the design document:

| # | Case | Result |
|---|---|---|
| 1 | `/login` shows a button with hostname `gitee.com` + favicon | ✅ |
| 2 | Redirects to `https://gitee.com/oauth/authorize?...&redirect_uri=http://192.168.1.10:8000/authorize` | ✅ |
| 3 | After authorization, redirects to `/authorize` and lands on the home page | ✅ |
| 4 | Repo sync fetches all public / private repos | ✅ |
| 5 | Repo activation succeeds (the `Activate()` placeholder must not error) | ✅ |
| 6 | Manual build is dispatched to the Windows / Mac / Linux agents | ✅ |
| 7 | Build completes, logs correct, artifacts packaged | ✅ |
| 8 | **A day later, without logging in again, sync / builds still work** (verifies `Refresh()`) | ✅ |

> Case 8 is the easiest to miss. To speed up verification, manually set `users.expiry` to one hour in the future and trigger a sync.

---

## Stage 2 Tasks (T13 onwards)

### T13 — `Hook()` WebHook Event Parsing
- Gitee WebHooks support `Push Hook`, `Tag Push Hook`, `Pull Request Hook`, `Note Hook`
- Implement signature verification (Gitee uses a plaintext secret in `X-Gitee-Token`, or `X-Gitee-Signature` + timestamp HMAC)
- Return semantics are in the design document: `(repo, pipeline, nil)` / `(repo, nil, nil)` / `ErrIgnoreEvent`
- The route already exists at `/api/hook` in `server/router/router.go` — nothing new needed

### T14 — Real `Activate()` / `Deactivate()`
- `POST /api/v5/repos/{owner}/{repo}/hooks`, with `url` pointing at `$WOODPECKER_HOST/api/hook`
- Take the webhook secret from `r.Hash`
- In `Deactivate`, a missing webhook must be **ignored**, not treated as an error

### T15 — `Status()` Build Status Reporting
- `POST /api/v5/repos/{owner}/{repo}/statuses/{sha}`
- Reuse `common.GetPipelineStatusURL()` / `GetPipelineStatusDescription()` / `GetPipelineStatusContext()`
- Failures are logged only and must not block the pipeline

### T16 — `PullRequests()` + PR Events
- `GET /api/v5/repos/{owner}/{repo}/pulls?state=open`
- Requires adding the `pull_requests` scope

### T17 — Org and Team Permissions
- `Org()` / `OrgMembership()` / `Teams()`: `GET /api/v5/user/orgs`, `GET /api/v5/orgs/{org}/members`
- `Branches()`: `GET /api/v5/repos/{owner}/{repo}/branches`
- `Dir()`: `GET /api/v5/repos/{owner}/{repo}/contents/{dir}?ref={sha}`, then fetch each file

---

## 2. Risks and Notes

| Risk | Impact | Mitigation |
|---|---|---|
| `Activate()` returns an error | Repo activation returns 500, completely unusable | Must return `nil` in Stage 1 (T7) |
| `Refresh()` not implemented | Gitee tokens expire in 1 day; everything 401s the next day | Must implement in Stage 1 (T3) |
| Copying Gitea's Bearer auth | Every API call returns 401 | Use `?access_token=` (design document 3.5) |
| Paginating by Link header | Infinite loop or missing data | Terminate when "returned items < perPage" (T2) |
| `File()` fetching the branch head instead of `b.Commit` | Build uses the wrong version of the config | Always pass `ref={b.Commit}` (T5) |
| Using `Repo.Link` / `Repo.IsPrivate` | Compile failure | Use `ForgeURL` / `IsSCMPrivate` (design document 8.2) |
| Gitee username with illegal characters | `User.Validate()` fails, user cannot log in | Use the `login` field; sanitize if needed and surface a clear error |
| Missing `docs/docs/` change | CI `check-feature-docs.sh` fails | T10 must not be skipped |
| New `.go` file without license header | Lint failure | `make generate-license-header` (T11) |

---

## 3. Definition of Done

- [ ] T0–T12 all completed
- [ ] `make lint`, `make test-server`, and the web `typecheck`/`lint`/`build` are all green
- [ ] All 8 acceptance cases in Chapter 10 of the design document pass
- [ ] `docs/docs/30-administration/10-configuration/12-forges/xx-gitee.md` committed
- [ ] Agents run the official binaries with no modifications
