# Direct System One evaluation

Set `TYPESAFE_API_KEY`, then run:

```bash
go run ./examples/typesafe-evaluate "I was charged twice. Please refund the duplicate payment."
```

Sends structured state and all three question types in one request. Prints answers, probabilities, confidence, model identity, and token usage. A live call uses your TypeSafe account.
