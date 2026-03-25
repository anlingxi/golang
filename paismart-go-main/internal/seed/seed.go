package seed

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"pai-smart-go/internal/repository"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/log"
)

// initSeedFiles 扫描目录下文件并通过标准上传流程导入（幂等）。
func InitSeedFiles(ctx context.Context, dir string, userRepo repository.UserRepository, uploadSvc service.UploadService) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		log.Infof("initSeedFiles: 目录 '%s' 不存在或不可用，跳过初始化导入", dir)
		return
	}

	// 选择归属用户：优先 admin，不存在则取第一个
	var ownerUserID uint
	var ownerOrg string
	if admin, err := userRepo.FindByUsername("admin"); err == nil && admin != nil {
		ownerUserID = admin.ID
		ownerOrg = admin.PrimaryOrg
	} else {
		if users, err := userRepo.FindAll(); err == nil && len(users) > 0 {
			ownerUserID = users[0].ID
			ownerOrg = users[0].PrimaryOrg
		} else {
			log.Warnf("initSeedFiles: 未找到可用用户，跳过初始化导入")
			return
		}
	}

	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		// 计算 MD5
		f, err := os.Open(path)
		if err != nil {
			log.Warnf("initSeedFiles: 打开文件失败: %s, err=%v", path, err)
			return nil
		}
		h := md5.New()
		size, copyErr := io.Copy(h, f)
		_ = f.Close()
		if copyErr != nil {
			log.Warnf("initSeedFiles: 读取文件失败: %s, err=%v", path, copyErr)
			return nil
		}
		fileMD5 := fmt.Sprintf("%x", h.Sum(nil))
		fileName := info.Name()

		// 幂等检查：已完成则跳过
		if uploaded, ferr := uploadSvc.FastUpload(ctx, fileMD5, ownerUserID); ferr == nil && uploaded {
			log.Infof("initSeedFiles: 已存在，跳过: %s (md5=%s)", fileName, fileMD5)
			return nil
		}

		// 分片上传
		const chunkSize int64 = 5 * 1024 * 1024
		totalChunks := int(math.Ceil(float64(size) / float64(chunkSize)))
		if totalChunks == 0 {
			log.Infof("initSeedFiles: 空文件跳过: %s", path)
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			log.Warnf("initSeedFiles: 重新打开文件失败: %s, err=%v", path, err)
			return nil
		}
		defer file.Close()

		for chunkIndex := 0; chunkIndex < totalChunks; chunkIndex++ {
			offset := int64(chunkIndex) * chunkSize
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				log.Warnf("initSeedFiles: Seek 失败: %s, chunk=%d, err=%v", path, chunkIndex, err)
				return nil
			}
			toRead := chunkSize
			if offset+toRead > size {
				toRead = size - offset
			}
			buf := make([]byte, toRead)
			if _, err := io.ReadFull(file, buf); err != nil {
				log.Warnf("initSeedFiles: 读取分片失败: %s, chunk=%d, err=%v", path, chunkIndex, err)
				return nil
			}
			// 适配 multipart.File
			cf := &chunkFile{Reader: bytes.NewReader(buf)}

			// 标记 is_public=true（全员可见），org 使用所有者主组织
			if _, _, err := uploadSvc.UploadChunk(ctx, fileMD5, fileName, size, chunkIndex, cf, ownerUserID, ownerOrg, true); err != nil {
				log.Warnf("initSeedFiles: 上传分片失败: %s, chunk=%d, err=%v", path, chunkIndex, err)
				return nil
			}
		}

		if _, err := uploadSvc.MergeChunks(ctx, fileMD5, fileName, ownerUserID); err != nil {
			log.Warnf("initSeedFiles: 合并失败: %s, err=%v", path, err)
			return nil
		}
		log.Infof("initSeedFiles: 导入完成并已触发向量化: %s", fileName)
		return nil
	})
	if walkErr != nil {
		log.Warnf("initSeedFiles: 遍历目录发生错误: %v", walkErr)
	}
}

// chunkFile 适配 bytes.Reader 到 multipart.File 所需接口
type chunkFile struct{ Reader *bytes.Reader }

func (c *chunkFile) Read(p []byte) (int, error)              { return c.Reader.Read(p) }
func (c *chunkFile) ReadAt(p []byte, off int64) (int, error) { return c.Reader.ReadAt(p, off) }
func (c *chunkFile) Seek(offset int64, whence int) (int64, error) {
	return c.Reader.Seek(offset, whence)
}
func (c *chunkFile) Close() error { return nil }
