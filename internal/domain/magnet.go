package domain

// Magnet describes a download resource for a movie.
type Magnet struct {
	Hash        string   `json:"hash"`
	Name        string   `json:"name"`
	Size        int64    `json:"size"`
	HasSubtitle bool     `json:"has_subtitle"`
	HD          bool     `json:"hd"`
	FilesCount  int      `json:"files_count"`
	CreatedAt   string   `json:"created_at"`
	Sources     []string `json:"sources,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
}
