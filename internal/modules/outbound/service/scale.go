package service

import (
	"fmt"

	"gowms/internal/pkg/errcode"
)

func (s *Service) checkDetailCount(count int) error {
	if count > s.limits.MaxShipmentDetails {
		return errcode.New(errcode.ShipDetailScaleExceeded.Code,
			fmt.Sprintf("单张出库单最多 %d 条明细，请拆单后重试", s.limits.MaxShipmentDetails))
	}
	return nil
}
