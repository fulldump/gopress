package api

type HttpError struct {
	Status      int    `json:"status"`
	Title       string `json:"title"`
	Code        string `json:"code,omitempty"`
	Description string `json:"description"`
	Details     any    `json:"details,omitempty"`
}

func (h HttpError) Error() string {
	return h.Description
}
