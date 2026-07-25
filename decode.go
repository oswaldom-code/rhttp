package rhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// DecodeJSON decodes a JSON response body into v and always closes the body,
// draining any remainder so the connection can be reused.
//
// It is opinionated: a status code >= 300 is an error and the body is not
// decoded. Callers that need the payload of error responses should read
// resp.Body directly instead.
func DecodeJSON(resp *http.Response, v any) error {
	defer drainAndClose(resp)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("rhttp: unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("rhttp: decode json: %w", err)
	}
	return nil
}
