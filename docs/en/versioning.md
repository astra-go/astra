# Versioning Policy

Astra follows [Semantic Versioning 2.0](https://semver.org/spec/v2.0.0.html).

---

## Version Number Rules

```
v MAJOR . MINOR . PATCH
   │        │       └─ Backwards-compatible bug fixes
   │        └──────── Backwards-compatible new features
   └───────────────── Incompatible API changes
```

| Release Type | Trigger | Migration Required |
|-------------|---------|-------------------|
| `PATCH` | Bug fixes, documentation updates, internal refactoring | No |
| `MINOR` | New features, deprecation markers | Usually no |
| `MAJOR` | Removal of deprecated APIs, breaking interface changes | Yes — see migration guide |

---

## Stability Levels

Every exported symbol (function, type, method, constant) is annotated with a stability level in the documentation:

| Label | Meaning |
|-------|---------|
| **Stable** | Will never break within the current major version |
| **Beta** | May be adjusted in a minor version, but a migration path will be provided |
| **Experimental** | May change or be removed at any time; not recommended for production |

---

## Support Lifecycle

| Type | Active Maintenance | Security Fixes |
|------|--------------------|----------------|
| Current major (v1) | Ongoing | Ongoing |
| Previous minor (v1.N-1) | 3 months after new minor release | 12 months after new minor release |
| Old major (v0) | Ended | 12 months after v1.0 release |

> **Security fixes are prioritized**: CVEs found in any supported version are released as `PATCH` versions
> with a notification period of no more than 14 days.

---

## Deprecation Process

1. In a new `MINOR` version, mark with `// Deprecated: use Foo instead` in documentation and `godoc` comments
2. Deprecated symbols remain available for at least **one MINOR version** (approximately 3 months) after marking
3. Removed in the next `MAJOR` version

---

## Minimum Go Version

Astra always supports the latest two stable Go releases (currently Go 1.25 / 1.24).
Raising the minimum Go version is a **MINOR** change (with sufficient advance notice).

---

## Versioned Documentation Site

This site is deployed with multiple versions using [mike](https://github.com/jimporter/mike).
URL format:

```
https://astra-go.github.io/astra/{version}/
```

Examples:

```
https://astra-go.github.io/astra/latest/   # latest stable
https://astra-go.github.io/astra/1.0/
https://astra-go.github.io/astra/0.10/
```

### Run All Versions Locally

```bash
pip install mkdocs-material mike

# Build and deploy versions to local gh-pages branch
mike deploy --push 1.0 latest
mike deploy --push 0.10

# Preview
mike serve
# Open http://localhost:8000 in your browser
```

---

## Release Cadence

| Type | Target Cadence |
|------|---------------|
| PATCH | As needed (bug / security fixes) |
| MINOR | Quarterly (approximately every 3 months) |
| MAJOR | Only after accumulating enough breaking changes |
