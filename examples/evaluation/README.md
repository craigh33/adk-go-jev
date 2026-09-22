# Routing evaluation

Run labelled Choice cases against TypeSafe and compare models or thresholds:

```bash
export TYPESAFE_API_KEY='...'
export TYPESAFE_BASE_URL='https://api.typesafe.ai'
go run ./cmd/typesafe-eval -dataset examples/evaluation/tickets.json > report.json
go run ./cmd/typesafe-eval -dataset examples/evaluation/tickets.json -min-confidence 0.85 > stricter-report.json
```

`TYPESAFE_BASE_URL` is optional and defaults to `https://api.typesafe.ai`. Use the API root, without `/v1/systemone`.

Use `-model` to select a supported TypeSafe model and `-timeout` to change the five-minute total deadline. These commands make billable live API calls; retries are disabled by default. The three sample cases are illustrative, not a validated accuracy baseline.

Each case supplies text or structured `state` and either `expected_choice` or `expected_fallback: true`. The harness uses the same inclusive confidence threshold as the ADK router; it assumes every rubric choice has a route. Unknown fields and invalid cases are rejected before any API call. If your router deliberately leaves a choice unmapped, model that fallback in a dedicated dataset or evaluate its policy separately.

Reports include per-case results, resolved model IDs, token usage, and:

| Metric | Meaning |
|--------|---------|
| `accuracy` | Correct choices and expected fallbacks divided by all processed cases; errors count as incorrect |
| `selected_accuracy` | Correct choices among cases that passed the confidence threshold |
| `fallback_rate` | Fallbacks divided by all processed cases |
| `coverage` | Confident selections divided by all processed cases |
| `p50_latency_ms`, `p95_latency_ms` | Nearest-rank request latency percentiles, including failed calls |

Per-case failures are recorded and later cases still run. Any failed call gives the CLI a non-zero exit status; inaccurate predictions alone do not. Cancellation writes the partial report and exits with an error. Reports omit submitted state, though case names and labels remain present. Reported tokens do not include usage omitted by failed calls or any retries performed by a custom client.

Library callers can use `evaluation.Load` and `evaluation.Run` with any `typesafe.Evaluator`. Use a representative, held-out dataset to choose a threshold and inspect errors before adopting it in routing or callbacks.
