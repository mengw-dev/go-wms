package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gowms/internal/modules/inbound/model"
	sysmodel "gowms/internal/modules/system/model"
)

// 导入文件清理：失败文件保留到期后删除并置空路径；孤儿文件超过保护期后删除；
// 保留期内的失败文件、刚写入的孤儿与其他文件都不受影响。
func TestCleanupExpiredImportFiles(t *testing.T) {
	s, db, ctx := importFixture(t)

	var nextID int64 = 900000
	writeTask := func(taskID string, status model.ImportTaskStatus, age time.Duration) *model.ImportTask {
		t.Helper()
		nextID++
		path := filepath.Join(s.uploadDir, taskID+".xlsx")
		if err := os.WriteFile(path, []byte("workbook"), 0o600); err != nil {
			t.Fatal(err)
		}
		task := &model.ImportTask{
			Base:   sysmodel.Base{ID: nextID},
			TaskID: taskID, Status: status, FileName: taskID + ".xlsx", FilePath: path,
		}
		if err := db.WithContext(ctx).Create(task).Error; err != nil {
			t.Fatal(err)
		}
		if age > 0 {
			if err := db.WithContext(ctx).Model(&model.ImportTask{}).Where("task_id = ?", taskID).
				UpdateColumn("updated_at", time.Now().Add(-age)).Error; err != nil {
				t.Fatal(err)
			}
		}
		return task
	}

	fresh := writeTask("IMP-FRESH", model.ImportFailed, 1*time.Hour)
	expired := writeTask("IMP-EXPIRED", model.ImportFailed, 100*time.Hour)

	// 孤儿文件：没有对应任务记录。老文件应删除，刚写入的在保护期内保留。
	orphan := filepath.Join(s.uploadDir, "IMP-ORPHAN.xlsx")
	newOrphan := filepath.Join(s.uploadDir, "IMP-NEWORPHAN.xlsx")
	other := filepath.Join(s.uploadDir, "manual-note.txt")
	for _, f := range []string{orphan, newOrphan, other} {
		if err := os.WriteFile(f, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * importOrphanGrace)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}

	if err := s.CleanupExpiredImportFiles(context.Background()); err != nil {
		t.Fatal(err)
	}

	assertMissing := func(path string) {
		t.Helper()
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file should be removed: %s (%v)", path, err)
		}
	}
	assertKept := func(path string) {
		t.Helper()
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file should be kept: %s (%v)", path, err)
		}
	}

	assertKept(fresh.FilePath)      // 保留期内的失败文件不删
	assertMissing(expired.FilePath) // 超过保留期的失败文件删除
	assertMissing(orphan)           // 超过保护期的孤儿文件删除
	assertKept(newOrphan)           // 刚写入的孤儿文件保留（可能正在落库）
	assertKept(other)               // 非导入文件不动

	reloaded, err := s.GetImport(ctx, expired.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.FilePath != "" {
		t.Fatalf("expired task path not cleared: %q", reloaded.FilePath)
	}
}
