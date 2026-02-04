package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mage-fts <search-term> [options]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --limit=N\t\tMax results per table (default: 20)")//TODO: Implement
		fmt.Println("  --match=text\t\tOnly search text columns")//TODO: Implement
		fmt.Println("  --tables=PATTERN\tOnly search matching tables")//TODO: Implement
		fmt.Println("  --exclude=PATTERN\tExclude matching tables")//TODO: Implement
		fmt.Println("  --dry-run\t\tShow queries without executing")//TODO: Implement
		os.Exit(1)
	}

	// Check if we're in a Magento root
	envFile := "app/etc/env.php"
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Error: app/etc/env.php not found")
		os.Exit(1)
	}

	searchTerm := os.Args[1]
	fmt.Printf("Searching for: %s\n", searchTerm)

	// Get database connection info from DDEV
	db, dbName, err := connectDdev()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("Connected to database successfully")

	// Get tables and their text columns
	tableColumns, err := getTableColumns(db, dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting tables: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d tables:\n", len(tableColumns))
	for table, columns := range tableColumns {
		fmt.Printf("  %s: %v\n", table, columns)
	}

	fmt.Println("TODO: search for", searchTerm)
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

func getTableColumns(db *sql.DB, dbName string) (map[string][]string, error) {
	query := `
		SELECT TABLE_NAME, COLUMN_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME`

	rows, err := db.Query(query, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make(map[string][]string)
	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return nil, err
		}
		tables[tableName] = append(tables[tableName], columnName)
	}
	return tables, rows.Err()
}
