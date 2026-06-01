package githubapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// graphqlRequest is the POST payload shape.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlError is one entry in a GraphQL "errors" array.
type graphqlError struct {
	Message string `json:"message"`
}

// graphql POSTs a query and decodes the "data" field into target. It surfaces
// any GraphQL errors and updates the Budget from a rateLimit block if the
// decoded data contains one (target may embed `RateLimit` under "rateLimit").
func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, target any) error {
	payload, err := json.Marshal(graphqlRequest{Query: query, Variables: vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql: status %d", resp.StatusCode)
	}

	// Decode errors and raw data first, then unmarshal data into target.
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []graphqlError  `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, len(envelope.Errors))
		for i, e := range envelope.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("graphql: empty data")
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return err
	}

	// Opportunistically update the budget if the data carries a rateLimit block.
	var rl struct {
		RateLimit RateLimit `json:"rateLimit"`
	}
	if err := json.Unmarshal(envelope.Data, &rl); err == nil && rl.RateLimit.ResetAt != "" {
		c.Budget.UpdateFromGraphQL(rl.RateLimit)
	}
	return nil
}
