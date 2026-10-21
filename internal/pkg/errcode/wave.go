package errcode

import "net/http"

var (
	WaveNotFound       = NewHTTP(50401, "波次不存在", http.StatusNotFound)
	WaveOrderInvalid   = NewHTTP(50402, "仅可选择同仓、已分配、未开工且未加入波次的订单", http.StatusConflict)
	WaveBudgetExceeded = NewHTTP(50403, "每个波次最多50单、1000个拣货任务", http.StatusBadRequest)
	WaveStatusWrong    = NewHTTP(50404, "波次当前状态不允许此操作", http.StatusConflict)
	WaveClaimRequired  = NewHTTP(50405, "请先领取波次；其他人持有时需等待租约到期", http.StatusConflict)
	WaveScanMismatch   = NewHTTP(50406, "货品、来源库位、批次或目标槽位核对不一致", http.StatusBadRequest)
	WaveQuantityWrong  = NewHTTP(50407, "数量必须为正且不能超过待拣或待分货数量", http.StatusBadRequest)
	WaveOrderLocked    = NewHTTP(50408, "该任务属于播种波次，请到波次工作台操作；未开工可先撤销波次", http.StatusConflict)
	WaveNotSorted      = NewHTTP(50409, "该订单尚未完成分货，不能发货", http.StatusConflict)
)
