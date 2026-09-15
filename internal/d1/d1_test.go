package d1

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestDB(t *testing.T, handler http.HandlerFunc) *sql.DB {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	db := sql.OpenDB(NewConnector(Config{
		AccountID:  "account",
		DatabaseID: "database",
		APIToken:   "token",
		Endpoint:   server.URL,
	}))
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	return db
}

func rawResponse(body string) string {
	return fmt.Sprintf(`{"result":[%s],"success":true,"errors":[],"messages":[]}`, body)
}

func TestQueryReturnsRowsInColumnOrder(t *testing.T) {
	db := newTestDB(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/accounts/account/d1/database/database/raw" {
			t.Errorf("unexpected path %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("unexpected authorization %q", got)
		}

		fmt.Fprint(w, rawResponse(`{
			"results": {"columns": ["id", "BrewDate", "GrindSetting"], "rows": [[7, "2026-05-20", 4.2]]},
			"success": true,
			"meta": {"changes": 0, "last_row_id": 0, "rows_read": 1, "rows_written": 0}
		}`))
	})

	var (
		id           int
		brewDate     string
		grindSetting float64
	)
	if err := db.QueryRow("SELECT id, BrewDate, GrindSetting FROM CoffeeEntries").Scan(&id, &brewDate, &grindSetting); err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Fatalf("expected id 7, got %d", id)
	}
	if brewDate != "2026-05-20" {
		t.Fatalf("unexpected brew date %q", brewDate)
	}
	if grindSetting != 4.2 {
		t.Fatalf("unexpected grind setting %v", grindSetting)
	}
}

func TestQueryWithNoRowsReturnsErrNoRows(t *testing.T) {
	db := newTestDB(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, rawResponse(`{
			"results": {"columns": ["id", "Roaster"], "rows": []},
			"success": true,
			"meta": {"changes": 0, "last_row_id": 0, "rows_read": 0, "rows_written": 0}
		}`))
	})

	var id int
	var roaster string
	err := db.QueryRow("SELECT id, Roaster FROM CoffeeRoasters WHERE id = ?", "missing").Scan(&id, &roaster)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestExecReturnsLastInsertIDAndChanges(t *testing.T) {
	db := newTestDB(t, func(w http.ResponseWriter, r *http.Request) {
		var request queryRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(request.Params) != 2 {
			t.Errorf("expected 2 params, got %v", request.Params)
		}

		fmt.Fprint(w, rawResponse(`{
			"results": {"columns": [], "rows": []},
			"success": true,
			"meta": {"changes": 1, "last_row_id": 42, "rows_read": 0, "rows_written": 1}
		}`))
	})

	result, err := db.Exec("INSERT INTO CoffeeRoasters (id, Roaster) VALUES (?, ?)", "shoebox", "Shoebox")
	if err != nil {
		t.Fatal(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("expected last insert id 42, got %d", id)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 row affected, got %d", affected)
	}
}

func TestExecSurfacesD1Errors(t *testing.T) {
	db := newTestDB(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"result":null,"success":false,"errors":[{"code":7500,"message":"UNIQUE constraint failed"}],"messages":[]}`)
	})

	if _, err := db.Exec("INSERT INTO CoffeeRoasters (id, Roaster) VALUES (?, ?)", "shoebox", "Shoebox"); err == nil {
		t.Fatal("expected an error from the D1 API")
	}
}

func TestMultiStatementExecUsesLastResultMeta(t *testing.T) {
	db := newTestDB(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":[
			{"results":{"columns":[],"rows":[]},"success":true,"meta":{"changes":0,"last_row_id":0}},
			{"results":{"columns":[],"rows":[]},"success":true,"meta":{"changes":3,"last_row_id":0}}
		],"success":true,"errors":[],"messages":[]}`)
	})

	result, err := db.Exec("CREATE TABLE a (id INTEGER); INSERT INTO a (id) VALUES (1);")
	if err != nil {
		t.Fatal(err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if affected != 3 {
		t.Fatalf("expected 3 rows affected from the last statement, got %d", affected)
	}
}
