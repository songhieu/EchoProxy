# Publishing the SDKs

EchoProxy ships 4 SDKs, each to its native package registry. The
GitHub Actions workflow `.github/workflows/publish-sdks.yml` automates
3 of them; Go is "publish-by-tag" so it's automatic without a workflow
step (the tag itself IS the release).

| SDK | Registry | Automation | First-time setup |
|-----|----------|-----------|------------------|
| Python (`sdk-python`) | PyPI | ✓ on tag push | PYPI_API_TOKEN secret **or** Trusted Publishers |
| TypeScript (`sdk-ts`) | npm | ✓ on tag push | NPM_TOKEN secret + reserve `@echoproxy` scope |
| Laravel (`sdk-laravel`) | Packagist | tag triggers Packagist poll | Submit URL once at packagist.org |
| Go (`sdk-reference-go`) | proxy.golang.org | tag IS the release | nothing — proxy auto-fetches |

## Workflow: cut one tag, publish everywhere

```bash
git tag v0.2.0
git push origin v0.2.0
```

This triggers BOTH:
1. `release.yml` → rebuilds 7 container images at `:0.2.0`, publishes Helm chart at `oci://ghcr.io/songhieu/charts/echoproxy:0.2.0`, creates GitHub Release
2. `publish-sdks.yml` → publishes Python + TS to PyPI/npm at `0.2.0`; nudges Go proxy + Packagist auto-polls

All 4 SDKs versioned together. Stripe-style.

## First-time setup

### 1. PyPI — Python SDK

**Option A — Trusted Publishers (recommended, no token to manage):**

1. Go to https://pypi.org/manage/account/publishing/
2. Click **Add a new publisher** → **GitHub**
3. Fill in:
   - PyPI Project Name: `echoproxy`
   - Owner: `songhieu`
   - Repository name: `EchoProxy`
   - Workflow name: `publish-sdks.yml`
   - Environment name: `pypi`
4. Save. Next tag push → publish works with zero secret to rotate.

**Option B — API token (fallback):**

1. https://pypi.org/manage/account/token → Create token, scope to project `echoproxy`
2. GitHub: Settings → Secrets and variables → Actions → New repository secret
3. Name: `PYPI_API_TOKEN`, value: paste token
4. Uncomment the `password:` line in `publish-sdks.yml` under the `python` job

**First publish quirk**: PyPI Trusted Publishers can't be configured until the project exists. Either upload the first version via Option B + an API token, OR submit a 1-line "pending publisher" claim on PyPI before the first publish (see https://docs.pypi.org/trusted-publishers/creating-a-project-through-oidc/).

### 2. npm — TypeScript SDK

**Reserve the scope first** (if not done already):

```bash
# Login at npmjs.com → Settings → Organizations → create "echoproxy"
# Scope must be set to "Unlimited public packages" (free for OSS)
```

Alternative if `@echoproxy` is taken: change `package.json` `name` to
`@songhieu/echoproxy` or just unscoped `echoproxy`.

**Token:**

1. https://www.npmjs.com/settings/<your-username>/tokens → Generate New Token → **Automation** (not Publish — Automation tokens skip 2FA, required for CI)
2. GitHub: Settings → Secrets → New repository secret
3. Name: `NPM_TOKEN`, value: paste token

Note: the workflow uses `--provenance` which requires GitHub Actions OIDC.
This works automatically once `id-token: write` permission is set (already done).
Provenance is great for supply-chain trust — published packages get a
verifiable "built from commit X via workflow Y" attestation.

### 3. Packagist — Laravel SDK (via subtree mirror)

**Why subtree mirror**: Packagist + Composer require `composer.json` at
the repo ROOT. EchoProxy is a monorepo with the SDK at `sdk-laravel/`,
so we follow the standard pattern (same as Symfony / Laravel themselves):
mirror the subdirectory to a standalone repo, then submit *that* to
Packagist. The mirror is automated via `.github/workflows/sdk-laravel-mirror.yml`.

**One-time setup:**

1. **Create empty mirror repo** on GitHub:
   - Name: `echoproxy-laravel`
   - Visibility: Public
   - **Do not** initialize with README/license — the mirror replaces history

2. **Generate a PAT** for the mirror to push:
   - https://github.com/settings/tokens/new
   - Note: `EchoProxy sdk-laravel mirror`
   - Scope: `repo` (full control). Expiration: as long as you trust.
   - Copy the token.

3. **Add as secret** on the monorepo:
   - `https://github.com/songhieu/EchoProxy/settings/secrets/actions` → New repository secret
   - Name: `MIRROR_TOKEN`, value: paste token.

4. **Trigger the mirror once** to populate the new repo:
   ```bash
   gh workflow run sdk-laravel-mirror.yml -R songhieu/EchoProxy
   # or just push any commit to main
   ```
   Watch it: `gh run watch -R songhieu/EchoProxy`

5. **Submit the MIRROR repo to Packagist** (NOT the monorepo):
   - https://packagist.org/packages/submit
   - Paste: `https://github.com/songhieu/echoproxy-laravel`
   - Submit. Now Packagist sees `composer.json` at the root and accepts.
   - Package URL: https://packagist.org/packages/echoproxy/sdk-laravel

6. **(Optional) Webhook for instant updates** instead of 30-min poll:
   - On Packagist package page → Settings → "Set up a service hook on GitHub"
   - This installs a webhook on the `echoproxy-laravel` repo (not the monorepo)
   - Any tag pushed there now refreshes Packagist within seconds

**Per-release:** nothing manual. `git push origin v0.2.0` from the
monorepo triggers `sdk-laravel-mirror.yml` → tag `v0.2.0` lands on the
mirror repo → Packagist pulls.

⚠️ **Composer package name** `echoproxy/sdk-laravel` (from
`composer.json`) is independent of the mirror repo name. Don't change
the `composer.json` `name` field unless you also rename the Packagist
package.

### 4. Go — sdk-reference-go

**No registry, no CI step, no token.** The Go module proxy at
proxy.golang.org auto-fetches any tagged version on first `go get`.

The only thing needed: the Go module path matches the GitHub URL. Done — `sdk-reference-go/go.mod` declares:

```
module github.com/songhieu/EchoProxy/sdk-reference-go
```

After a tag `v0.2.0`, end users run:

```bash
go get github.com/songhieu/EchoProxy/sdk-reference-go@v0.2.0
```

The proxy fetches once and caches forever (immutability guarantee).

**Note on submodule tagging**: since this is a Go submodule (not at the
repo root), the *strict* Go modules pattern would tag as
`sdk-reference-go/v0.2.0`. But since we version all SDKs together, we
use the simpler shared `v0.2.0` tag at the repo root — Go modules
accepts this too as long as the SDK's go.mod is at the path the user
imports.

## Verification

After a tag push, all four registries should reflect the new version
within a few minutes:

```bash
# PyPI
pip index versions echoproxy

# npm
npm view @echoproxy/sdk versions --json

# Packagist (web)
open https://packagist.org/packages/echoproxy/sdk-laravel

# Go (any directory)
go list -m -versions github.com/songhieu/EchoProxy/sdk-reference-go
```

If any show "version not found", check the corresponding workflow run in
GitHub Actions for errors.

## Bumping versions

Use semver discipline:

| Change | Bump |
|--------|------|
| Bug fix, no API change | patch (`v0.1.0` → `v0.1.1`) |
| New optional feature, back-compat | minor (`v0.1.0` → `v0.2.0`) |
| Breaking API change | major (`v0.1.0` → `v1.0.0`) |

For pre-`v1.0.0` packages, conventional practice is to use minor for
breaking changes too — users know to read the changelog when bumping
`0.x` versions. After `v1.0.0`, semver is strict.

## Rolling back a release

PyPI, npm: can yank a version (marks as "do not auto-install") but
cannot delete published versions for any version that has been
downloaded. Packagist & Go: cannot delete tagged versions ever
(immutability guarantee, by design).

**Best practice**: don't yank, ship `v0.2.1` with the fix. The buggy
version stays around but installs steer users to the latest.
