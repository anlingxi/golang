package pipeline

type ProcessResult struct {
	FileMD5      string
	FileName     string
	ChunkCount   int
	StoredDocIDs []string
}
