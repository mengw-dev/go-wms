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

// PutawayLocations does not reserve a location; the write path revalidates it.
func (s *Service) PutawayLocations(ctx context.Context, id int64, q *dto.PutawayLocationQuery) ([]*dto.PutawayLocation, int64, error) {
	if q == nil || id <= 0 || q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || utf8.RuneCountInString(q.Code) > 64 || utf8.RuneCountInString(q.Keyword) > 64 || strings.ContainsFunc(q.Code, unicode.IsControl) {
		return nil, 0, errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	if _, err := s.repo.GetOrder(ctx, s.tm.DB(), id); errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, errcode.OrderNotFound
	} else if err != nil {
		return nil, 0, err
	}
	return s.repo.PutawayLocations(s.tm.DB().WithContext(ctx), id, q)
}
