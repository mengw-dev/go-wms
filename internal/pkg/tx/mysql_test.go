package tx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
	"gowms/internal/testutil"
)

func retryDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenIsolatedMySQL(t, os.Getenv("WMS_TEST_DSN"))
	if err := db.Exec("CREATE TABLE retry_rows (id INT PRIMARY KEY, n INT NOT NULL) ENGINE=InnoDB").Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 50; i++ {
		if err := db.Exec("INSERT INTO retry_rows VALUES (?, 0)", i).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func rowValue(t *testing.T, db *gorm.DB, id int) int {
	t.Helper()
	var value int
	if err := db.Raw("SELECT n FROM retry_rows WHERE id = ?", id).Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	return value
}

// holder 修改更多行，使 InnoDB 选择业务事务作为较轻的死锁牺牲者。
// 两个事务分别持有行 1/2 再交叉申请；不依赖 sleep 推测加锁时机。
func realDeadlock(ctx context.Context, db, victim *gorm.DB) error {
	holder := db.WithContext(ctx).Begin()
	if holder.Error != nil {
		return holder.Error
	}
	defer holder.Rollback()
	if err := holder.Exec("UPDATE retry_rows SET n = n + 1 WHERE id = 2 OR id >= 10").Error; err != nil {
		return err
	}
	if err := victim.Exec("UPDATE retry_rows SET n = n + 1 WHERE id IN (1, 3)").Error; err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		if err := holder.Exec("UPDATE retry_rows SET n = n + 1 WHERE id = 1").Error; err != nil {
			done <- err
			return
		}
		done <- holder.Commit().Error
	}()
	err := victim.Exec("UPDATE retry_rows SET n = n + 1 WHERE id = 2").Error
	holderErr := <-done // 所有数据库调用有 context 截止时间，返回前回收 goroutine。
	if holderErr != nil {
		// 这是 fixture 构造失败，不能误当成待测业务事务的 1213。
		return errors.New("unexpected holder failure instead of victim rollback: " + holderErr.Error())
	}
	return err
}

func assertBusyResponse(t *testing.T, err error, mysqlNumber uint16) {
	t.Helper()
	var driverErr *mysql.MySQLError
	if !errors.As(err, &driverErr) || driverErr.Number != mysqlNumber {
		t.Fatalf("driver cause lost: %v", err)
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/operation", nil)
	response.Fail(c, err)
	var body response.Body
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if w.Code != 503 || body.Code != errcode.TransactionBusy.Code || body.Msg != errcode.TransactionBusy.Msg {
		t.Fatalf("unexpected public response: %d %s", w.Code, w.Body.String())
	}
}

func TestRealDeadlockRetriesFreshTransactionAndRollsBack(t *testing.T) {
	db := retryDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	attempts := 0
	err := New(db).TxRetry(ctx, MaxTxRetry, func(q *gorm.DB) error {
		attempts++
		if attempts == 1 {
			err := realDeadlock(ctx, db, q)
			var driverErr *mysql.MySQLError
			if !errors.As(err, &driverErr) || driverErr.Number != 1213 {
				t.Fatalf("real deadlock not reached: %v", err)
			}
			return err
		}
		if rowValue(t, q, 2) != 1 { // 新事务必须看到 holder 已提交的进度。
			t.Fatal("retry used stale state")
		}
		return q.Exec("UPDATE retry_rows SET n = n + 1 WHERE id IN (1, 2, 3)").Error
	})
	if err != nil || attempts != 2 || rowValue(t, db, 3) != 1 || rowValue(t, db, 2) != 2 {
		t.Fatalf("fresh retry failed: attempts=%d err=%v", attempts, err)
	}
}

func TestRealDeadlockExhaustionReturns503AndKeepsRollback(t *testing.T) {
	db := retryDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	attempts := 0
	err := New(db).TxRetry(ctx, MaxTxRetry, func(q *gorm.DB) error {
		attempts++
		return realDeadlock(ctx, db, q)
	})
	assertBusyResponse(t, err, 1213)
	if attempts != MaxTxRetry || rowValue(t, db, 3) != 0 {
		t.Fatalf("exhausted transaction committed or wrong attempts: %d", attempts)
	}
}

func TestRealLockTimeoutDoesNotRetryAndRollsBackWholeTransaction(t *testing.T) {
	db := retryDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	holder := db.WithContext(ctx).Begin()
	defer holder.Rollback()
	if err := holder.Exec("UPDATE retry_rows SET n = 7 WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	attempts := 0
	err := New(db).TxRetry(ctx, MaxTxRetry, func(q *gorm.DB) error {
		attempts++
		if err := q.Exec("SET SESSION innodb_lock_wait_timeout = 1").Error; err != nil {
			return err
		}
		if err := q.Exec("UPDATE retry_rows SET n = 9 WHERE id = 3").Error; err != nil {
			return err
		}
		return q.Exec("UPDATE retry_rows SET n = 8 WHERE id = 1").Error
	})
	assertBusyResponse(t, err, 1205)
	if attempts != 1 || rowValue(t, db, 3) != 0 {
		t.Fatalf("1205 retried or earlier writes survived: attempts=%d", attempts)
	}
}

func TestBusinessConflictAndCancellationKeepRollback(t *testing.T) {
	db := retryDatabase(t)
	attempts := 0
	err := New(db).TxRetry(context.Background(), MaxTxRetry, func(q *gorm.DB) error {
		attempts++
		if err := q.Exec("UPDATE retry_rows SET n = n + 1 WHERE id = 3").Error; err != nil {
			return err
		}
		return errcode.ShipOrderVersionBad
	})
	if !errors.Is(err, errcode.ShipOrderVersionBad) || attempts != MaxTxRetry || rowValue(t, db, 3) != 0 {
		t.Fatalf("business conflict semantics changed: %v (%d)", err, attempts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	attempts = 0
	err = New(db).TxRetry(ctx, MaxTxRetry, func(*gorm.DB) error {
		attempts++
		cancel()
		return errcode.Conflict
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled backoff started another transaction: %v (%d)", err, attempts)
	}
}

func TestRetryWindowDoesNotRestartSlowTransaction(t *testing.T) {
	db := retryDatabase(t)
	attempts := 0
	err := New(db).TxRetry(context.Background(), MaxTxRetry, func(*gorm.DB) error {
		attempts++
		time.Sleep(retryWindow + 10*time.Millisecond)
		return errcode.Conflict
	})
	if !errors.Is(err, errcode.Conflict) || attempts != 1 {
		t.Fatalf("retry window exceeded: %v (%d)", err, attempts)
	}
}
