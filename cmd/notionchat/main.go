package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mughu-id/notionchat/internal/api"
	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/credentials"
)

func main() {
	settings := config.Load()
	creds := credentials.NewStore(settings.SessionFile, settings.AccountPath)
	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	server := api.NewServer(settings, creds)

	log.Printf("NotionChat Go server starting on http://%s", addr)
	log.Printf("Connect Notion session: http://%s/", addr)

	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}