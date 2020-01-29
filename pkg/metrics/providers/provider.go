package providers

type Interface interface {
	// RunQuery executes the query and converts the first result to float64
	RunQuery(query string) (float64, error)

	// IsOnline calls the provider endpoint and returns an error if the API is unreachable
	IsOnline() (bool, error)
}
