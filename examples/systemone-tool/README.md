# System One as an ADK tool

Set `TYPESAFE_API_KEY`, `GOOGLE_API_KEY`, and `GEMINI_MODEL` to an available Gemini model, then run:

```bash
go run ./examples/systemone-tool "I was charged twice. Please refund the duplicate payment."
```

Gemini calls a System One tool with text. Questions and permitted choices are configured in Go. The tool returns the full structured result for the agent to explain. Live calls use both accounts.

The tool is independent of Gemini and can also be attached to ADK agents using the Bedrock, Ollama, or Kronk providers.
