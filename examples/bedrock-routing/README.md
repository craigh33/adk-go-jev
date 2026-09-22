# Bedrock routing

Jev classifies the ticket, then a Bedrock-backed ADK agent handles billing, technical support, or clarification. Classification runs directly without a generative model. The assessment and route are persisted under `ticket_assessment` before the selected child runs.

```bash
export TYPESAFE_API_KEY='...'
export TYPESAFE_BASE_URL='https://api.typesafe.ai'
export AWS_PROFILE='your-profile'
export AWS_REGION='your-region'
export BEDROCK_MODEL_ID='your-model-or-inference-profile'
# Optional: export TYPESAFE_MODEL='a-supported-model-id'
cd examples/bedrock-routing
go run . 'I was charged twice'
```

`TYPESAFE_BASE_URL` is optional and defaults to `https://api.typesafe.ai`. Use the API root, without `/v1/systemone`.

AWS credentials use the default SDK chain. The configured identity needs permission to invoke the selected Bedrock model. This makes live TypeSafe and Bedrock calls, which may incur charges.

The example prints the classifier assessment followed by the selected agent's response. Confidence below `0.75` selects the clarification agent. API failures stop the run. Choose a confidence threshold appropriate to your application.

This example has a separate Go module, pinned to `adk-go-bedrock v1.7.7`; its local replacement uses the current checkout of `adk-go-typesafe`. AWS dependencies do not enter the library's module. Run `make check-examples` from the repository root to compile and test it without credentials.
