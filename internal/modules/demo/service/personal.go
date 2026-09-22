package service

import (
	"strings"

	"gowms/internal/pkg/errcode"
)

// 持久体验账号（user1..userN）：与演示账号同构——一人一租户、共用固定密码、数据互不影响；
// 区别是数据长期保留（不参与演示重置、无会话锁），且登录页由访客自行挑选账号。
// 账号与角色由 bootstrap.SeedPersonalAccounts 幂等创建，这里只负责登录页的公开查询与领取。

// PersonalAccountInfo 持久体验账号信息（登录页"个人空间"入口）。
type PersonalAccountInfo struct {
	TenantID int64  `json:"tenant_id,string"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
}

// PersonalEnabled 持久体验账号是否开放（登录页据此决定是否显示"个人空间"入口）。
func (s *Service) PersonalEnabled() bool {
	return s.cfg != nil && s.cfg.Personal.Enabled && s.cfg.Personal.Instances > 0
}

// PersonalAccounts 返回全部可选的持久账号（供访客自行挑选；密码不在列表里下发，领取时才返回）。
func (s *Service) PersonalAccounts() ([]PersonalAccountInfo, error) {
	if !s.PersonalEnabled() {
		return nil, errcode.PersonalDisabled
	}
	list := make([]PersonalAccountInfo, 0, s.cfg.Personal.Instances)
	for i := 1; i <= s.cfg.Personal.Instances; i++ {
		list = append(list, PersonalAccountInfo{
			TenantID: s.cfg.Personal.AccountTenantID(i),
			Username: s.cfg.Personal.AccountUsername(i),
			Nickname: s.cfg.Personal.AccountNickname(i),
		})
	}
	return list, nil
}

// ClaimPersonalAccount 校验用户名属于配置的持久账号后返回登录凭据（公开固定密码），
// 供登录页选好账号后直接进入。数据长期保留：不重置、不设置会话锁。
func (s *Service) ClaimPersonalAccount(username string) (*PersonalAccountInfo, error) {
	if !s.PersonalEnabled() {
		return nil, errcode.PersonalDisabled
	}
	name := strings.TrimSpace(username)
	index := s.cfg.Personal.AccountIndexByUsername(name)
	if index == 0 {
		return nil, errcode.PersonalNotFound
	}
	return &PersonalAccountInfo{
		TenantID: s.cfg.Personal.AccountTenantID(index),
		Username: s.cfg.Personal.AccountUsername(index),
		Nickname: s.cfg.Personal.AccountNickname(index),
		Password: s.cfg.Personal.Password,
	}, nil
}
