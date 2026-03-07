package review

import "time"

type Comment struct {
	ID             string    `json:"id" yaml:"id"`
	Line           int       `json:"line" yaml:"line"`
	EndLine        int       `json:"end_line,omitempty" yaml:"end_line,omitempty"`
	ContentSnippet string    `json:"content_snippet" yaml:"content_snippet"`
	Body           string    `json:"body" yaml:"body"`
	CreatedAt      time.Time `json:"created_at" yaml:"created_at"`
}

type ReviewState struct {
	File     string    `json:"file" yaml:"file"`
	Comments []Comment `json:"comments" yaml:"comments"`
}
