# MAINTENANCE

This document describes how the project team maintains the Tingly Box project, including CI/CD and operations practices.

## CI/CD Workflow

### Branch Naming Convention

All CI branches should be created with the `ci/` prefix:

```bash
git checkout -b ci/your-fix-description
```

Examples:
- `ci/openai-api-fix`
- `ci/routing-bug-hotfix`
- `ci/smooth-rule-enhancement`

### Automatic Build Trigger

Pushing to any `ci/*` branch automatically triggers the build pipeline:

```bash
git push origin ci/your-fix-description
```

The GitHub Actions workflow (`.github/workflows/gh-release.yml`) will:
1. Build the frontend (React + MUI)
2. Generate API client from swagger.json
3. Build CLI binaries for all platforms (linux-amd64, linux-arm64, macos-amd64, macos-arm64, windows-amd64)
4. Compress binaries with UPX
5. Upload build artifacts

### Verification Steps

Before proceeding to release:

1. **Check the CI workflow**: Navigate to the Actions tab in GitHub and verify:
   - All jobs completed successfully
   - No build errors
   - No test failures

2. **Verify the fix**: Ensure the change works as expected

### Creating Release Tags

Once the fix is verified and the pipeline passes, create a release tag:

```bash
# Check existing tags for reference
git tag --sort=-creatordate | head -20

# Create a new tag (format: v0.YYYYMMDD.HHMM-type)
git tag v0.20260403.2230-hotfix

# Push the tag to trigger release
git push origin v0.20260403.2230-hotfix
```

### Tag Naming Convention

```
v0.YYYYMMDD.HHMM-type
```

- `YYYYMMDD`: Release date (year, month, day)
- `HHMM`: Release time (hour, minute) in 24-hour format
- `type`: Release type
  - `hotfix`: Quick fix for critical issues
  - `preview`: Pre-release for testing
  - No suffix: Stable release

Examples:
- `v0.20260403.2230-hotfix` - Hotfix on 2026-04-03 at 22:30
- `v0.20260403.1200-preview` - Preview release
- `v0.20260402.1800` - Stable release

### Release Process

When a tag is pushed:

1. The GitHub Actions workflow detects the tag push
2. All platform binaries are built
3. A GitHub Release is automatically created with:
   - All platform binaries as zip files
   - SHA256 checksums
   - Auto-generated release notes

## Build Artifacts

### CLI Binaries

The following binaries are built for each release:

| Platform | Architecture | Artifact Name |
|----------|--------------|---------------|
| Linux | AMD64 | `tingly-box-linux-amd64.zip` |
| Linux | ARM64 | `tingly-box-linux-arm64.zip` |
| macOS | AMD64 (Intel) | `tingly-box-macos-amd64.zip` |
| macOS | ARM64 (Apple Silicon) | `tingly-box-macos-arm64.zip` |
| Windows | AMD64 | `tingly-box-windows-amd64.zip` |

### Optimization

All binaries are:
- Built with `-trimpath` for smaller stack traces
- Stripped of symbols (`-s -w` ldflags)
- Compressed with UPX using LZMA for maximum compression

### GUI Builds (Optional)

GUI versions can be built via manual workflow dispatch:
- macOS: `tingly-box-gui-macos-arm64.zip` (TinglyBox.app)
- Windows: `tingly-box-gui-windows-amd64.zip` (TinglyBox.exe)

## Development Workflow

### Local Build

For local testing:

```bash
# Build frontend
cd frontend && pnpm install && pnpm build

# Build CLI
go build ./cli/tingly-box
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/protocol/...
```

## Operations

### Monitoring

- CI status: Check GitHub Actions tab
- Release artifacts: Check GitHub Releases page

### Rollback Procedure

If a release introduces issues:

1. Identify the problematic commit
2. Create a fix branch: `git checkout -b ci/rollback-issue`
3. Implement the fix
4. Push to trigger CI: `git push origin ci/rollback-issue`
5. Create new hotfix tag: `git tag v0.YYYYMMDD.HHMM-hotfix`
6. Push tag: `git push origin v0.YYYYMMDD.HHMM-hotfix`

## References

- CI/CD Configuration: [`.github/workflows/gh-release.yml`](../.github/workflows/gh-release.yml)
- NPX Publish Workflow: [`.github/workflows/gh-npx-publish.yml`](../.github/workflows/gh-npx-publish.yml)
- User Manual: [`docs/user-manual.md`](user-manual.md)
- Docker Setup: [`docs/docker.md`](docker.md)

## NPX Package Publishing

After a GitHub Release is created, publish NPX packages to npm for easy installation.

### Workflow: gh-npx-publish.yml

This is a manual workflow that publishes npm packages based on an existing GitHub Release.

### Steps to Publish NPX Packages

1. **Navigate to Actions tab** in GitHub
2. **Select "NPX Package Publish from GitHub Release"** workflow
3. **Click "Run workflow"**
4. **Fill in the required inputs**:

   | Input | Description | Example |
   |-------|-------------|---------|
   | `release_tag` | GitHub Release tag to download assets from | `v0.20260403.2230-hotfix` |
   | `npx_version` | NPX package version (defaults to tag without `v`) | `0.20260403.2230-hotfix` |
   | `publish_cli` | Publish CLI package (`tingly-box`) | `true` |
   | `publish_bundle` | Publish bundle package (`tingly-box-bundle`) | `true` |
   | `publish_gui` | Publish GUI package (`tingly-box-gui`) | `false` |

5. **Important**: The `release_tag` must match an existing GitHub Release tag
6. **The `npx_version`** should match the `release_tag` (without the `v` prefix) for consistency

### Published Packages

| Package | Description | Install Command |
|---------|-------------|-----------------|
| `tingly-box` | CLI package (downloads binary on first run) | `npx tingly-box@version` |
| `tingly-box-bundle` | Bundle package (includes pre-built binaries, ~70MB) | `npx tingly-box-bundle@version` |
| `tingly-box-gui` | GUI package (desktop app) | `npx tingly-box-gui@version start` |

### NPX Tags

When the workflow completes, it creates git tags in the format `npx-{version}`:

- `npx-0.20260403.2230-hotfix` - For CLI + Bundle releases
- `npx-gui-0.20260403.2230-hotfix` - For GUI releases

### Example: Complete Release Flow

```bash
# 1. Create CI branch
git checkout -b ci/your-fix

# 2. Make changes and push
git push origin ci/your-fix

# 3. Verify CI passes in GitHub Actions

# 4. Create release tag
git tag v0.20260403.2230-hotfix
git push origin v0.20260403.2230-hotfix

# 5. Wait for GitHub Release to be created automatically

# 6. Go to GitHub Actions → "NPX Package Publish from GitHub Release" → "Run workflow"
#    - release_tag: v0.20260403.2230-hotfix
#    - npx_version: 0.20260403.2230-hotfix (or leave empty for auto)
#    - publish_cli: true
#    - publish_bundle: true

# 7. NPX packages are published to npm and npx-* tags are created
```
