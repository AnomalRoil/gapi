# GAPI: Go APIs

The goal of GAPI is to allow you to list and compare the public APIs in your Go codebase easily.

Usage:
```
go install github.com/AnomalRoil/gapi@latest
gapi path/to/source
```

You can specify your current APIs in your `api` folder in files named `v*.txt` (e.g. `api/v1.0.1.txt api/v1.2.3.txt`),
you can specify also exceptions for features that were removed in `api/except.txt`.



## Future works

- handle architecture and OS using build.Context
- handle v2 / major bumps that are allowed to break APIs
- try and re-introduce the notion of Walker to have concurrent processing