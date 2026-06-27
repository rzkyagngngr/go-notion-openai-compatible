package api

import (
	"log"
	"strings"

	"github.com/mughu-id/notionchat/internal/client"
	"github.com/mughu-id/notionchat/internal/errors"
)

func (s *Server) completeWithAutoRefresh(
	c *client.NotionAIClient,
	prompt, system, model, threadID, latestUser string,
	ideAgent, toolsActive bool,
	clientTools []map[string]any,
) (*client.ChatResult, *client.NotionAIClient, error) {
	result, err := c.Complete(prompt, system, model, threadID, latestUser, ideAgent, toolsActive, clientTools)
	if err == nil {
		return result, c, nil
	}
	if !isStaleSessionError(err) {
		return nil, c, err
	}
	if _, refreshErr := s.credentials.RefreshAll(); refreshErr != nil {
		log.Printf("auto-refresh failed: %v", refreshErr)
		return nil, c, err
	}
	if !s.credentials.SessionHealthy() {
		return nil, c, err
	}
	log.Printf("auto-refresh: retrying chat after session recovery")
	c2, clientErr := s.getClient()
	if clientErr != nil {
		return nil, c, err
	}
	result2, err2 := c2.Complete(prompt, system, model, "", latestUser, ideAgent, toolsActive, clientTools)
	return result2, c2, err2
}

func (s *Server) streamWithAutoRefresh(
	c *client.NotionAIClient,
	prompt, system, model, threadID, latestUser string,
	ideAgent, toolsActive bool,
	clientTools []map[string]any,
) (*client.StreamHandle, *client.NotionAIClient, error) {
	handle, err := c.StreamDeltas(prompt, system, model, threadID, latestUser, ideAgent, toolsActive, clientTools)
	if err == nil {
		return handle, c, nil
	}
	if !isStaleSessionError(err) {
		return nil, c, err
	}
	if _, refreshErr := s.credentials.RefreshAll(); refreshErr != nil {
		return nil, c, err
	}
	if !s.credentials.SessionHealthy() {
		return nil, c, err
	}
	log.Printf("auto-refresh: retrying stream after session recovery")
	c2, clientErr := s.getClient()
	if clientErr != nil {
		return nil, c, err
	}
	handle2, err2 := c2.StreamDeltas(prompt, system, model, "", latestUser, ideAgent, toolsActive, clientTools)
	return handle2, c2, err2
}

func isStaleSessionError(err error) bool {
	e, ok := err.(*errors.NotionChatError)
	if !ok || e.StatusCode != 502 {
		return false
	}
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "session expired") ||
		strings.Contains(msg, "token_v2") ||
		strings.Contains(msg, "empty inference stream") ||
		strings.Contains(msg, "reconnect at /")
}