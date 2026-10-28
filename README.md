# GAPI: Go APIs

GAPI lists the public API of your Go module and checks it against the API you declared,
so that every API change is visible and deliberate. It reuses the approach (and the file format)
of the Go project's own `cmd/api` checker, which guards the Go 1 compatibility promise.

```
go install github.com/AnomalRoil/gapi@latest
gapi -print . > api/v1.0.0.txt   # once: record your current API
gapi .                           # in CI: fail on any undeclared change
```

## How it works

`gapi` type-checks every package of the module (`./...`), skipping `main` packages and,
unless `-internal` is set, `internal` packages. Each exported identifier becomes one
feature line, for example:

```
pkg example.com/mod/p, func NewClient(string, ...Option) (*Client, error)
pkg example.com/mod/p, method (*Client) Close() error
pkg example.com/mod/p, type Client struct, Name string
```

It then compares these features with the files in the `api` folder:

| File               | Content                                                       |
|--------------------|---------------------------------------------------------------|
| `api/v*.txt`       | API of released versions, e.g. `api/v1.2.0.txt`               |
| `api/next/*.txt`   | New APIs accepted for the upcoming release                    |
| `api/except.txt`   | Features removed or changed on purpose                        |

The check fails when:
- a declared feature is gone and not listed in `api/except.txt`: printed as `-feature` (breaking change),
- a feature is not declared anywhere: printed as `+feature` (undeclared addition),
- an API file is malformed (blank line, CRLF, missing final newline).

Lines starting with `#` are comments.

## Workflow

1. Add a new API: run `gapi -next api/next/my-feature.txt .` to declare it, and commit that file
   with your change. Reviewers see the API change in the diff.
2. Remove or change an API on purpose: add the old `-` lines to `api/except.txt`.
3. Release `vX.Y.Z`: move the pending APIs into a version file.

```
cat api/next/*.txt | sort > api/vX.Y.Z.txt && rm api/next/*.txt
```

## Options

```
gapi [OPTION]... [SOURCE]      # SOURCE defaults to "."
  -print        print the current API instead of checking it
  -next FILE    append undeclared new APIs to FILE instead of failing on them
  -internal     include internal packages
  -verbose      print loaded files, packages and features
```

## GitHub Action

```yaml
name: "Check Public APIs"
on:
  push:
    branches:
      - main
  pull_request:

jobs:
  api:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: 'stable'
      - name: Check breakage in public APIs
        run: go run github.com/AnomalRoil/gapi@latest .
```

Use a recent `gapi`: it must be built with a `golang.org/x/tools` version that can read the
export data of your Go toolchain. If loading fails, `gapi` stops instead of reporting bogus differences.

## Differences with Go's `cmd/api`

- Type constraints are recorded (`type Number interface { ~float64 | ~int }`),
  since narrowing a constraint breaks callers. Go's tool records `interface {}`.
- No per-OS/arch contexts: the API is computed for the current `GOOS`/`GOARCH` and build tags.

## Future works

- handle architecture and OS using build.Context
- handle v2 / major bumps that are allowed to break APIs
