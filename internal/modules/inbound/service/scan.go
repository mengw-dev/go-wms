package service

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// ScanOrderSKU is a read-only lookup; confirmation still uses existing command APIs.
func (s *Service) ScanOrderSKU(ctx context.Context, id int64, code string) ([]*dto.ScanSKU, error) {
	code = strings.TrimSpace(code)
	if id <= 0 || code == "" || utf8.RuneCountInString(code) > 64 || strings.ContainsFunc(code, unicode.IsControl) {
		return nil, errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	if _, err := s.repo.GetOrder(ctx, s.tm.DB(), id); errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.OrderNotFound
	} else if err != nil {
		return nil, err
	}
	return s.repo.ScanOrderSKU(s.tm.DB().WithContext(ctx), id, code)
}
