package dto

type PutawayLocationQuery struct {
	Code     string `form:"code" binding:"max=64"`
	Keyword  string `form:"keyword" binding:"max=64"`
	Page     int    `form:"page,default=1" binding:"gte=1"`
	PageSize int    `form:"page_size,default=100" binding:"gte=1,lte=100"`
}
type PutawayLocation struct {
	ID     int64  `json:"id,string"`
	Code   string `json:"code"`
	Status int    `json:"status"`
}
