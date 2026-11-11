package db

type Annotation struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Code    string   `json:"code"`
	ViewIDs []string `json:"viewIds"`
	Enabled bool     `json:"enabled"`
}
