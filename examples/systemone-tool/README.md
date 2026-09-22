# System One as an ADK tool

Set your TypeSafe and Gemini credentials, the TypeSafe API base URL, and an available Gemini model, then run from the repository root:

```bash
export TYPESAFE_API_KEY='...'
export TYPESAFE_BASE_URL='https://api.typesafe.ai'
export GOOGLE_API_KEY='...'
export GEMINI_MODEL='your-supported-model'
go run ./examples/systemone-tool "I was charged twice. Please refund the duplicate payment."
```

`TYPESAFE_BASE_URL` is optional and defaults to `https://api.typesafe.ai`. Use the API root, without `/v1/systemone`.

Gemini calls a System One tool with text. Questions and permitted choices are configured in Go. The tool returns the full structured result for the agent to explain. Live calls use both accounts.

The tool is independent of Gemini and can also be attached to ADK agents using the Bedrock, Ollama, or Kronk providers.
