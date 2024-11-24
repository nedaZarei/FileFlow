package model

type RetryFileUpload struct {
	FileUpload
	RetryCount int    `json:"retry_count"`
	Error      string `json:"error"`
}
