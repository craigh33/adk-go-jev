# TypeSafe API schema

`openapi.json` is a snapshot of [TypeSafe's public OpenAPI schema](https://api.typesafe.ai/openapi.json), retrieved on 2026-09-22 (API version `0.2.0`, OpenAPI `3.1.0`). TypeSafe's [Python SDK](https://github.com/typesafe-ai/typesafe-sdk-python/blob/main/src/typesafe_sdk/_schemas/models.py) uses the same generation source.

`make generate` runs pinned `oapi-codegen v2.8.0` and writes `internal/typesafe/types.gen.go`. CI regenerates from this local snapshot and rejects differences. Normal builds use the committed generated code and do not run the generator or fetch the schema.

The overlay changes Go representations only: flexible JSON content uses `any`, choice descriptions use `map[string]any`, and discriminators use strings. The generator configuration uses `float64` for API numbers. Required fields and unions remain in the upstream schema; handwritten boundary checks validate flexible content and response completeness.

To update, download the public schema to a temporary file, inspect its changes, replace `api/openapi.json`, and run `make generate test lint build`. Commit the schema and generated changes together.

This is a temporary API layer until TypeSafe publishes a supported Go SDK. Generated types stay internal; `typesafe` provides the public facade and `tools/systemone` depends only on its small `EvaluationAPI` interface. The SDK migration should replace the client boundary while retaining the ADK tool contract.
