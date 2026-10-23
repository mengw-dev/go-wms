package errcode

var (
	FrozenNotEnough     = New(43101, "可用量或冻结量不足；已分配库存请先处理出库异常")
	ExceptionNotAllowed = New(43102, "当前作业不可登记或处理异常，请刷新并检查任务、波次与单据状态")
	ReturnNotAllowed    = New(43103, "退货数量超出原单实发数量，或原单、批次、库位不符合退货条件")
)
