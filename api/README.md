# TypeSafe API generation

Generate directly from [TypeSafe's public OpenAPI definition](https://api.typesafe.ai/openapi.json). TypeSafe's [Python SDK](https://github.com/typesafe-ai/typesafe-sdk-python/blob/main/src/typesafe_sdk/_schemas/models.py) uses the same generation source. The schema is not stored in this repository.

`make generate` runs pinned `oapi-codegen v2.8.0`, loads the live definition, and writes `internal/typesafe/types.gen.go`. `make check-generated` does the same and rejects differences. CI therefore detects upstream changes that alter generated code. Both targets require network access; normal builds use the committed generated code without fetching the schema or running the generator.

The overlay changes Go representations only: flexible JSON content uses `any`, choice descriptions use `map[string]any`, and discriminators use strings. The generator configuration uses `float64` for API numbers. Required fields and unions remain in the upstream schema; handwritten boundary checks validate flexible content and response completeness.

To update, run `make generate`, review the generated diff, then run `make test lint build`. Commit the generated changes and any corresponding client updates together.

This is a temporary API layer until TypeSafe publishes a supported Go SDK. Generated types stay internal; `typesafe` provides the public facade and `tools/systemone` depends only on its small `EvaluationAPI` interface. The SDK migration should replace the client boundary while retaining the ADK tool contract.
