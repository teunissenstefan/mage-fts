package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mage-fts <search-term> [options]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --limit=N       Max results per table (default: 100)")
		fmt.Println("  --match=text    Only search text columns")
		fmt.Println("  --tables=PATTERN Only search matching tables")
		fmt.Println("  --exclude=PATTERN Exclude matching tables")
		fmt.Println("  --dry-run       Show queries without executing")
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
	fmt.Println("TODO")
}


