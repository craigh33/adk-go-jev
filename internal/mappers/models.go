package mappers

// ContextProjection is the text representation supplied to Jev. Unsupported
// marks content whose full meaning could not be represented.
type ContextProjection struct {
	Text        string
	Unsupported bool
}
