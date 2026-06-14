package eddsdk

import "fmt"

// APIError is a non-2xx HTTP response from an edd-cloud service.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("edd-cloud API error %d: %s", e.Status, e.Message)
}
