package codeximport

type ImportOpenAISessionsRequest struct {
	SourceProvider  string `json:"sourceProvider,omitempty"`
	TargetProvider  string `json:"targetProvider,omitempty"`
	CodexHome       string `json:"codexHome,omitempty"`
	SqliteHome      string `json:"sqliteHome,omitempty"`
	StateDBPath     string `json:"stateDbPath,omitempty"`
	IncludeArchived *bool  `json:"includeArchived,omitempty"`
	CreateBackup    *bool  `json:"createBackup,omitempty"`
	DryRun          bool   `json:"dryRun,omitempty"`
}

type ImportOpenAISessionsResponse struct {
	Success               bool     `json:"success"`
	Message               string   `json:"message,omitempty"`
	CodexHome             string   `json:"codexHome"`
	SqliteHome            string   `json:"sqliteHome"`
	StateDBPath           string   `json:"stateDbPath"`
	SourceProvider        string   `json:"sourceProvider"`
	TargetProvider        string   `json:"targetProvider"`
	DryRun                bool     `json:"dryRun"`
	ScannedFiles          int      `json:"scannedFiles"`
	UpdatedSessionFiles   int      `json:"updatedSessionFiles"`
	UpdatedArchivedFiles  int      `json:"updatedArchivedFiles"`
	UpdatedThreadRows     int64    `json:"updatedThreadRows"`
	UpdatedFiles          []string `json:"updatedFiles,omitempty"`
	BackupPaths           []string `json:"backupPaths,omitempty"`
	SkippedLockedFiles    []string `json:"skippedLockedFiles,omitempty"`
	SkippedInvalidJSONL   []string `json:"skippedInvalidJsonl,omitempty"`
	SkippedUnchangedFiles int      `json:"skippedUnchangedFiles"`
}
