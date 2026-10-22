package dto

type ScanQuery struct {
	Code string `form:"code" binding:"required,max=64"`
}
type ScanSKU struct {
	ID      int64  `json:"id,string"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Barcode string `json:"barcode"`
}
