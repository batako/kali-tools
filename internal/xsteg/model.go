package xsteg

type Report struct {
	Version       int           `json:"version"`
	SourcePath    string        `json:"source_path"`
	SourceSHA256  string        `json:"source_sha256"`
	Size          int64         `json:"size"`
	MIME          string        `json:"mime"`
	Mode          string        `json:"mode"`
	ScanCompleted bool          `json:"scan_completed"`
	Status        string        `json:"status"`
	OutputPath    string        `json:"output_path"`
	StartedAt     string        `json:"started_at"`
	EndedAt       string        `json:"ended_at,omitempty"`
	Tools         []ToolResult  `json:"tools,omitempty"`
	Findings      []Finding     `json:"findings,omitempty"`
	Wordlists     []WordlistRun `json:"wordlists,omitempty"`
	Warnings      []string      `json:"warnings,omitempty"`
}

type ToolResult struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Command    []string `json:"command,omitempty"`
	OutputFile string   `json:"output_file,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

type Finding struct {
	Backend      string `json:"backend"`
	Kind         string `json:"kind"`
	Summary      string `json:"summary"`
	OriginalName string `json:"original_name,omitempty"`
	Path         string `json:"path,omitempty"`
	Password     string `json:"password,omitempty"`
}

type WordlistRun struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type listedReport struct {
	ID     int
	Path   string
	Report Report
}
