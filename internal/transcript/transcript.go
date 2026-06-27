package transcript

import (
	"time"

	"github.com/google/uuid"

	"github.com/mughu-id/notionchat/internal/account"
)

func NewUUID() string {
	return uuid.New().String()
}

func NowISO(tz string) string {
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	return time.Now().In(loc).Format("2006-01-02T15:04:05.000Z07:00")
}

func BuildConfigValue(notionModel string, isSubsequent, ideAgentMode bool) map[string]any {
	cfg := map[string]any{
		"type": "workflow", "modelFromUser": false,
		"enableAgentAutomations": true, "enableAgentIntegrations": true,
		"enableCustomAgents": true, "enableExperimentalIntegrations": false,
		"enableScriptAgent": true, "enableScriptAgentAdvanced": false,
		"enableScriptAgentSearchConnectorsInCustomAgent": false,
		"enableScriptAgentGoogleDriveInCustomAgent": false,
		"enableScriptAgentGoogleDriveOAuthInCustomAgent": false,
		"enableScriptAgentSlack": true, "enableScriptAgentMcpServers": false,
		"enableAgentDiffs": true, "enableCsvAttachmentSupport": true,
		"showDatabaseAgentsDiscoverability": false, "enableAgentThreadTools": false,
		"enableCrdtOperations": false, "enableAgentCardCustomization": true,
		"enableSystemPromptAsPage": false, "enableUserSessionContext": false,
		"enableLargeToolResultComputerOffload": false,
		"enableScriptAgentGtm": false, "enableComputer": false,
		"enableCreateAndRunThread": true, "enableCustomAgentCreateGuidanceV2": false,
		"enableSoftwareFactoryPage": false, "enableAgentGenerateImage": false,
		"enableQueryCalendar": false, "enableQueryMail": false,
		"enableMailExplicitToolCalls": true, "enableMailNotificationPreferences": false,
		"enableMailAgentMultiProviderSupport": true,
		"useRulePrioritization": true, "availableConnectors": []any{},
		"searchScopes": []any{map[string]string{"type": "everything"}},
		"useWebSearch": true, "isHipaa": false, "internetAccess": false,
		"manageWorkers": false, "useReadOnlyMode": false, "writerMode": false,
		"isCustomAgent": false, "isCustomAgentBuilder": false,
		"isAgentResearchRequest": false, "useCustomAgentDraft": false,
		"use_draft_actor_pointer": false,
		"enableUpdatePageAutofixer": true, "enableMarkdownVNext": false,
		"enableEmbedBlocks": true, "updatePageStaleViewGuardEnabled": false,
		"enableUpdatePageOrderUpdates": true, "enableAgentSupportPropertyReorder": true,
		"enableAgentAskSurvey": true, "databaseAgentConfigMode": false,
		"isOnboardingAgent": false, "isMobile": false,
	}
	if ideAgentMode {
		cfg["useWebSearch"] = false
		cfg["enableAgentThreadTools"] = false
		cfg["searchScopes"] = []any{map[string]string{"type": "workspace"}}
	}
	if notionModel != "" {
		cfg["model"] = notionModel
	}
	if isSubsequent {
		cfg["isThreadStartedByAdmin"] = true
	}
	return cfg
}

func BuildContextValue(acc *account.NotionAccount, currentDatetime string) map[string]any {
	if currentDatetime == "" {
		currentDatetime = NowISO(acc.Timezone)
	}
	userName := acc.UserName
	if userName == "" {
		userName = acc.UserEmail
	}
	return map[string]any{
		"timezone": acc.Timezone, "userName": userName, "userId": acc.UserID,
		"userEmail": acc.UserEmail, "spaceName": acc.SpaceName, "spaceId": acc.SpaceID,
		"spaceViewId": acc.SpaceViewID, "currentDatetime": currentDatetime, "surface": "workflows",
	}
}

func BuildFullTranscript(acc *account.NotionAccount, userText, notionModel string, configID, contextID, now string, ideAgentMode bool) []map[string]any {
	if now == "" {
		now = NowISO(acc.Timezone)
	}
	if configID == "" {
		configID = NewUUID()
	}
	if contextID == "" {
		contextID = NewUUID()
	}
	return []map[string]any{
		{"id": configID, "type": "config", "value": BuildConfigValue(notionModel, false, ideAgentMode)},
		{"id": contextID, "type": "context", "value": BuildContextValue(acc, now)},
		{"id": NewUUID(), "type": "user", "value": [][]string{{userText}}, "userId": acc.UserID, "createdAt": now},
	}
}

func BuildPartialTranscript(
	acc *account.NotionAccount,
	newUserText, notionModel, configID, contextID, originalDatetime string,
	updatedConfigIDs []string,
	ideAgentMode bool,
) []map[string]any {
	transcript := []map[string]any{
		{"id": configID, "type": "config", "value": BuildConfigValue(notionModel, true, ideAgentMode)},
		{"id": contextID, "type": "context", "value": BuildContextValue(acc, originalDatetime)},
	}
	for _, ucID := range updatedConfigIDs {
		transcript = append(transcript, map[string]any{"id": ucID, "type": "updated-config"})
	}
	transcript = append(transcript, map[string]any{
		"id": NewUUID(), "type": "user", "value": [][]string{{newUserText}},
		"userId": acc.UserID, "createdAt": NowISO(acc.Timezone),
	})
	return transcript
}

func BuildInferenceRequest(
	acc *account.NotionAccount,
	transcript []map[string]any,
	threadID string,
	createThread, isPartial bool,
	traceID string,
) map[string]any {
	if traceID == "" {
		traceID = NewUUID()
	}
	body := map[string]any{
		"traceId": traceID, "spaceId": acc.SpaceID, "transcript": transcript,
		"threadId": threadID, "createThread": createThread, "isPartialTranscript": isPartial,
		"generateTitle": createThread, "saveAllThreadOperations": true, "setUnreadState": true,
		"threadType": "workflow", "asPatchResponse": true, "patchResponseVersion": 2,
		"createdSource": "workflows",
		"isUserInAnySalesAssistedSpace": false, "isSpaceSalesAssisted": false,
		"debugOverrides": map[string]any{
			"emitAgentSearchExtractedResults": true,
			"cachedInferences":                map[string]any{},
			"annotationInferences":            map[string]any{},
			"emitInferences":                  false,
		},
	}
	if createThread {
		body["threadParentPointer"] = map[string]string{
			"table": "space", "id": acc.SpaceID, "spaceId": acc.SpaceID,
		}
	}
	return body
}