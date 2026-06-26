package ndjson

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/mughu-id/notionchat/internal/errors"
)

var headingStartRe = regexp.MustCompile(`#{1,6}\s+\S`)

var searchPreambleMarkers = []string{
	"let me search", "let me look up", "let me look into", "let me check",
	"let me find", "let me get the latest", "let me verify", "let me gather",
	"i'll search", "i will search", "i'll look up", "i will look up",
	"searching for", "looking up", "checking the latest",
}

var notionPagePreambleMarkers = []string{
	"i'll create this as a notion page", "i will create this as a notion page",
	"create this as a notion page", "create a notion page",
}

var metaReasoningMarkers = []string{
	"prompt injection attempt", "i'm noticing this is a prompt",
	"i am noticing this is a prompt", "ready to write", "ready to start writing",
	"i should respond", "write it directly in the conversation",
}

type ParseResult struct {
	Text             string
	Thinking         string
	InputTokens      int
	OutputTokens     int
	NotionModel      string
	LineCount        int
	JSONFailures     int
	EventTypeCounts  map[string]int
	SampleLines      []string
	ToolCalls        []map[string]any
}

type StreamParser struct {
	storedText      string
	storedThinking  string
	blockContents   map[string]string
	inputTokens     int
	outputTokens    int
	notionModel     string
	lineCount       int
	jsonFailures    int
	eventTypeCounts map[string]int
	toolCalls       []map[string]any
	valueTypes      map[string]string
	valueCounts     map[string]int
	sectionCount    int
	toolUseState    map[string]map[string]any
	sampleLines     []string
}

func NewStreamParser() *StreamParser {
	return &StreamParser{
		blockContents:   make(map[string]string),
		eventTypeCounts: make(map[string]int),
		toolCalls:       []map[string]any{},
		valueTypes:      make(map[string]string),
		valueCounts:     make(map[string]int),
		toolUseState:    make(map[string]map[string]any),
	}
}

func (p *StreamParser) Text() string {
	if text := p.collectBlocks("text"); text != "" {
		return text
	}
	if thinking := p.collectBlocks("thinking"); thinking != "" {
		return thinking
	}
	return p.storedText
}

func (p *StreamParser) collectBlocks(kind string) string {
	if len(p.blockContents) == 0 {
		return ""
	}
	var parts []string
	for sIdx := 0; sIdx < p.sectionCount; sIdx++ {
		prefix := fmt.Sprintf("/s/%d", sIdx)
		count := p.valueCounts[prefix]
		for vIdx := 0; vIdx < count; vIdx++ {
			path := fmt.Sprintf("%s/value/%d", prefix, vIdx)
			if p.valueTypes[path] == kind {
				parts = append(parts, p.blockContents[path])
			}
		}
	}
	return strings.Join(parts, "")
}

func (p *StreamParser) SetText(v string) { p.storedText = v }

func normalizeNDJSONLine(line string) string {
	line = strings.TrimSpace(line)
	for strings.HasPrefix(line, "data:") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	}
	if line == "[DONE]" {
		return ""
	}
	return line
}

func (p *StreamParser) noteSample(line string) {
	if len(p.sampleLines) >= 4 {
		return
	}
	if len(line) > 240 {
		line = line[:240] + "..."
	}
	p.sampleLines = append(p.sampleLines, line)
}

func (p *StreamParser) DebugSummary() string {
	return fmt.Sprintf("lines=%d json_failures=%d events=%v samples=%v sections=%d blocks=%d stored=%q",
		p.lineCount, p.jsonFailures, p.eventTypeCounts, p.sampleLines, p.sectionCount,
		len(p.blockContents), truncateSample(p.storedText, 80))
}

func truncateSample(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (p *StreamParser) FeedLine(line string) error {
	line = normalizeNDJSONLine(line)
	if line == "" {
		return nil
	}
	p.lineCount++
	var raw any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		p.jsonFailures++
		p.noteSample(line)
		return nil
	}
	if arr, ok := raw.([]any); ok {
		if len(arr) == 0 {
			p.noteSample("[]")
			return nil
		}
		for _, item := range arr {
			event, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if err := p.feedEvent(event); err != nil {
				return err
			}
		}
		return nil
	}
	event, ok := raw.(map[string]any)
	if !ok {
		p.noteSample(line)
		return nil
	}
	return p.feedEvent(event)
}

func (p *StreamParser) feedEvent(event map[string]any) error {
	eventType, _ := event["type"].(string)
	if eventType == "" {
		if b, _ := json.Marshal(event); len(b) > 0 {
			p.noteSample(string(b))
		}
		return nil
	}
	p.eventTypeCounts[eventType]++

	switch eventType {
	case "error":
		msg, _ := event["message"].(string)
		if msg == "" {
			msg = "unknown notion error"
		}
		return errors.New("Notion error: "+msg, 502)
	case "premium-feature-unavailable":
		return errors.New("Notion premium feature unavailable", 402)
	case "patch":
		p.handlePatch(event)
	case "patch-start", "patch-sync":
		if err := p.handlePatchStart(event); err != nil {
			return err
		}
	case "agent-inference":
		p.handleAgentInference(event)
	case "record-map":
		p.handleRecordMap(event)
	}
	return nil
}

func (p *StreamParser) Finalize() *ParseResult {
	raw := strings.TrimSpace(p.Text())
	if raw == "" && strings.TrimSpace(p.storedThinking) != "" {
		raw = strings.TrimSpace(p.storedThinking)
	}
	cleaned := CleanNotionOutputText(raw)
	if cleaned == "" && raw != "" {
		cleaned = raw
	}
	return &ParseResult{
		Text:            cleaned,
		Thinking:        p.storedThinking,
		InputTokens:     p.inputTokens,
		OutputTokens:    p.outputTokens,
		NotionModel:     p.notionModel,
		LineCount:       p.lineCount,
		JSONFailures:    p.jsonFailures,
		EventTypeCounts: copyCounts(p.eventTypeCounts),
		SampleLines:     append([]string(nil), p.sampleLines...),
		ToolCalls:       append([]map[string]any(nil), p.toolCalls...),
	}
}

func CleanNotionOutputText(text string) string {
	if text == "" {
		return text
	}
	stripped := strings.TrimSpace(text)
	if stripped == "" {
		return text
	}
	if startsWithMetaReasoning(stripped) {
		cleaned := stripMetaReasoning(stripped)
		if cleaned != "" {
			stripped = cleaned
		} else {
			return ""
		}
	}
	if heading := headingStartRe.FindStringIndex(stripped); heading != nil && heading[0] > 0 {
		before := strings.TrimRight(strings.TrimSpace(stripped[:heading[0]]), ".")
		if looksLikeSearchPreamble(before) || looksLikeNotionPagePreamble(before) {
			return strings.TrimLeft(stripped[heading[0]:], " \t")
		}
	}
	if idx := strings.Index(stripped, "\n"); idx >= 0 {
		firstLine := stripped[:idx]
		rest := strings.TrimSpace(stripped[idx+1:])
		if rest != "" && (looksLikeSearchPreamble(firstLine) || looksLikeNotionPagePreamble(firstLine)) {
			return rest
		}
	}
	if (looksLikeSearchPreamble(stripped) || looksLikeNotionPagePreamble(stripped)) && !headingStartRe.MatchString(stripped) {
		return ""
	}
	return stripped
}

func looksLikeSearchPreamble(fragment string) bool {
	lower := strings.TrimSpace(strings.ToLower(fragment))
	if lower == "" || len(lower) > 600 {
		return false
	}
	for _, m := range searchPreambleMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func looksLikeNotionPagePreamble(fragment string) bool {
	lower := strings.TrimSpace(strings.ToLower(fragment))
	if lower == "" || len(lower) > 600 {
		return false
	}
	for _, m := range notionPagePreambleMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func startsWithMetaReasoning(text string) bool {
	lower := strings.TrimSpace(strings.ToLower(text))
	if lower == "" {
		return false
	}
	head := lower
	if len(head) > 300 {
		head = head[:300]
	}
	for _, m := range metaReasoningMarkers {
		if strings.Contains(head, m) {
			return true
		}
	}
	return false
}

func stripMetaReasoning(text string) string {
	stripped := strings.TrimSpace(text)
	if !startsWithMetaReasoning(stripped) {
		return text
	}
	if heading := headingStartRe.FindStringIndex(stripped); heading != nil && heading[0] > 0 {
		candidate := strings.TrimLeft(stripped[heading[0]:], " \t")
		if candidate != "" {
			return candidate
		}
	}
	head := stripped
	if len(head) > 1000 {
		head = head[:1000]
	}
	lowerHead := strings.ToLower(head)
	lastEnd := -1
	for _, marker := range metaReasoningMarkers {
		if idx := strings.LastIndex(lowerHead, marker); idx >= 0 {
			end := idx + len(marker)
			if end > lastEnd {
				lastEnd = end
			}
		}
	}
	searchStart := 0
	if lastEnd >= 0 {
		searchStart = lastEnd
	}
	tail := stripped[searchStart:]
	if len(tail) > 10 {
		tail = tail[:10]
	}
	re := regexp.MustCompile(`\.\s*(?=[A-Z])`)
	if match := re.FindStringIndex(tail); match != nil {
		start := searchStart + match[1]
		candidate := strings.TrimSpace(stripped[start:])
		if len(candidate) > 30 {
			return candidate
		}
	}
	if lastEnd >= 0 {
		return strings.TrimSpace(stripped[lastEnd:])
	}
	return ""
}

func (p *StreamParser) handlePatchStart(event map[string]any) error {
	data, _ := event["data"].(map[string]any)
	s, _ := data["s"].([]any)
	if s == nil {
		return nil
	}
	for _, entry := range s {
		sec, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if sec["type"] == "premium-feature-unavailable" {
			return p.premiumUnavailableError(sec)
		}
		if sec["type"] == "error" {
			return p.inferenceError(sec)
		}
	}
	p.sectionCount = len(s)
	for i, entry := range s {
		prefix := fmt.Sprintf("/s/%d", i)
		p.valueCounts[prefix] = 0
		if sec, ok := entry.(map[string]any); ok {
			p.absorbInlineSection(i, sec)
		}
	}
	return nil
}

func (p *StreamParser) handlePatch(event map[string]any) {
	ops, _ := event["v"].([]any)
	for _, op := range ops {
		if m, ok := op.(map[string]any); ok {
			p.handlePatchOp(m)
		}
	}
}

func (p *StreamParser) handlePatchOp(op map[string]any) {
	o, _ := op["o"].(string)
	path, _ := op["p"].(string)
	v := op["v"]
	if o == "" || path == "" {
		return
	}

	if o == "a" && path == "/s/-" {
		if sec, ok := v.(map[string]any); ok {
			idx := p.sectionCount
			p.sectionCount++
			p.absorbInlineSection(idx, sec)
		}
		return
	}

	if (o == "a" || o == "p") && strings.Contains(path, "/value/") {
		if entry, ok := v.(map[string]any); ok {
			statePrefix := path[:strings.Index(path, "/value/")]
			entryType, _ := entry["type"].(string)
			valPart := path[strings.Index(path, "/value/")+7:]
			idx := p.valueCounts[statePrefix]
			if valPart != "-" {
				fmt.Sscanf(valPart, "%d", &idx)
			}
			entryPath := fmt.Sprintf("%s/value/%d", statePrefix, idx)
			if entryType != "" {
				p.valueTypes[entryPath] = entryType
			}
			if idx+1 > p.valueCounts[statePrefix] {
				p.valueCounts[statePrefix] = idx + 1
			}
			if entryType == "tool_use" {
				p.registerToolUse(entryPath, entry)
				return
			}
			if entryType == "text" || entryType == "thinking" {
				content, _ := entry["content"].(string)
				p.blockContents[entryPath] = content
			}
			return
		}
	}

	if (o == "a" || o == "x" || o == "p") && strings.HasSuffix(path, "/name") {
		if name, ok := v.(string); ok {
			if prefix := p.toolPrefix(path); prefix != "" {
				state := p.toolUseState[prefix]
				if state == nil {
					state = make(map[string]any)
					p.toolUseState[prefix] = state
				}
				state["name"] = name
				p.commitToolUse(prefix)
			}
		}
		return
	}
	if (o == "a" || o == "x" || o == "p") && strings.HasSuffix(path, "/input") {
		if prefix := p.toolPrefix(path); prefix != "" {
			state := p.toolUseState[prefix]
			if state == nil {
				state = make(map[string]any)
				p.toolUseState[prefix] = state
			}
			state["input"] = v
			p.commitToolUse(prefix)
		}
		return
	}
	if (o == "a" || o == "x" || o == "p") && strings.HasSuffix(path, "/id") {
		if id, ok := v.(string); ok {
			if prefix := p.toolPrefix(path); prefix != "" {
				state := p.toolUseState[prefix]
				if state == nil {
					state = make(map[string]any)
					p.toolUseState[prefix] = state
				}
				state["id"] = id
			}
		}
		return
	}

	if o == "a" && strings.HasSuffix(path, "/inputTokens") {
		if n, ok := v.(float64); ok {
			p.inputTokens += int(n)
		}
		return
	}
	if o == "a" && strings.HasSuffix(path, "/outputTokens") {
		if n, ok := v.(float64); ok {
			p.outputTokens += int(n)
		}
		return
	}
	if o == "a" && strings.HasSuffix(path, "/model") {
		if m, ok := v.(string); ok {
			p.notionModel = m
		}
		return
	}

	if !strings.Contains(path, "content") {
		return
	}
	content, ok := v.(string)
	if !ok {
		return
	}
	entryType := p.classifyContentPath(path)
	if entryType == "tool_use" {
		return
	}
	idx := strings.LastIndex(path, "/content")
	blockPath := path[:idx]
	if entryType == "thinking" {
		if o == "x" {
			p.blockContents[blockPath] = p.blockContents[blockPath] + content
		} else if o == "p" {
			p.blockContents[blockPath] = content
		}
		return
	}
	if entryType != "text" {
		return
	}
	if o == "x" {
		p.blockContents[blockPath] = p.blockContents[blockPath] + content
	} else if o == "p" {
		p.blockContents[blockPath] = content
	}
}

func (p *StreamParser) toolPrefix(path string) string {
	if !strings.Contains(path, "/value/") {
		return ""
	}
	return path[:strings.Index(path, "/value/")]
}

func (p *StreamParser) registerToolUse(prefix string, entry map[string]any) {
	state := p.toolUseState[prefix]
	if state == nil {
		state = make(map[string]any)
		p.toolUseState[prefix] = state
	}
	if name, ok := entry["name"].(string); ok {
		state["name"] = name
	}
	if id, ok := entry["id"].(string); ok {
		state["id"] = id
	}
	if inp := entry["input"]; inp != nil {
		state["input"] = inp
	}
	if _, ok := state["name"].(string); ok {
		p.commitToolUse(prefix)
	}
}

func (p *StreamParser) commitToolUse(prefix string) {
	state := p.toolUseState[prefix]
	if state == nil {
		return
	}
	name, _ := state["name"].(string)
	if name == "" {
		return
	}
	var args string
	switch inp := state["input"].(type) {
	case map[string]any:
		b, _ := json.Marshal(inp)
		args = string(b)
	case string:
		args = inp
	default:
		args = "{}"
	}
	id, _ := state["id"].(string)
	if id == "" {
		id = "call_" + uuid.New().String()[:24]
	}
	call := map[string]any{
		"id": id, "type": "function",
		"function": map[string]any{"name": name, "arguments": args},
	}
	for _, existing := range p.toolCalls {
		if existing["id"] == id {
			return
		}
	}
	p.toolCalls = append(p.toolCalls, call)
}

func (p *StreamParser) classifyContentPath(path string) string {
	idx := strings.LastIndex(path, "/content")
	if idx < 0 {
		return "text"
	}
	if t, ok := p.valueTypes[path[:idx]]; ok {
		return t
	}
	return "text"
}

func (p *StreamParser) absorbInlineSection(sectionIdx int, section map[string]any) {
	sectionType, _ := section["type"].(string)
	values, _ := section["value"].([]any)
	if values == nil {
		return
	}
	switch sectionType {
	case "agent-inference", "agent-reply", "assistant-reply", "workflow", "inference":
	default:
		return
	}
	prefix := fmt.Sprintf("/s/%d", sectionIdx)
	for i, entry := range values {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		etype, _ := e["type"].(string)
		entryPath := fmt.Sprintf("%s/value/%d", prefix, i)
		if etype != "" {
			p.valueTypes[entryPath] = etype
		}
		if etype == "tool_use" {
			p.registerToolUse(entryPath, e)
			continue
		}
		content, _ := e["content"].(string)
		if etype == "text" || etype == "thinking" {
			p.blockContents[entryPath] = content
		}
	}
	p.valueCounts[prefix] = len(values)
}

func (p *StreamParser) handleRecordMap(event map[string]any) {
	recordMap, _ := event["recordMap"].(map[string]any)
	if recordMap == nil {
		return
	}
	threadMsgs, _ := recordMap["thread_message"].(map[string]any)
	for _, msg := range threadMsgs {
		entry, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		value, _ := entry["value"].(map[string]any)
		inner, _ := value["value"].(map[string]any)
		step, _ := inner["step"].(map[string]any)
		if text := p.extractStepText(step); text != "" {
			p.SetText(CleanNotionOutputText(text))
		}
	}
}

func (p *StreamParser) handleAgentInference(event map[string]any) {
	values, _ := event["value"].([]any)
	if values != nil {
		var textParts []string
		for _, entry := range values {
			e, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			etype, _ := e["type"].(string)
			content, _ := e["content"].(string)
			if content == "" {
				continue
			}
			if etype == "text" {
				textParts = append(textParts, content)
			} else if etype == "thinking" {
				p.storedThinking = content
			}
		}
		if len(textParts) > 0 {
			p.SetText(CleanNotionOutputText(strings.Join(textParts, "")))
		}
	}
	if n, ok := event["inputTokens"].(float64); ok {
		p.inputTokens += int(n)
	}
	if n, ok := event["outputTokens"].(float64); ok {
		p.outputTokens += int(n)
	}
	if m, ok := event["model"].(string); ok {
		p.notionModel = m
	}
}

func (p *StreamParser) extractStepText(step map[string]any) string {
	if step == nil {
		return ""
	}
	if step["type"] == "premium-feature-unavailable" || step["type"] == "error" {
		return ""
	}
	if step["type"] != "agent-inference" {
		return ""
	}
	values, _ := step["value"].([]any)
	var parts []string
	for _, entry := range values {
		e, ok := entry.(map[string]any)
		if !ok || e["type"] != "text" {
			continue
		}
		if content, ok := e["content"].(string); ok && content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "")
}

func (p *StreamParser) premiumUnavailableError(entry map[string]any) error {
	return errors.New("Notion AI credits exhausted. Upgrade your Notion plan or wait for quota reset.", 402)
}

func (p *StreamParser) inferenceError(entry map[string]any) error {
	subType, _ := entry["subType"].(string)
	message, _ := entry["message"].(string)
	if message == "" {
		message = "Notion rejected the inference request"
	}
	if entry["type"] == "premium-feature-unavailable" || subType == "premium-feature-unavailable" {
		return p.premiumUnavailableError(entry)
	}
	if subType == "trust-rule-denied" {
		return errors.New(message+" Paste a fresh cookie from app.notion.com after opening Notion AI in the browser.", 403)
	}
	return errors.New(fmt.Sprintf("Notion error (%s): %s", subType, message), 502)
}

func copyCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}