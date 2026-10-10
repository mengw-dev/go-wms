package service

import (
	"context"
	"errors"
	"sort"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// externalOrderContent 外部创建请求的内容快照。
// 规范化规则：
//   - 仓库：按编码解析为仓库 ID；
//   - 明细：按「SKU 编码 + 数量」表示，同一编码只允许出现一次，明细顺序不影响相等判断；
//   - 备注：精确字符串比较（含空串，不裁剪空白）。
//
// 只包含创建期不可变字段；订单后续状态、已拣数量、更新时间一律不参与比较。
type externalOrderContent struct {
	warehouseID int64
	remark      string
	// details 按 SKU 编码升序排列，后续创建与回放核对共用同一份。
	details []externalOrderDetail
}

// externalOrderDetail 规范化后的创建明细：SKU 编码 + 解析出的 SKU ID + 数量。
type externalOrderDetail struct {
	skuCode string
	skuID   int64
	qty     int
}

// CreateExternal 按仓库/货品编码创建外部推送的出库单，并以业务单号幂等：
// 同租户同号同内容返回原单（idempotent=true），同租户同号不同内容返回 409，
// 不同租户同号互不影响，并发同号创建仍只产生一张订单（数据库唯一键兜底）。
//
// 内容核对覆盖两条路径：首次查询命中、并发撞唯一键后回查，统一走 replayExternalOrder。
// 业务单号去重跟随订单生命周期保留，不依赖 7 天清理的请求级幂等记录。
func (s *Service) CreateExternal(ctx context.Context, req *dto.ExternalCreateOrderReq, operator string) (*model.ShipmentOrder, bool, error) {
	// 非 HTTP 调用同样只操作一个租户，不能将平台 0 当作跨租户通行证。
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))

	// 先解析并规范化内容：命中已存在订单时也要用同一份内容核对，保证两条路径口径一致。
	content, err := s.resolveExternalContent(ctx, req)
	if err != nil {
		return nil, false, err
	}

	existing, err := s.repo.GetOrderByBizNo(ctx, s.tm.DB(), req.BizOrderNo)
	if err == nil {
		return s.replayExternalOrder(ctx, existing, content)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	items := make([]dto.OrderDetailItem, 0, len(content.details))
	for _, d := range content.details {
		items = append(items, dto.OrderDetailItem{SKUID: d.skuID, ExpectedQty: d.qty})
	}
	order, err := s.Create(ctx, &dto.CreateOrderReq{
		WarehouseID: content.warehouseID,
		BizOrderNo:  req.BizOrderNo,
		Remark:      content.remark,
		Details:     items,
	}, operator)
	if errors.Is(err, errcode.BizOrderDuplicate) {
		// 路径 2：并发请求撞唯一键后回查，同样核对内容。
		existing, getErr := s.repo.GetOrderByBizNo(ctx, s.tm.DB(), req.BizOrderNo)
		if getErr != nil {
			return nil, false, getErr
		}
		return s.replayExternalOrder(ctx, existing, content)
	}
	if err != nil {
		return nil, false, err
	}
	return order, false, nil
}

// resolveExternalContent 解析并规范化创建内容：仓库按编码解析为仓库 ID，
// 明细拒绝重复 SKU 编码（与手工建单口径一致）并按编码升序排列。
func (s *Service) resolveExternalContent(ctx context.Context, req *dto.ExternalCreateOrderReq) (*externalOrderContent, error) {
	warehouse, err := s.basic.GetWarehouseByCode(ctx, req.WarehouseCode)
	if err != nil {
		return nil, err
	}
	content := &externalOrderContent{warehouseID: warehouse.ID, remark: req.Remark}
	seen := make(map[string]struct{}, len(req.Details))
	for _, item := range req.Details {
		sku, err := s.basic.GetSKUByCode(ctx, item.SKUCode)
		if err != nil {
			return nil, err
		}
		// 统一用主数据中的规范编码做去重与创建快照：数据库排序规则大小写不敏感，
		// 若保留请求原始书写，同码不同大小写会在重试时被误判为内容不一致（409），
		// 重复 SKU 检测也会漏掉大小写变体。仓库按 ID 比较，不受编码书写影响。
		code := sku.Code
		if _, dup := seen[code]; dup {
			return nil, errcode.ShipDetailDuplicateSKU
		}
		seen[code] = struct{}{}
		content.details = append(content.details, externalOrderDetail{skuCode: code, skuID: sku.ID, qty: item.ExpectedQty})
	}
	sort.Slice(content.details, func(i, j int) bool { return content.details[i].skuCode < content.details[j].skuCode })
	return content, nil
}

// replayExternalOrder 核对已存在订单与本次创建内容：
// 一致返回原单（幂等回放）；不一致返回 409；
// 历史明细缺少 SKU 编码快照、无法可靠重建原始内容时报告待人工核对，
// 绝不把无法证明一致的历史订单当成重复成功放行。
func (s *Service) replayExternalOrder(ctx context.Context, existing *model.ShipmentOrder, content *externalOrderContent) (*model.ShipmentOrder, bool, error) {
	if existing.WarehouseID != content.warehouseID || existing.Remark != content.remark {
		return nil, false, errcode.BizOrderContentConflict
	}
	// 明细的 sku_code/expected_qty 是创建时写入的不可变快照，创建后不被任何路径更新，
	// 因此历史订单与新建订单都按同一份持久化内容核对。
	rows, err := s.repo.ListDetails(s.tm.DB().WithContext(ctx), existing.ID)
	if err != nil {
		return nil, false, err
	}
	if len(rows) != len(content.details) {
		return nil, false, errcode.BizOrderContentConflict
	}
	byCode := make(map[string]int, len(rows))
	for _, row := range rows {
		if row.SKUCode == "" {
			return nil, false, errcode.BizOrderContentUnknown
		}
		byCode[row.SKUCode] = row.ExpectedQty
	}
	for _, d := range content.details {
		if byCode[d.skuCode] != d.qty {
			return nil, false, errcode.BizOrderContentConflict
		}
	}
	return existing, true, nil
}
