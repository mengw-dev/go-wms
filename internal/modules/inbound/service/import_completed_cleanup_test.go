package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gowms/internal/modules/inbound/model"
)

func TestCompletedImportCleanupRecoversAndRetries(t *testing.T) {
	s, db, ctx := importFixture(t)
	paths := make(map[model.ImportTaskStatus]string)
	for _, status := range []model.ImportTaskStatus{model.ImportCompleted, model.ImportPending, model.ImportProcessing, model.ImportFailed} {
		path := filepath.Join(s.uploadDir, "IMP-"+string(status)+".xlsx")
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths[status] = path
		if err := db.WithContext(ctx).Create(&model.ImportTask{TaskID: "IMP-" + string(status), Status: status, FilePath: path}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// 成功状态刚提交也可补偿；失败文件仍遵守保留期，待处理文件绝不能删。
	for range 2 {
		if err := s.CleanupExpiredImportFiles(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	for status, path := range paths {
		_, err := os.Stat(path)
		if status == model.ImportCompleted {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("completed file survived: %v", err)
			}
		} else if err != nil {
			t.Fatalf("active/retained %s file removed: %v", status, err)
		}
	}
	completed, err := s.GetImport(ctx, "IMP-"+string(model.ImportCompleted))
	if err != nil || completed.FilePath != "" {
		t.Fatalf("completed path not cleared: %+v %v", completed, err)
	}

	// 非空目录模拟 os.Remove 暂时失败；下轮障碍消失后必须重试。
	blocked := filepath.Join(s.uploadDir, "IMP-blocked.xlsx")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(blocked, "busy")
	if err := os.WriteFile(child, []byte("busy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&model.ImportTask{TaskID: "IMP-retry", Status: model.ImportCompleted, FilePath: blocked}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupExpiredImportFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	retry, err := s.GetImport(ctx, "IMP-retry")
	if err != nil || retry.FilePath != blocked {
		t.Fatalf("failed deletion lost retry path: %+v %v", retry, err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupExpiredImportFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	retry, err = s.GetImport(ctx, "IMP-retry")
	if err != nil || retry.FilePath != "" {
		t.Fatalf("retry did not clear path: %+v %v", retry, err)
	}
	// 文件已不存在（删除后进程退出），同样可以清理持久化路径。
	if err := db.WithContext(ctx).Model(&model.ImportTask{}).Where("task_id = ?", retry.TaskID).Updates(map[string]any{"file_path": blocked, "updated_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupExpiredImportFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	retry, err = s.GetImport(ctx, retry.TaskID)
	if err != nil || retry.FilePath != "" {
		t.Fatalf("absent file not reconciled: %+v %v", retry, err)
	}
}
