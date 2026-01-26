# Homebrew Formula

This directory contains the Homebrew formula for installing Tingly-Box.

## Files

- `tingly-box.rb` - Homebrew formula (auto-generated on releases)
- `generate.rb` - Formula generation script (used by CI/CD)

## Installation

```bash
# Tap the repository
brew tap tingly-dev/tingly-box

# Install
brew install tingly-box

# Run
tingly-box start
```

## Development

Generate formula locally:
```bash
ruby Formula/generate.rb VERSION=v1.6.1 SHA256_DARWIN_ARM64=xxx SHA256_DARWIN_AMD64=yyy SHA256_LINUX_AMD64=zzz
```
