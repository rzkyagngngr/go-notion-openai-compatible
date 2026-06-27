package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mughu-id/notionchat/internal/api"
	"github.com/mughu-id/notionchat/internal/browserrefresh"
	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/credentials"
)

func main() {
	settings := config.Load()
	brCfg := browserrefresh.LoadConfig()
	refresher := browserrefresh.NewRefresher(brCfg)
	creds := credentials.NewStore(settings.SessionFile, settings.AccountPath, refresher)
	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	server := api.NewServer(settings, creds)

	stopRefresh := make(chan struct{})
	defer close(stopRefresh)
	go creds.StartBackgroundRefresh(stopRefresh)

	log.Printf("NotionChat Go server starting on http://%s", addr)
	log.Printf("Browser refresh mode: %s", brCfg.Mode)
	log.Printf("Seed session from Windows: go run ./cmd/notionsync --url http://%s", addr)

	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}