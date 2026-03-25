package pipeline

import (
	"context"
	"fmt"

	documentpipeline "pai-smart-go/internal/eino/document/pipeline"
	"pai-smart-go/pkg/tasks"
)

type Processor struct {
	docPipeline *documentpipeline.Service
}

func NewProcessor(docPipeline *documentpipeline.Service) *Processor {
	return &Processor{
		docPipeline: docPipeline,
	}
}

func (p *Processor) Process(ctx context.Context, task tasks.FileProcessingTask) error {
	if p.docPipeline == nil {
		return fmt.Errorf("document pipeline is nil")
	}

	_, err := p.docPipeline.Process(ctx, documentpipeline.ProcessRequest{
		FileMD5:   task.FileMD5,
		FileName:  task.FileName,
		UserID:    task.UserID,
		OrgTag:    task.OrgTag,
		IsPublic:  task.IsPublic,
		Bucket:    task.Bucket,
		ObjectKey: task.ObjectKey,
		MimeType:  "",
	})
	return err
}
package pipeline

import (
	"context"
	"fmt"

	documentpipeline "pai-smart-go/internal/eino/document/pipeline"
)

type FileProcessingTask struct {
	FileMD5   string
	FileName  string
	UserID    uint
	OrgTag    string
	IsPublic  bool
	Bucket    string
	ObjectKey string
	MimeType  string
}

type Processor struct {
	docPipeline *documentpipeline.Service
}

func NewProcessor(docPipeline *documentpipeline.Service) *Processor {
	return &Processor{
		docPipeline: docPipeline,
	}
}

func (p *Processor) Process(ctx context.Context, task FileProcessingTask) error {
	if p.docPipeline == nil {
		return fmt.Errorf("document pipeline is nil")
	}

	_, err := p.docPipeline.Process(ctx, documentpipeline.ProcessRequest{
		FileMD5:   task.FileMD5,
		FileName:  task.FileName,
		UserID:    task.UserID,
		OrgTag:    task.OrgTag,
		IsPublic:  task.IsPublic,
		Bucket:    task.Bucket,
		ObjectKey: task.ObjectKey,
		MimeType:  task.MimeType,
	})
	return err
}
