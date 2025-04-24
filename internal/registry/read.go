package registry

import (
	"fmt"
	"io"
	"net/http"
)

// maxBody bounds what this package reads from a registry. A tag listing and a
// token are both small, and a bound keeps a wrong answer from filling memory.
const maxBody = 32 << 20

func readAll(response *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read the answer of the registry: %w", err)
	}

	return body, nil
}
