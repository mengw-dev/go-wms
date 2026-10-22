package errcode

var (
	MoveNotAllowed   = NewHTTP(30010, "仅支持启用仓库内两个不同的可用库位之间移库", 400)
	MoveInsufficient = NewHTTP(30011, "可用库存不足，已分配库存不能移库，请刷新核对", 400)
)
