package errcode

import (
	"errors"
	"fmt"
)

// Error 统一业务错误：携带错误码与展示消息。
type Error struct {
	Code  int
	Msg   string
	cause error
}

func (e *Error) Error() string { return fmt.Sprintf("[%d] %s", e.Code, e.Msg) }

// Unwrap 保留底层错误，使 errors.Is/errors.As 可以穿透业务错误。
func (e *Error) Unwrap() error { return e.cause }

func New(code int, msg string) *Error { return &Error{Code: code, Msg: msg} }

// Wrap 将底层错误包装为稳定的业务错误，同时保留原始错误链。
func Wrap(err error, template *Error) *Error {
	if err == nil {
		return nil
	}
	if template == nil {
		template = Internal
	}
	return &Error{Code: template.Code, Msg: template.Msg, cause: err}
}

// From 将任意 error 归一化为 *Error，未知错误归为 Internal。
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internal
}

// conflictCodes 可安全重试的并发冲突错误码集合：乐观锁版本冲突 / 行竞争 / 死锁语义冲突。
var conflictCodes = map[int]struct{}{
	40900: {}, // Conflict 通用并发冲突
	40003: {}, // 入库单版本冲突
	50003: {}, // 出库单版本冲突
	50201: {}, // 分配并发冲突
	50202: {}, // 发货并发冲突
	60005: {}, // 盘点单版本冲突
}

// IsConflict 判断错误是否为并发冲突类（乐观锁失败、行竞争）——这类错误可以整事务重试。
func IsConflict(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return IsConflictCode(e.Code)
}

// IsConflictCode 判断业务码是否属于可安全重试的并发冲突。
func IsConflictCode(code int) bool {
	_, ok := conflictCodes[code]
	return ok
}

// 通用错误码
var (
	OK              = New(0, "success")
	Internal        = New(500, "系统内部错误")
	ParamError      = New(400, "参数错误")
	NotFound        = New(404, "资源不存在")
	PayloadTooLarge = New(41300, "请求内容过大")
	Unauthorized    = New(40100, "未登录或登录已过期")
	Forbidden       = New(40300, "无权限执行该操作")
	Conflict        = New(40900, "数据并发冲突，请重试")
)

// 系统/认证 10000+
var (
	UserExist                  = New(10001, "用户名已存在")
	UserOrPwdWrong             = New(10002, "用户名或密码错误")
	UserDisabled               = New(10003, "用户已被禁用")
	RoleExist                  = New(10004, "角色名已存在")
	RoleInUse                  = New(10005, "角色已绑定用户，禁止删除")
	OldPwdWrong                = New(10006, "原密码错误")
	OperRecordFail             = New(10007, "操作日志记录失败")
	PermFormatInvalid          = New(10008, "权限标识格式错误，应为 wms:module:action")
	UserIDInvalid              = New(10009, "用户不存在")
	RoleIDInvalid              = New(10010, "角色不存在")
	ModifyAdminForbidden       = New(10011, "不允许操作内置管理员账号")
	TooManyLoginAttempts       = New(10012, "登录失败次数过多，请稍后再试")
	ModifyBuiltinRoleForbidden = New(10013, "不允许修改或删除内置超级管理员角色")
	IntegrationUnauthorized    = New(10014, "集成 API Key 无效")
	IntegrationDisabled        = New(10015, "集成 API 未启用")
)

// 基础资料 20000+
var (
	WarehouseExist    = New(20001, "仓库编码已存在")
	WarehouseNotFound = New(20002, "仓库不存在")
	WarehouseHasStock = New(20003, "仓库下存在库存，禁止删除")
	LocationExist     = New(20004, "库位编码已存在")
	LocationNotFound  = New(20005, "库位不存在")
	LocationHasStock  = New(20006, "库位存在库存，只能禁用不能删除")
	SKUExist          = New(20007, "货品编码已存在")
	BarcodeExist      = New(20008, "条码已存在")
	SKUNotFound       = New(20009, "货品不存在")
	WarehouseDisabled = New(20010, "仓库已禁用")
	LocationDisabled  = New(20011, "库位已禁用")
	SKUDisabled       = New(20012, "货品已禁用")
	// 20013 在 basic service 中内联定义（单次批量生成库位数量上限）
	LocationWarehouseMismatch = New(20014, "库位不属于该仓库")
	SKUHasStock               = New(20015, "货品存在库存，禁止删除")
	WarehouseHasReferences    = New(20016, "仓库下存在库位，禁止删除")
	LocationBatchLimit        = New(20013, "单次批量生成不超过 1000 个库位")
)

// 库存 30000+
var (
	InventoryNotFound  = New(30001, "库存记录不存在")
	AvailableNotEnough = New(30201, "可用库存不足")
	AllocatedNotEnough = New(30202, "已分配库存不足")
	StockNotEnough     = New(30203, "库存总量不足")
	LocationRequired   = New(30204, "缺少上架库位")
	AdjustNotAllow     = New(30205, "库存调整数量非法")
)

// 入库 40000+
var (
	OrderNotFound        = New(40001, "入库单不存在")
	OrderStatusWrong     = New(40002, "入库单状态不允许该操作")
	OrderVersionBad      = New(40003, "入库单已被其他人操作，请刷新重试")
	ReceiveQtyOver       = New(40004, "收货数量超过剩余应收数量")
	PutawayQtyOver       = New(40005, "上架数量超过任务剩余数量")
	TaskNotFound         = New(40006, "任务不存在")
	TaskStatusWrong      = New(40007, "任务状态不允许该操作")
	BatchNoRequired      = New(40008, "缺少批次号")
	OrderNoDuplicate     = New(40009, "单号重复，请重试")
	ImportTaskNotFound   = New(40010, "导入任务不存在")
	ImportFileInvalid    = New(40011, "导入文件无效")
	ImportTemplateHeader = New(40012, "导入文件表头不符合模板")
	TaskQtyOver          = New(40013, "数量超过任务剩余数量")
	BatchNoInconsistent  = New(40014, "同一明细的批次号必须与首次收货一致")
	DetailDuplicateSKU   = New(40015, "同一货品请合并为一行明细")
)

// 出库 50000+
var (
	ShipOrderNotFound      = New(50001, "出库单不存在")
	ShipOrderStatusWrong   = New(50002, "出库单状态不允许该操作")
	ShipOrderVersionBad    = New(50003, "出库单已被其他人操作，请刷新重试")
	ShipQtyOver            = New(50004, "拣货数量超过任务剩余数量")
	BizOrderDuplicate      = New(50005, "业务订单号已存在")
	AllocConflict          = New(50201, "分配并发冲突，请重试")
	ShipConflict           = New(50202, "发货并发冲突，请重试")
	ShipShippedForbidden   = New(50006, "出库单已进入拣货/发货，禁止取消")
	ShipDetailDuplicateSKU = New(50007, "同一货品请合并为一行明细")
	PickBatchMismatch      = New(50008, "扫描的批次与任务不一致，请核对后再拣货")
	PickLocationMismatch   = New(50009, "扫描的库位与任务不一致，请核对后再拣货")
)

// 盘点 60000+
var (
	StocktakeNotFound    = New(60001, "盘点单不存在")
	StocktakeStatusWrong = New(60002, "盘点单状态不允许该操作")
	StocktakeQtyInvalid  = New(60003, "实盘数量非法")
	StocktakeNoDetail    = New(60004, "盘点单没有可盘点的库存明细")
	StocktakeVersionBad  = New(60005, "盘点单已被其他人操作，请刷新重试")
)

// 演示模式 70000+
var (
	DemoDisabled        = New(70001, "演示模式未启用")
	DemoBusy            = New(70002, "演示环境正在被使用，请稍后重试")
	DemoSessionInvalid  = New(70003, "演示会话已失效，请重新登录")
	DemoDataMissing     = New(70004, "演示基础数据不存在，请先重置演示数据")
	DemoPickTaskMissing = New(70005, "没有可拣货任务，请先执行并发出库审核分配测试")
	DemoStockNotEnough  = New(70006, "可用库存不足，请先点击“一键补货入库”或减少并发数量")
)
