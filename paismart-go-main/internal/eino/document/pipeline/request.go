package pipeline

type ProcessRequest struct {
	FileMD5   string
	FileName  string
	UserID    uint
	OrgTag    string
	IsPublic  bool
	Bucket    string
	ObjectKey string
	MimeType  string
}
