package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
)

type ServerInfo struct {
	Username string `json:"username"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
}

type MagerunEntry struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// sshClientConn at package level so the SSH tunnel stays open while db is in use
var sshClientConn *ssh.Client

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, path[2:]), nil
	}
	return path, nil
}

func connectSSH(serverJSON string) (*sql.DB, string, error) {
	var info ServerInfo
	if err := json.Unmarshal([]byte(serverJSON), &info); err != nil {
		return nil, "", fmt.Errorf("invalid server JSON: %w", err)
	}

	client, err := dialSSH(info)
	if err != nil {
		return nil, "", fmt.Errorf("SSH connection failed: %w", err)
	}
	sshClientConn = client

	fmt.Fprintf(os.Stderr, "Connected to %s@%s:%d\n", info.Username, info.Server, info.Port)

	magentoRoot, err := findMagentoRoot(client)
	if err != nil {
		return nil, "", fmt.Errorf("could not find Magento root: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found Magento root: %s\n", magentoRoot)

	dbInfoJSON, err := runMagerun(client, magentoRoot, "db:info --format=json")
	if err != nil {
		return nil, "", fmt.Errorf("magerun failed: %w", err)
	}

	host, dbname, username, password, err := parseMagerunDbInfo(dbInfoJSON)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse db info: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Connecting to database: %s\n", dbname)

	mysql.RegisterDialContext("mysql+ssh", func(ctx context.Context, addr string) (net.Conn, error) {
		return sshClientConn.Dial("tcp", addr)
	})

	// magerun showign "localhost" meaning the DB is local to the remote server;
	// convert to 127.0.0.1 so the MySQL driver uses TCP through the tunnel
	if host == "localhost" {
		host = "127.0.0.1"
	}

	dsn := fmt.Sprintf("%s:%s@mysql+ssh(%s:3306)/%s", username, password, host, dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, "", fmt.Errorf("failed to ping database: %w", err)
	}

	return db, dbname, nil
}

func parseMagerunDbInfo(jsonStr string) (host, dbname, username, password string, err error) {
	var entries map[string]MagerunEntry
	if err = json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		return
	}
	for _, entry := range entries {
		switch entry.Name {
		case "host":
			host = entry.Value
		case "dbname":
			dbname = entry.Value
		case "username":
			username = entry.Value
		case "password":
			password = entry.Value
		}
	}
	if host == "" || dbname == "" || username == "" {
		err = fmt.Errorf("incomplete database info from magerun (got host=%q dbname=%q username=%q)", host, dbname, username)
	}
	return
}

func dialSSH(info ServerInfo) (*ssh.Client, error) {
	keyPaths := []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"}

	var lastErr error
	for _, keyPath := range keyPaths {
		expanded, err := expandPath(keyPath)
		if err != nil {
			continue
		}
		key, err := os.ReadFile(expanded)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			lastErr = fmt.Errorf("could not parse %s: %w", keyPath, err)
			continue
		}
		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", info.Server, info.Port), &ssh.ClientConfig{
			User:            info.Username,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		})
		if err != nil {
			lastErr = err
			continue
		}
		return client, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no SSH keys found in %v", keyPaths)
}

func findMagentoRoot(client *ssh.Client) (string, error) {
	// bin/magento exists only in a proper Magento root that also has vendor/
	out, _ := runSSHCommand(client, "find ~ -maxdepth 10 -path '*/bin/magento' -not -path '*/vendor/*' 2>/dev/null | head -1")
	if binMagento := strings.TrimSpace(out); binMagento != "" {
		// bin/magento is at <root>/bin/magento
		return path.Dir(path.Dir(binMagento)), nil
	}
	return "", fmt.Errorf("bin/magento not found on remote server")
}

func runMagerun(client *ssh.Client, root, command string) (string, error) {
	candidates := []string{"n98-magerun2", "magerun2", "magerun2-latest", "magerun", "php n98-magerun2.phar"}
	for _, magerun := range candidates {
		out, _ := runSSHCommand(client, fmt.Sprintf("cd %s && %s %s 2>/dev/null", root, magerun, command))
		if strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	return "", fmt.Errorf("no working magerun found (tried: %v)", candidates)
}

func runSSHCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var stdout bytes.Buffer
	session.Stdout = &stdout
	session.Run(cmd) // non-zero exit codes are not errors (because of magerun)
	return stdout.String(), nil
}

func connectSSHWordPress(serverJSON string) (*sql.DB, string, error) {
	var info ServerInfo
	if err := json.Unmarshal([]byte(serverJSON), &info); err != nil {
		return nil, "", fmt.Errorf("invalid server JSON: %w", err)
	}

	client, err := dialSSH(info)
	if err != nil {
		return nil, "", fmt.Errorf("SSH connection failed: %w", err)
	}
	sshClientConn = client

	fmt.Fprintf(os.Stderr, "Connected to %s@%s:%d\n", info.Username, info.Server, info.Port)

	wpConfigPath, err := findWordPressConfig(client)
	if err != nil {
		return nil, "", fmt.Errorf("could not find wp-config.php: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found wp-config.php: %s\n", wpConfigPath)

	host, dbname, username, password, err := readWpConfig(client, wpConfigPath)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse wp-config.php: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Connecting to WordPress database: %s\n", dbname)

	mysql.RegisterDialContext("mysql+ssh", func(ctx context.Context, addr string) (net.Conn, error) {
		return sshClientConn.Dial("tcp", addr)
	})

	if host == "localhost" {
		host = "127.0.0.1"
	}

	dsn := fmt.Sprintf("%s:%s@mysql+ssh(%s:3306)/%s", username, password, host, dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, "", fmt.Errorf("failed to ping database: %w", err)
	}

	return db, dbname, nil
}

func findWordPressConfig(client *ssh.Client) (string, error) {
	out, _ := runSSHCommand(client, "find ~ -maxdepth 8 -name 'wp-config.php' 2>/dev/null | head -1")
	if wpConfig := strings.TrimSpace(out); wpConfig != "" {
		return wpConfig, nil
	}
	return "", fmt.Errorf("wp-config.php not found on remote server")
}

func readWpConfig(client *ssh.Client, configPath string) (host, dbname, username, password string, err error) {
	out, _ := runSSHCommand(client, fmt.Sprintf("cat '%s'", configPath))
	content := strings.TrimSpace(out)
	if content == "" {
		return "", "", "", "", fmt.Errorf("failed to read wp-config.php at %s", configPath)
	}

	dbname = parseWpDefine(content, "DB_NAME")
	username = parseWpDefine(content, "DB_USER")
	password = parseWpDefine(content, "DB_PASSWORD")
	host = parseWpDefine(content, "DB_HOST")

	if dbname == "" || username == "" || host == "" {
		err = fmt.Errorf("incomplete WordPress database info (got host=%q dbname=%q username=%q)", host, dbname, username)
	}
	return
}

func parseWpDefine(content, name string) string {
	pattern := fmt.Sprintf(`(?i)define\s*\(\s*['"]%s['"]\s*,\s*['"]([^'"]+)['"]`, regexp.QuoteMeta(name))
	re := regexp.MustCompile(pattern)
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return m[1]
	}
	return ""
}
