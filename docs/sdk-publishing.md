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
   - PyPI Project Name: `echoproxy-sdk`
   - Owner: `songhieu`
   - Repository name: `EchoProxy`
   - Workflow name: `publish-sdks.yml`
   - Environment name: `pypi`
4. Save. Next tag push → publish works with zero secret to rotate.

**Option B — API token (fallback):**

1. https://pypi.org/manage/account/token → Create token, scope to project `echoproxy-sdk`
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
`@songhieu/echoproxy-sdk` or just unscoped `echoproxy-sdk`.

**Token:**

1. https://www.npmjs.com/settings/<your-username>/tokens → Generate New Token → **Automation** (not Publish — Automation tokens skip 2FA, required for CI)
2. GitHub: Settings → Secrets → New repository secret
3. Name: `NPM_TOKEN`, value: paste token

Note: the workflow uses `--provenance` which requires GitHub Actions OIDC.
This works automatically once `id-token: write` permission is set (already done).
Provenance is great for supply-chain trust — published packages get a
verifiable "built from commit X via workflow Y" attestation.

### 3. Packagist — Laravel SDK

One-time setup, **no CI**:

1. Go to https://packagist.org/packages/submit
2. Paste: `https://github.com/songhieu/EchoProxy`
3. Submit. Packagist accepts the package as `echoproxy/sdk-laravel` (matches `composer.json` `name`).
4. Optionally enable GitHub webhook for instant updates:
   - On Packagist package page → Settings → "Set up a service hook on GitHub" button
   - Or manually: GitHub repo Settings → Webhooks → Add webhook → URL `https://packagist.org/api/github?username=songhieu`, content type JSON, secret from Packagist API page

Without webhook, Packagist polls every ~30 minutes. With webhook, new tags appear within seconds.

⚠️ **The package name `echoproxy/sdk-laravel`** must match `composer.json` `name`. If you change the composer name, also resubmit on Packagist (or rename via Packagist UI).

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
pip index versions echoproxy-sdk

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
