# Changes in this fork

Baseline: [grant/go-diagram](https://github.com/grant/go-diagram) (last updated 2018), which no longer built on current Go or Node.

## Bugs fixed

- **Data loss on save (critical).** Editing any struct in a file without imports duplicated the first struct and deleted the file's first function and every non-struct type declaration (e.g. `type ID string`). The bundled `example1/demo.go` had already been corrupted this way (`Edge` declared three times). Write-back now replaces only struct declarations, in place, and a regression test covers it.
- **Struct tags and generics dropped on save.** Rewriting a file removed `json:"..."` tags and type parameters (`Cache[T any]` became `Cache`). Both now round-trip.
- **Crash on modern Go.** The parser panicked on any type expression it didn't recognise, including generics (`List[T]`), variadics, anonymous structs, and `pkg.Type` selectors on non-identifiers. It now handles them all and never panics.
- **Directories silently skipped.** Any path containing the letters "app" (e.g. `application/`, `mapper/`) was ignored. Now only `app`, `vendor`, `node_modules`, `testdata`, and hidden directories are skipped.
- **One bad file hid the whole project.** A syntax error anywhere caused a panic. Parse errors are now reported to the browser while the rest of the project still renders.
- **Server races and crashes.** Global state was shared across connections without locking; a nil map lookup could crash the server on an unknown package; the `lastMod` query parse was inverted. Replaced with a per-project mutex, per-connection sessions, and explicit errors. `go test -race` is clean.
- **New or deleted files not detected.** Change detection only looked at modification times of already-known files. It now fingerprints all `.go` files, so additions and removals appear too.
- **Frontend memory leak.** `UMLDiagram` started a new `setInterval` on every render, so timers piled up while panning. Edge positions are now computed once after each render and on resize.
- **Frontend state mutation.** The reducer shallow-copied state and then mutated nested objects, and it sent websocket messages from inside the reducer. Replaced with Redux Toolkit (Immer) reducers and a sync middleware.
- **Stale edits.** Struct inputs copied props into state once, so server updates never reached an open diagram. Inputs now reset when the underlying value changes.
- **Hard-coded `ws://localhost:8080`** replaced with the page's own host, so custom ports, Docker, and the Vite dev proxy work.

## Modernisation

- Go modules (`go.mod`), Go 1.24; removed deprecated `ioutil`.
- Frontend moved from Webpack 1 / Babel 6 / React 0.14 / react-router 1 to **Vite 8 / React 19 / Redux Toolkit 2**; removed jQuery, underscore, and the unmaintained `react-input-autosize` (replaced with a small component).
- Server binds to `127.0.0.1` by default (it writes files), with `-addr`, `-static`, `-read-only`, and `-no-browser` flags and a `/healthz` endpoint.

## New features

- Edges start at the specific field that holds the reference, and are drawn behind structs.
- Zoom (ctrl/⌘ + scroll or buttons) and scroll-to-pan.
- Error banner instead of `alert()`; after a rejected edit the diagram reverts to what is on disk.
- Connection status and automatic reconnect.
- Generic type parameters shown in struct headers; struct tags shown on hover; embedded fields labelled.
- Input validation for struct and field names before anything touches disk.

## Tooling

- Go tests for parsing, generics, directory filtering, error handling, write-back round-trips, and a websocket integration test.
- Frontend unit tests (Vitest) for the reducer, the sync middleware, and edge geometry.
- Multi-stage `Dockerfile` (distroless runtime) and GitHub Actions CI (gofmt, vet, race tests, frontend tests and build, Docker build).
