package fetcher

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type (
	SqliteFetcher struct {
		db *sql.DB
	}
)

func NewSqliteFetcher() SqliteFetcher {
	db, err := sql.Open("sqlite3", "./chinook.db")
	if err != nil {
		log.Fatal(err)
	}

	return SqliteFetcher{
		db: db,
	}
}

func (s SqliteFetcher) Select(ctx context.Context, query string) ([]string, []map[string]string, error) {
	dbRows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: error querying: %w", err)
	}
	defer dbRows.Close()

	cols, err := dbRows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: error getting columns: %w", err)
	}

	var rows []map[string]string
	for dbRows.Next() {
		rowValues := make([]any, len(cols))
		for i := range cols {
			rowValues[i] = new(sql.RawBytes)
		}

		err = dbRows.Scan(rowValues...)
		if err != nil {
			return nil, nil, fmt.Errorf("sqlite: error scanning rows: %w", err)
		}

		row := make(map[string]string)
		for i, col := range rowValues {
			colString := string(*col.(*sql.RawBytes))
			row[cols[i]] = colString
		}

		rows = append(rows, row)
	}

	return cols, rows, nil
}
