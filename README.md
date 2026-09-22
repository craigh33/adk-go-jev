<p align="center">
  <img
    src="docs/images/readme-header.png"
    alt="adk-go-typesafe banner showing Agent Development Kit connected to TypeSafe AI"
    width="100%"
  />
</p>

# adk-go-typesafe

[TypeSafe AI](https://typesafe.ai/) System One integration for [adk-go](https://github.com/google/adk-go), bringing Choice, Score, and Noul primitives to Go agents and workflows with models such as [Jev](https://typesafe.ai/blog/introducing-system-one-models-and-jev).

Provides a typed HTTP client and an ADK function tool. The client is a temporary bridge until TypeSafe publishes a Go SDK; its API types are generated from TypeSafe's OpenAPI schema.

**Other providers:** [adk-go-bedrock](https://github.com/craigh33/adk-go-bedrock) · [adk-go-ollama](https://github.com/craigh33/adk-go-ollama) · [adk-go-kronk](https://github.com/craigh33/adk-go-kronk)

## Requirements

- **Go**: match [`go.mod`](go.mod).
- Live examples will require a **TypeSafe API key** (`TYPESAFE_API_KEY`); see the [TypeSafe quick start](https://docs.typesafe.ai/introduction/quickstart).

## Install

```bash
go get github.com/craigh33/adk-go-typesafe
```

## Usage

```go
import "github.com/craigh33/adk-go-typesafe/typesafe"

client, err := typesafe.New(nil) // Reads TYPESAFE_API_KEY.
if err != nil {
    return err
}

response, err := client.Evaluate(ctx, &typesafe.Request{
    State: "I was charged twice. Please refund the duplicate payment.",
    Questions: map[string]typesafe.Question{
        "department": typesafe.Choice{
            Instructions: "Which team should handle this?",
            Criteria: map[string]any{
                "billing":   "Payments and refunds",
                "technical": "Bugs and integrations",
            },
        },
        "urgent": typesafe.Noul{Instructions: "Does this require immediate attention?"},
    },
})
if err != nil {
    return err
}

department := response.Answers["department"].(typesafe.ChoiceAnswer)
fmt.Println(department.Choice, department.Confidence, department.Probabilities)
```

`Choice` selects a named option, `Score` evaluates ordered levels, and `Noul` returns the probability of yes. Responses retain probabilities, confidence, score legends, model identity, and token usage. State accepts text, JSON objects, or arrays; question instructions and criteria can also contain structured JSON.

The default model is `jev-latest`. Set `Options.Model` for a client default or `Request.Model` for a single evaluation. `Options` also accepts an API key, base URL, and HTTP client. The default timeout is ten seconds; context cancellation is preserved. Calls are not retried automatically: use `errors.As` with `*typesafe.APIError` to inspect `StatusCode` and `RetryAfter` when applying your retry policy.

## ADK tool

```go
import "github.com/craigh33/adk-go-typesafe/tools/systemone"

evaluate, err := systemone.New(systemone.Config{
    API: client,
    Questions: map[string]typesafe.Question{
        "urgent": typesafe.Noul{Instructions: "Does this require immediate attention?"},
    },
})
if err != nil {
    return err
}
// Add evaluate to llmagent.Config.Tools.
```

The application fixes the questions and rubrics; the calling agent supplies only text in `state`. ADK generates the tool's input schema from its Go input type. The tool returns the full structured evaluation and works with ADK models that support function tools, including your Bedrock, Ollama, or Kronk setup. For structured state, use the client directly.

The tool depends on a small `EvaluationAPI` interface so the HTTP implementation can later be replaced by an adapter for the official SDK. No ADK core changes or `model.LLM` implementation are required.

## Examples

- [`examples/typesafe-evaluate`](examples/typesafe-evaluate): text and structured state with all three question types.
- [`examples/systemone-tool`](examples/systemone-tool): a Gemini-backed ADK agent calling an application-configured tool.

## Development

```bash
git clone https://github.com/craigh33/adk-go-typesafe.git
cd adk-go-typesafe
git switch -c feat/your-change
make pre-commit-install
make check-generated test lint build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development tools and contribution guidelines.

`make generate` uses a pinned generator and the checked-in [OpenAPI snapshot](api). CI checks that generated types are current. Generation tools are not required by library consumers.

## Repository layout

- [`typesafe`](typesafe): TypeSafe API client.
- [`tools/systemone`](tools/systemone): ADK tools for System One evaluations.
- [`internal/mappers`](internal/mappers): request and response conversions.
- [`internal/typesafe`](internal/typesafe): generated API wire types.
- [`api`](api): upstream OpenAPI snapshot and generation configuration.
- [`examples`](examples): runnable direct-client and ADK examples.

## Kudos

- [TypeSafe AI](https://typesafe.ai/) for Jev and the [System One API](https://docs.typesafe.ai/).
- [Google ADK](https://github.com/google/adk-go) for the Agent Development Kit for Go.
- [adk-go-bedrock](https://github.com/craigh33/adk-go-bedrock) for the repository structure, tooling, and contribution conventions.

This is an independent community integration, not an official TypeSafe AI or Google library.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for Makefile targets, required pre-commit setup, commit message conventions, and pull request guidelines. For new issues, use the [bug report](https://github.com/craigh33/adk-go-typesafe/issues/new?template=bug_report.yml) or [feature request](https://github.com/craigh33/adk-go-typesafe/issues/new?template=feature_request.yml) templates.

## License

Apache 2.0 — see [LICENSE](LICENSE).

[Contributing](CONTRIBUTING.md) · [Issues](https://github.com/craigh33/adk-go-typesafe/issues) · [Security](https://github.com/craigh33/adk-go-typesafe/security)
