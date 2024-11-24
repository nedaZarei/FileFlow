package model

type FileUpload struct {
	ID         int64  `json:"id"`
	FileURL    string `json:"file_url"`
	BucketName string `json:"bucket_name"`
	ObjectName string `json:"object_name"`
}
