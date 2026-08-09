package parms

// SearchParameters contains search values that may be overridden at runtime.
type SearchParameters struct {
	Contempt int
}

// Search contains the default search parameters.
var Search = SearchParameters{
	Contempt: 5,
}
