package service

import "time"

// beginLoginAttempt 在慢查询和密码校验之前原子预占次数，避免并发请求同时通过检查。
// 账号限制按解析后的租户/用户 ID + IP 计数；查询前另设 IP 总额度。
func (s *Service) beginLoginAttempt(key string) bool {
	return s.reserveLoginAttempt(key, maxLoginAttempts)
}

func (s *Service) reserveLoginAttempt(key string, limit int) bool {
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
	if item.Attempts >= limit {
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
