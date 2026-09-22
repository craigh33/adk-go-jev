# Direct System One evaluation

Set your TypeSafe API key and API base URL, then run from the repository root:

```bash
export TYPESAFE_API_KEY='...'
export TYPESAFE_BASE_URL='https://api.typesafe.ai'
go run ./examples/typesafe-evaluate "I was charged twice. Please refund the duplicate payment."
```

`TYPESAFE_BASE_URL` is optional and defaults to `https://api.typesafe.ai`. Use the API root, without `/v1/systemone`.

Sends structured state and all three question types in one request. Prints answers, probabilities, confidence, model identity, and token usage. A live call uses your TypeSafe account.
