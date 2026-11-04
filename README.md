# GAPI: Go APIs

The goal of GAPI is to allow you to list and compare the public APIs in your Go codebase easily.
It is based on the way the Go project itself [does it](https://github.com/golang/go/blob/e30e65f7a8bda0351d9def5a6bc91471bddafd3d/src/cmd/api/api_test.go) to ensure it has a stable API.

Usage:
```
go install github.com/AnomalRoil/gapi@latest
gapi path/to/source
```

You can specify your current APIs in your `api` folder in files named `v*.txt` (e.g. `api/v1.0.1.txt api/v1.2.3.txt`),
you can specify also exceptions for features that were removed in `api/except.txt`.

If you want to specify another "base directory", e.g. `.github/api`, you can do it by specifying the `GAPI_FOLDER` env variable, otherwise it'll default to the specified `path/to/source/api` folder.

## Github Action

You can easily use it as a Github Action:
```
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
      - name: Checkout
        uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Check breakage in public APIs
        env:
          GAPI_FOLDER=".github/api"
        run: |
          go install github.com/AnomalRoil/gapi@latest
          gapi .
```

## Future works

- handle architecture and OS using build.Context
- handle v2 / major bumps that are allowed to break APIs
- try and re-introduce the notion of Walker to have concurrent processing
