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
		"type": "workflow", "modelFromUser": !isSubsequent,
		"enableAgentAutomations": false, "enableAgentIntegrations": false,
		"enableCustomAgents": false, "enableExperimentalIntegrations": false,
		"enableAgentDiffs": false, "enableCsvAttachmentSupport": false,
		"showDatabaseAgentsDiscoverability": false, "enableAgentThreadTools": false,
		"enableCrdtOperations": false, "enableAgentCardCustomization": false,
		"enableSystemPromptAsPage": false, "enableUserSessionContext": false,
		"enableLargeToolResultComputerOffload": false, "enableScriptAgentAdvanced": false,
		"enableScriptAgent": false, "enableScriptAgentSearchConnectorsInCustomAgent": false,
		"enableScriptAgentGoogleDriveInCustomAgent": false,
		"enableScriptAgentGoogleDriveOAuthInCustomAgent": false,
		"enableScriptAgentSlack": false, "enableScriptAgentMcpServers": false,
		"enableScriptAgentGtm": false, "enableScriptAgentCustomToolCalling": false,
		"enableComputer": false, "enableCreateAndRunThread": false,
		"enableSoftwareFactoryPage": false, "enableAgentGenerateImage": false,
		"enableSpeculativeSearch": false, "enableQueryCalendar": false,
		"enableQueryMail": false, "enableMailExplicitToolCalls": false,
		"enableMailNotificationPreferences": false, "enableMailAgentMultiProviderSupport": false,
		"useRulePrioritization": true, "availableConnectors": []any{},
		"customConnectorInfo": []any{}, "searchScopes": []any{map[string]string{"type": "everything"}},
		"useWebSearch": true, "isHipaa": false, "internetAccess": true,
		"useReadOnlyMode": false, "writerMode": false, "isCustomAgent": false,
		"model": notionModel, "isCustomAgentBuilder": false, "isAgentResearchRequest": false,
		"useCustomAgentDraft": false, "use_draft_actor_pointer": false,
		"enableUpdatePageAutofixer": false, "enableMarkdownVNext": false,
		"enableEmbedBlocks": false, "updatePageStaleViewGuardEnabled": false,
		"enableUpdatePageOrderUpdates": false, "enableAgentSupportPropertyReorder": false,
		"agentShortUpdatePageResult": false, "enableAgentAskSurvey": false,
		"databaseAgentConfigMode": false, "isOnboardingAgent": false, "isMobile": false,
	}
	if ideAgentMode {
		cfg["useWebSearch"] = false
		cfg["enableAgentThreadTools"] = false
		cfg["searchScopes"] = []any{map[string]string{"type": "workspace"}}
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
	return map[string]any{
		"timezone": acc.Timezone, "userName": acc.UserName, "userId": acc.UserID,
		"userEmail": acc.UserEmail, "spaceName": acc.SpaceName, "spaceId": acc.SpaceID,
		"spaceViewId": acc.SpaceViewID, "currentDatetime": currentDatetime, "surface": "ai_module",
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
		"generateTitle": false, "saveAllThreadOperations": false, "setUnreadState": false,
		"threadType": "workflow", "asPatchResponse": true, "patchResponseVersion": 2,
		"hasHeartbeat": false, "createdSource": "ai_module",
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