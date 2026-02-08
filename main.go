package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type DdevDescribe struct {
	Raw struct {
		DBInfo struct {
			PublishedPort int    `json:"published_port"`
			Username      string `json:"username"`
			Password      string `json:"password"`
			DBName        string `json:"dbname"`
		} `json:"dbinfo"`
	} `json:"raw"`
}

type TableInfo struct {
	Name    string
	Columns []string
}

type SearchResult struct {
	TableName    string
	DisplayQuery string
	Rows         [][]interface{}
	Columns      []string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: mage-fts <search-term> [options]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintln(os.Stderr, "  --limit=N\t\tMax results per table (default: 20)")//TODO: Implement
		fmt.Fprintln(os.Stderr, "  --match=text\t\tOnly search text columns")//TODO: Implement
		fmt.Fprintln(os.Stderr, "  --tables=PATTERN\tOnly search matching tables")//TODO: Implement
		fmt.Fprintln(os.Stderr, "  --exclude=PATTERN\tExclude matching tables")//TODO: Implement
		fmt.Fprintln(os.Stderr, "  --dry-run\t\tShow queries without executing")//TODO: Implement
		os.Exit(1)
	}

	// Check if we're in a Magento root
	envFile := "app/etc/env.php"
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Error: app/etc/env.php not found")
		os.Exit(1)
	}

	searchTerm := os.Args[1]
	fmt.Fprintf(os.Stderr, "Searching for: %s\n", searchTerm)

	// Get database connection info from DDEV
	db, dbName, err := connectDdev()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Fprintln(os.Stderr, "Connected to database successfully")

	// Get tables and their columns
	tables, err := getTableColumns(db, dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting tables: %v\n", err)
		os.Exit(1)
	}


	fmt.Fprintf(os.Stderr, "Found %d tables\n", len(tables))

	// Collect results
	var allResults []SearchResult

	// Generate and execute queries for each table
	for _, table := range tables {
		if len(table.Columns) == 0 {
			continue
		}

		result, err := searchTable(db, dbName, table.Name, table.Columns, searchTerm)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error searching table %s: %v\n", table.Name, err)
			continue
		}

		if len(result.Rows) > 0 {
			allResults = append(allResults, result)
		}
	}

	// Display results
	for _, result := range allResults {
		fmt.Printf("Table: %s - Query:\n", result.TableName)
		fmt.Printf("%s\n", result.DisplayQuery)
		for _, row := range result.Rows {
			// Display first column and test column TODO: change
			firstCol := formatValue(row[0])
			sampleCol := ""
			if len(row) > 1 {
				sampleCol = formatValue(row[1])
			}
			fmt.Printf("%s, \"%s\"\n", firstCol, sampleCol)
		}
		fmt.Println()
	}
}

func connectDdev() (*sql.DB, string, error) {
	cmd := exec.Command("ddev", "describe", "-j")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to run DDEV describe: %w", err)
	}

	var desc DdevDescribe
	if err := json.Unmarshal(output, &desc); err != nil {
		return nil, "", fmt.Errorf("failed to parse DDEV output: %w", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s",
		desc.Raw.DBInfo.Username,
		desc.Raw.DBInfo.Password,
		desc.Raw.DBInfo.PublishedPort,
		desc.Raw.DBInfo.DBName,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, "", fmt.Errorf("failed to ping database: %w", err)
	}

	return db, desc.Raw.DBInfo.DBName, nil
}

func getTableColumns(db *sql.DB, dbName string) ([]TableInfo, error) {
	query := `
		SELECT TABLE_NAME, COLUMN_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, ORDINAL_POSITION`

	rows, err := db.Query(query, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	var currentTable *TableInfo

	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return nil, err
		}

		// If on new table, append the previous one and start a new one
		if currentTable == nil || currentTable.Name != tableName {
			if currentTable != nil {
				tables = append(tables, *currentTable)
			}
			currentTable = &TableInfo{
				Name:    tableName,
				Columns: []string{},
			}
		}

		currentTable.Columns = append(currentTable.Columns, columnName)
	}

	// Append last table
	if currentTable != nil {
		tables = append(tables, *currentTable)
	}

	return tables, rows.Err()
}

func searchTable(db *sql.DB, dbName, tableName string, columns []string, searchTerm string) (SearchResult, error) {
	// Build WHERE clause with OR conditions for all columns
	whereConditions := []string{}
	args := []interface{}{}

	for _, column := range columns {
		whereConditions = append(whereConditions, fmt.Sprintf("`%s` LIKE ?", column))
		args = append(args, "%"+searchTerm+"%")
	}

	// Build full query
	query := fmt.Sprintf("SELECT t.* FROM %s.%s t WHERE %s LIMIT 20;",
		dbName,
		tableName,
		strings.Join(whereConditions, " OR "))

	result := SearchResult{
		TableName:    tableName,
		DisplayQuery: buildDisplayQuery(query, args),
		Rows:         [][]interface{}{},
		Columns:      []string{},
	}

	fmt.Fprintf(os.Stderr, "Searching through table: %s\n", tableName)

	// Execute query
	rows, err := db.Query(query, args...)
	if err != nil {
		return result, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	resultColumns, err := rows.Columns()
	if err != nil {
		return result, fmt.Errorf("failed to get columns: %w", err)
	}
	result.Columns = resultColumns

	// Fetch all rows
	for rows.Next() {
		values := make([]interface{}, len(resultColumns))
		valuePtrs := make([]interface{}, len(resultColumns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return result, fmt.Errorf("failed to scan row: %w", err)
		}

		result.Rows = append(result.Rows, values)
	}

	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}

func formatValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	if b, ok := val.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", val)
}

func buildDisplayQuery(query string, args []interface{}) string {
	result := query
	for _, arg := range args {
		value := fmt.Sprintf("'%v'", arg)
		result = strings.Replace(result, "?", value, 1)
	}
	return result
}
