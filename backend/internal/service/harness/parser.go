package harness

// LineParser converts one provider-native JSONL/stdout line into zero or more
// normalized agent events.
type LineParser interface {
	ParseLine(line []byte) ([]Event, error)
}
