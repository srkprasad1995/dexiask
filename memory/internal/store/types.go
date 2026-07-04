package store

// REST DTO shapes for the memory browser.

// ScopeInfo is one entry of GET /v1/memory/scopes.
type ScopeInfo struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
	Label string `json:"label"`
}

// EntryMeta is the listing view of a memory entry.
type EntryMeta struct {
	Topic   string `json:"topic"`
	Entry   string `json:"entry"`
	Scope   string `json:"scope"`
	ScopeID string `json:"scope_id"`
	Preview string `json:"preview"`
}

// EntryFull is a memory entry including its body.
type EntryFull struct {
	Topic   string `json:"topic"`
	Entry   string `json:"entry"`
	Scope   string `json:"scope"`
	ScopeID string `json:"scope_id"`
	Preview string `json:"preview"`
	Body    string `json:"body"`
}

// TopicInfo is one entry of GET /v1/memory/topics.
type TopicInfo struct {
	Topic      string   `json:"topic"`
	EntryCount int      `json:"entry_count"`
	Entries    []string `json:"entries"`
}

// WorkingFileInfo is one entry of GET /v1/memory/working.
type WorkingFileInfo struct {
	Filename   string `json:"filename"`
	Date       string `json:"date"`
	EntryCount int    `json:"entry_count"`
	Preview    string `json:"preview"`
}

// LogEntry is one entry of GET /v1/memory/log.
type LogEntry struct {
	Timestamp string   `json:"timestamp"`
	Action    string   `json:"action"`
	Details   []string `json:"details"`
}
