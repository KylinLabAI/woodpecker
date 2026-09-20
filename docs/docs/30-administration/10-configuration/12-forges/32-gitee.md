---
toc_max_heading_level: 2
---

# Gitee

Woodpecker comes with built-in support for [Gitee](https://gitee.com). To enable Gitee you should configure the Woodpecker server using the following environment variables:

```ini
WOODPECKER_GITEE=true
WOODPECKER_GITEE_URL=https://gitee.com
WOODPECKER_GITEE_CLIENT=YOUR_GITEE_CLIENT
WOODPECKER_GITEE_SECRET=YOUR_GITEE_CLIENT_SECRET
```

:::warning
Gitee support is currently in an early stage. The sections below describe what is implemented today and which features are still missing. See [Known limitations](#known-limitations) before enabling it in production.
:::

## Gitee OAuth Application

Register your application with Gitee to create your client id and secret. You can manage OAuth applications at `https://gitee.com/oauth/applications`.

1. Create a new application.
2. Set the **Application Homepage** to the URL of your Woodpecker instance (e.g. `https://ci.example.com`).
3. Set the **Authorization callback URL** to `https://<host>/authorize` — the scheme and hostname must match your Woodpecker instance exactly.
4. After creating the application, copy the **Client ID** and **Client Secret** into the `WOODPECKER_GITEE_CLIENT` and `WOODPECKER_GITEE_SECRET` configuration values.

:::info
Gitee access tokens issued through the OAuth flow expire after **one day**. Woodpecker refreshes the token automatically, so no manual intervention is required as long as the OAuth application remains valid.
:::

## Configuration

This is a full list of configuration options. Please note that many of these options use default configuration values that should work for the majority of installations.

---

### GITEE

- Name: `WOODPECKER_GITEE`
- Default: `false`

Enables the Gitee driver.

---

### GITEE_URL

- Name: `WOODPECKER_GITEE_URL`
- Default: `https://gitee.com`

Configures the Gitee server address.

---

### GITEE_CLIENT

- Name: `WOODPECKER_GITEE_CLIENT`
- Default: none

Configures the Gitee OAuth client id. This is used to authorize access.

---

### GITEE_CLIENT_FILE

- Name: `WOODPECKER_GITEE_CLIENT_FILE`
- Default: none

Read the value for `WOODPECKER_GITEE_CLIENT` from the specified filepath.

---

### GITEE_SECRET

- Name: `WOODPECKER_GITEE_SECRET`
- Default: none

Configures the Gitee OAuth client secret. This is used to authorize access.

---

### GITEE_SECRET_FILE

- Name: `WOODPECKER_GITEE_SECRET_FILE`
- Default: none

Read the value for `WOODPECKER_GITEE_SECRET` from the specified filepath.

---

### GITEE_SKIP_VERIFY

- Name: `WOODPECKER_GITEE_SKIP_VERIFY`
- Default: `false`

Configure if SSL verification should be skipped.

## Known limitations

The Gitee integration is still being completed. The following is implemented:

- OAuth login and automatic token refresh.
- Listing and fetching repositories the user can access.
- Reading the pipeline configuration file at the exact commit that triggered a build.
- Resolving the latest commit of a branch (required for the cron feature).
- Generating the `.netrc` credentials used to clone repositories.

The following features are **not yet implemented** and will be added in a later release:

- Webhook-based build triggers (push and pull-request events). Until this lands, activating a repository does not register a webhook, so pushes will not automatically start builds.
- Commit status reporting back to Gitee.
- Pull-request builds, organization membership, teams, branch listing and directory browsing.

Because webhooks are not registered yet, you can still trigger a build manually from the Woodpecker UI once a repository has been activated.
