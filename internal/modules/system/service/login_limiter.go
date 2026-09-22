package service

import "time"

// beginLoginAttempt 在慢查询和密码校验之前原子预占次数，避免并发请求同时通过检查。
// 限制作用于当前进程的“用户名 + 客户端 IP”；登录成功后清空，失败或取消则保留计数。
func (s *Service) beginLoginAttempt(key string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	if now.Sub(s.loginLastSweep) >= loginSweepInterval {
		for k, item := range s.loginAttempts {
			if !now.Before(item.ResetAt) {
				delete(s.loginAttempts, k)
			}
		}
		s.loginLastSweep = now
	}
	item, exists := s.loginAttempts[key]
	if exists && !now.Before(item.ResetAt) {
		delete(s.loginAttempts, key)
		exists = false
	}
	if !exists {
		// 达到容量后拒绝新 key，不能通过驱逐其他人的记录来绕过其次数限制。
		if len(s.loginAttempts) >= maxTrackedLogins {
			return false
		}
		item = loginAttempt{ResetAt: now.Add(loginFailureWindow)}
	}
	if item.Attempts >= maxLoginAttempts {
		return false
	}
	item.Attempts++
	s.loginAttempts[key] = item
	return true
}

func (s *Service) clearLoginAttempts(key string) {
	s.loginMu.Lock()
	delete(s.loginAttempts, key)
	s.loginMu.Unlock()
}
