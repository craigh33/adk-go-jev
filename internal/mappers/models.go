package mappers

// Projection contains rendered content and records whether any data was omitted.
type Projection struct {
	Text       string
	Incomplete bool
}
