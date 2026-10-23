package repository

import (
	"gorm.io/gorm"

	"gowms/internal/modules/inventory/model"
)

func (r *Repository) FreezeQty(db *gorm.DB, id int64, qty int, unfreeze bool) (int64, error) {
	from, to := "available_quantity", "frozen_quantity"
	if unfreeze {
		from, to = to, from
	}
	res := db.Model(&model.Inventory{}).Where("id=? AND "+from+">=? AND ?>0", id, qty, qty).Updates(map[string]any{
		from: gorm.Expr(from+"-?", qty), to: gorm.Expr(to+"+?", qty), "version": gorm.Expr("version+1"),
	})
	return res.RowsAffected, res.Error
}
