# Coding Guidelines
- only one struct per file
- keep files small and focused
- document public structs, functions, and methods with comments
- use clear and descriptive names for variables, functions, and types
- avoid global variables and state
- handle errors gracefully and return them to the caller
- use interfaces to abstract away implementation details and make code more testable
- use composition over inheritance to promote code reuse and flexibility
- KISS (Keep It Simple, Stupid) - avoid unnecessary complexity and over-engineering
- YAGNI (You Ain't Gonna Need It) - don't implement features or functionality until they are actually needed
- DRY (Don't Repeat Yourself) - avoid duplicating code and logic, and instead abstract it into reusable functions or types
- use Go's built-in concurrency features (goroutines and channels) to write efficient and scalable code
- follow Go's standard formatting and style guidelines (gofmt, goimports, etc.) to ensure consistency and readability across the codebase
- use context.Context to manage cancellation and timeouts for long-running operations and to pass request-scoped values across API boundaries
- use error wrapping (fmt.Errorf with %w) to provide more context and information about errors while preserving the original error for inspection and handling by callers

# Testing Guidelines
- write unit tests for all public functions and methods
- dont write tests for private functions and methods
- fuzzytest all public functions and methods with a variety of inputs, including edge cases
- use table-driven tests to organize test cases and make them easier to read and maintain

# Structure
- every pane should have a corresponding struct that implements the Pane interface
