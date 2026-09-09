package chartworks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

// CallOptions carries request coordinates, never identity or executable authority.
// Body is JSON or raw upload bytes according to the registered operation. Query
// accepts only registered single-valued parameters. Attempts defaults to one and
// is bounded at three; only declared reads or owner-classified keyed operations
// can opt into replay. Transport errors, cancellation, authorization, conflicts
// and expired/missing objects are never automatically retried.
type CallOptions struct {
	ResourceID     string
	Query          url.Values
	Body           []byte
	IdempotencyKey string
	Attempts       int
}

// CallResult preserves the exact bounded wire body, including lossless numbers.
// JSON remains JSON bytes; text metrics are not wrapped in a fabricated object.
type CallResult struct {
	ContentType string
	Body        []byte
}

// Invoke calls only an operation in the running server's generated registry.
// It uses the same authenticated transport as typed SDK methods and forwards no
// arbitrary URL or user-selected authorization header. Returned catalog rows
// cannot be used to edit this binding. Future domains appear only when registered.
func (c *Client) Invoke(ctx context.Context, operationID string, in CallOptions) (CallResult, error) {
	if !wireID(operationID) || in.Attempts < 0 || in.Attempts > 3 {
		return CallResult{}, ErrInvalidCall
	}
	ctx, cancel, err := c.clientContext(ctx)
	if err != nil {
		return CallResult{}, err
	}
	defer cancel()
	rows, err := c.Operations(ctx)
	if err != nil {
		return CallResult{}, err
	}
	var row OperationInfo
	for _, candidate := range rows {
		if candidate.ID == operationID {
			row = candidate
			break
		}
	}
	if row.ID == "" {
		return CallResult{}, ErrUnknownOperation
	}
	if row.Audience != "http" {
		return CallResult{}, ErrInvalidCall
	}
	attempts := in.Attempts
	if attempts == 0 {
		attempts = 1
	}
	if attempts > 1 && (row.Replay == "never" || row.Replay == "keyed" && in.IdempotencyKey == "") {
		return CallResult{}, ErrUnsafeRetry
	}
	path, body, err := prepareCall(row, in)
	if err != nil {
		return CallResult{}, err
	}
	var outputSchema *gateway.Schema
	if row.Method != http.MethodHead && row.ResponseContentType == "application/json" {
		outputSchema, err = gateway.NewSchema("operationOutput", row.ResponseSchema)
		if err != nil {
			return CallResult{}, ErrInvalidCatalog
		}
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return CallResult{}, err
		}
		var output string
		err = c.callReader(ctx, row.Method, path, in.IdempotencyKey, row.RequestContentType, bytes.NewReader(body), &output, 32<<20)
		if err == nil {
			result := CallResult{ContentType: row.ResponseContentType, Body: []byte(output)}
			if outputSchema != nil && outputSchema.ValidateResponse(result.Body, 32<<20) != nil {
				return CallResult{}, errors.New("chartworks: invalid operation response")
			}
			return result, nil
		}
		var status *StatusError
		if attempt+1 == attempts || !errors.As(err, &status) || !retryStatus(status.Status) {
			return CallResult{}, err
		}
		// The exact body, path and logical key are frozen before the first
		// attempt. Only the caller's current token is reacquired on replay.
		timer := time.NewTimer((100 * time.Millisecond) << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return CallResult{}, ctx.Err()
		case <-timer.C:
		}
	}
	return CallResult{}, err
}

func retryStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func prepareCall(row OperationInfo, in CallOptions) (string, []byte, error) {
	if len(in.Body) > row.MaxBodyBytes || len(in.IdempotencyKey) > 128 || in.IdempotencyKey != "" && !wireID(in.IdempotencyKey) {
		return "", nil, ErrInvalidCall
	}
	path := row.Path
	if strings.Contains(path, "{id}") {
		if !wireID(in.ResourceID) || in.ResourceID == "." || in.ResourceID == ".." {
			return "", nil, ErrInvalidCall
		}
		path = strings.Replace(path, "{id}", in.ResourceID, 1)
	} else if in.ResourceID != "" {
		return "", nil, ErrInvalidCall
	}
	query := make(url.Values)
	knownQuery := map[string]bool{}
	keyAllowed := false
	for _, parameter := range row.Parameters {
		var value string
		var supplied bool
		switch parameter.In {
		case "path":
			value, supplied = in.ResourceID, in.ResourceID != ""
		case "query":
			knownQuery[parameter.Name] = true
			values, exists := in.Query[parameter.Name]
			if exists && len(values) != 1 {
				return "", nil, ErrInvalidCall
			}
			if exists {
				value, supplied = values[0], true
				query.Set(parameter.Name, value)
			}
		case "header":
			if !strings.EqualFold(parameter.Name, "Idempotency-Key") {
				// New mandatory header contracts require an intentional client
				// extension; they never turn into arbitrary credential forwarding.
				if parameter.Required {
					return "", nil, ErrInvalidCall
				}
				continue
			}
			keyAllowed = true
			value, supplied = in.IdempotencyKey, in.IdempotencyKey != ""
		}
		if parameter.Required && !supplied {
			return "", nil, ErrInvalidCall
		}
		if supplied && !parameterValue(parameter, value) {
			return "", nil, ErrInvalidCall
		}
	}
	for name := range in.Query {
		if !knownQuery[name] {
			return "", nil, ErrInvalidCall
		}
	}
	if in.IdempotencyKey != "" && !keyAllowed {
		return "", nil, ErrInvalidCall
	}
	encoded := query.Encode()
	if len(encoded) > 64<<10 {
		return "", nil, ErrInvalidCall
	}
	if encoded != "" {
		path += "?" + encoded
	}
	body := append([]byte(nil), in.Body...)
	if row.RequestContentType == "application/json" {
		if len(body) == 0 {
			body = []byte("{}")
		}
		schema, err := gateway.NewSchema("operationInput", row.RequestSchema)
		if err != nil {
			return "", nil, ErrInvalidCatalog
		}
		if schema.Validate(body, row.MaxBodyBytes) != nil {
			return "", nil, ErrInvalidCall
		}
	}
	return path, body, nil
}

func parameterValue(parameter OperationParameter, value string) bool {
	if len(value) > 16<<10 {
		return false
	}
	var shape struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(parameter.Schema, &shape) != nil {
		return false
	}
	var data []byte
	switch shape.Type {
	case "string":
		data, _ = json.Marshal(value)
	case "integer":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return false
		}
		data = []byte(value)
	case "number", "boolean":
		data = []byte(value)
	default:
		return false
	}
	schema, err := gateway.NewSchema("operationParameter", parameter.Schema)
	return err == nil && schema.Validate(data, (16<<10)+2) == nil
}
