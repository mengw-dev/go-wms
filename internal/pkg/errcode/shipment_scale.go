package errcode

var (
	ShipDetailScaleExceeded = New(50020, "出库明细超过单据规模上限，请拆单后重试")
	AllocationScaleExceeded = New(50021, "库存批次分散，分配行数超过单次审核上限，请拆单或整理库存后重试")
)
