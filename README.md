<p align="center">
  <img
    src="docs/images/readme-header.png"
    alt="adk-go-typesafe banner showing Agent Development Kit connected to TypeSafe AI"
    width="100%"
  />
</p>

# adk-go-typesafe

[TypeSafe AI](https://typesafe.ai/) System One integration for [adk-go](https://github.com/google/adk-go), bringing Choice, Score, and Noul primitives to Go agents and workflows with models such as [Jev](https://typesafe.ai/blog/introducing-system-one-models-and-jev).

**Status:** Repository bootstrap only. The TypeSafe client, ADK integration, and runnable examples are not implemented yet.

**Other providers:** [adk-go-bedrock](https://github.com/craigh33/adk-go-bedrock) · [adk-go-ollama](https://github.com/craigh33/adk-go-ollama) · [adk-go-kronk](https://github.com/craigh33/adk-go-kronk)

## Requirements

- **Go**: match [`go.mod`](go.mod).
- Live examples will require a **TypeSafe API key** (`TYPESAFE_API_KEY`); see the [TypeSafe quick start](https://docs.typesafe.ai/introduction/quickstart).

## Development

```bash
git clone https://github.com/craigh33/adk-go-typesafe.git
cd adk-go-typesafe
git switch -c feat/your-change
make pre-commit-install
make test lint build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development tools and contribution guidelines.

## Repository layout

- [`typesafe`](typesafe): TypeSafe API client.
- [`tools/systemone`](tools/systemone): ADK tools for System One evaluations.
- [`internal/mappers`](internal/mappers): request and response conversions.
- [`examples`](examples): runnable examples as the integration is implemented.

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
