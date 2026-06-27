package store

// FailedItem records one row that could not be processed in a batch operation,
// together with a human-readable reason. Batch endpoints use partial-success
// semantics: succeeded IDs are returned alongside per-ID failure reasons in the
// same HTTP 200 response.
type FailedItem struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}
