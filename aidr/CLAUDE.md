## Core Principles

- **KISS**: Keep It Simple, Stupid
- **DRY**: Don't Repeat Yourself
- **YAGNI**: You Ain't Gonna Need It
- **MUST** = CI enforced; **SHOULD** = strong recommendation; **CAN** = optional

**IMPORTANT**: These rules are for development guidance only. Never copy CLAUDE.md rules into Go code comments.

## Essential Rules

### Code Quality

- **CQ-1 (MUST)** Run `gofmt`, `go vet`, `golangci-lint`
- **CQ-2 (MUST)** No package name stuttering: `package kv; type Store` not `KVStore`
- **CQ-3 (MUST)** Functions >2 args use input struct (ctx stays separate)
- **CQ-4 (SHOULD)** Small, composable functions; small interfaces
- **CQ-5 (MUST)** All exported functions/types must have godot-compliant comments: start with name, complete sentences, end with periods

### Errors & Contexts

- **EC-1 (MUST)** Wrap errors: `fmt.Errorf("op %s: %w", name, err)`
- **EC-2 (MUST)** Use `errors.Is`/`errors.As`, not string matching
- **EC-3 (MUST)** Context first param; never store in structs
- **EC-4 (MUST)** Honor context cancellation/timeouts

### Concurrency & Safety

- **CS-1 (MUST)** Senders close channels, receivers don't
- **CS-2 (MUST)** Tie goroutines to context; prevent leaks
- **CS-3 (MUST)** Protect shared state with mutex/atomic

### Dependencies & Config

- **DC-1 (SHOULD)** Prefer stdlib; justify new deps
- **DC-2 (MUST)** Config via env/flags; validate on startup; immutable after init

### Testing & Observability

- **TO-1 (MUST)** Table-driven tests with `t.Cleanup`; run with `-race`
- **TO-2 (MUST)** Structured logging with `slog`
- **TO-3 (SHOULD)** Measure before optimizing

### Project Structure & Organization

- **PS-1 (MUST)** `/cmd/` contains ONLY production-deployable binaries; test tools belong in `/test/`
- **PS-2 (MUST)** Exported packages in `/pkg/` must have no dependencies on `/internal/` or `/cmd/`
- **PS-3 (SHOULD)** Follow standard Go project layout: `/cmd/`, `/internal/`, `/pkg/`, `/test/`
- **PS-4 (SHOULD)** Internal packages shared across binaries go in `/internal/`; single-binary code can stay in `/cmd/<binary>/`
- **PS-5 (CAN)** Use `/scripts/` for build, deployment, and CI scripts
- **PS-6 (CAN)** Place testdata close to tests: `test/testdata/` for integration tests, `pkg/<name>/testdata/` for unit tests

## Function Quality Checklist

1. **Readability**: Can you easily follow what it does? If yes, stop here.
2. **Complexity**: Too many if/else branches or deep nesting?
3. **Right tool**: Would data structures (trees, queues) make this cleaner?
4. **Dependencies**: Any hidden dependencies that should be parameters?
5. **Naming**: Is the name clear and consistent with the codebase?

**Remember**: Don't put CLAUDE.md rules in Go comments. Keep code comments focused on explaining business logic, not coding standards.
