// Package d1 provides a database/sql driver for Cloudflare D1.
//
// D1 is only reachable over Cloudflare's HTTP API, so every query is a
// request/response round trip. The driver therefore does not support
// transactions: Begin returns driver.ErrSkip. Statements with several
// semicolon-separated queries are executed as one D1 batch.
package d1

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultEndpoint = "https://api.cloudflare.com/client/v4"

type Config struct {
	AccountID  string
	DatabaseID string
	APIToken   string
	Endpoint   string
	HTTPClient *http.Client
}

// NewConnector returns a driver.Connector for the given D1 database.
// Use it with sql.OpenDB.
func NewConnector(cfg Config) driver.Connector {
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultEndpoint
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &connector{cfg: cfg, client: client}
}

type connector struct {
	cfg    Config
	client *http.Client
}

func (c *connector) Connect(context.Context) (driver.Conn, error) {
	return &conn{cfg: c.cfg, client: c.client}, nil
}

func (c *connector) Driver() driver.Driver {
	return d1Driver{}
}

type d1Driver struct{}

func (d1Driver) Open(string) (driver.Conn, error) {
	return nil, errors.New("d1: create connections with sql.OpenDB(d1.NewConnector(...))")
}

type conn struct {
	cfg    Config
	client *http.Client
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return &stmt{conn: c, query: query}, nil
}

func (c *conn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	return &stmt{conn: c, query: query}, nil
}

func (c *conn) Close() error { return nil }

func (c *conn) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.run(ctx, query, args)
	if err != nil {
		return nil, err
	}

	return execResult{lastInsertID: result.lastRowID, rowsAffected: result.changes}, nil
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	result, err := c.run(ctx, query, args)
	if err != nil {
		return nil, err
	}

	return &rows{columns: result.columns, rows: result.rows}, nil
}

func (c *conn) Ping(ctx context.Context) error {
	_, err := c.run(ctx, "SELECT 1", nil)
	return err
}

type execResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r execResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r execResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type stmt struct {
	conn  *conn
	query string
}

func (s *stmt) Close() error  { return nil }
func (s *stmt) NumInput() int { return -1 }

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.ExecContext(context.Background(), s.query, namedValues(args))
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, namedValues(args))
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

type rows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *rows) Columns() []string { return r.columns }
func (r *rows) Close() error      { return nil }

func (r *rows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}

	copy(dest, r.rows[r.index])
	r.index++

	return nil
}

type result struct {
	columns   []string
	rows      [][]driver.Value
	changes   int64
	lastRowID int64
}

type queryRequest struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type apiResponse struct {
	Result  []apiResult `json:"result"`
	Success bool        `json:"success"`
	Errors  []apiError  `json:"errors"`
}

type apiResult struct {
	Results json.RawMessage `json:"results"`
	Success bool            `json:"success"`
	Meta    struct {
		Changes   int64 `json:"changes"`
		LastRowID int64 `json:"last_row_id"`
	} `json:"meta"`
}

type rawResults struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

const maxResponseSize = 32 << 20

func (c *conn) run(ctx context.Context, query string, args []driver.NamedValue) (*result, error) {
	params := make([]any, 0, len(args))
	for _, arg := range args {
		params = append(params, paramValue(arg.Value))
	}

	body, err := json.Marshal(queryRequest{SQL: query, Params: params})
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(c.cfg.Endpoint, "/")
	url := fmt.Sprintf("%s/accounts/%s/d1/database/%s/raw", endpoint, c.cfg.AccountID, c.cfg.DatabaseID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	var parsed apiResponse
	if err = json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("d1: decode response (status %d): %w", resp.StatusCode, err)
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return nil, fmt.Errorf("d1: %s (code %d)", parsed.Errors[0].Message, parsed.Errors[0].Code)
		}
		return nil, fmt.Errorf("d1: request failed with status %d", resp.StatusCode)
	}

	out := &result{}
	if len(parsed.Result) == 0 {
		return out, nil
	}

	// A batch returns one entry per statement; the last entry holds the
	// metadata for the statement that actually wrote.
	last := parsed.Result[len(parsed.Result)-1]
	out.changes = last.Meta.Changes
	out.lastRowID = last.Meta.LastRowID

	if len(last.Results) > 0 && last.Results[0] == '{' {
		var raw rawResults
		decoder := json.NewDecoder(bytes.NewReader(last.Results))
		decoder.UseNumber()
		if err = decoder.Decode(&raw); err != nil {
			return nil, err
		}

		out.columns = raw.Columns
		out.rows = make([][]driver.Value, len(raw.Rows))
		for i, row := range raw.Rows {
			values := make([]driver.Value, len(row))
			for j, value := range row {
				values[j] = normalizeValue(value)
			}
			out.rows[i] = values
		}
	}

	return out, nil
}

// paramValue normalizes Go values D1 stores differently than SQLite would. A
// bool is a valid driver.Value, but D1 persists it as the text "true"/"false",
// which later fails to scan into an integer column, so send 1/0 instead.
func paramValue(value any) any {
	if boolean, ok := value.(bool); ok {
		if boolean {
			return int64(1)
		}

		return int64(0)
	}

	return value
}

func normalizeValue(value any) driver.Value {
	switch typed := value.(type) {
	case nil, bool, string, int64, float64:
		return typed
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		if decimal, err := typed.Float64(); err == nil {
			return decimal
		}
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func namedValues(values []driver.Value) []driver.NamedValue {
	named := make([]driver.NamedValue, len(values))
	for i, value := range values {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}

	return named
}
