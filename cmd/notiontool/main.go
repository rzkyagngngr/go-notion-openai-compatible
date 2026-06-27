package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mughu-id/notionchat/internal/account"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	accPath := getenv("NOTIONCHAT_ACCOUNT", "data/notion_account.json")
	switch os.Args[1] {
	case "account-cookie":
		acc, err := account.Load(accPath)
		if err != nil {
			fatal(err)
		}
		fmt.Println(account.BuildCookieHeader(acc))
	case "account-field":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: notiontool account-field <key>"))
		}
		acc, err := account.Load(accPath)
		if err != nil {
			fatal(err)
		}
		data, _ := json.Marshal(acc)
		var raw map[string]any
		_ = json.Unmarshal(data, &raw)
		if v, ok := raw[os.Args[2]]; ok {
			fmt.Printf("%v\n", v)
			return
		}
		fatal(fmt.Errorf("unknown field %q", os.Args[2]))
	case "session-field":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: notiontool session-field <key>"))
		}
		path := getenv("NOTIONCHAT_SESSION_FILE", "data/session.json")
		data, err := os.ReadFile(path)
		if err != nil {
			fatal(err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			fatal(err)
		}
		if v, ok := raw[os.Args[2]]; ok {
			fmt.Printf("%v\n", v)
			return
		}
		fatal(fmt.Errorf("unknown field %q", os.Args[2]))
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n  notiontool account-cookie\n  notiontool account-field <key>\n  notiontool session-field <key>\n")
	fmt.Fprintf(os.Stderr, "Account path: %s\n", filepath.Clean(getenv("NOTIONCHAT_ACCOUNT", "data/notion_account.json")))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}