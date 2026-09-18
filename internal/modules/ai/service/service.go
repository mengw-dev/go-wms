package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"gowms/internal/modules/ai/client"
	"gowms/internal/modules/ai/repository"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
)

// 检索增强常量：控制快照数据量与提示词长度。
const (
	topSKULimit           = 10 // 库存 Top N
	lowAvailableThreshold = 50 // 可用 <= 该值视为低可用
	lowAvailableLimit     = 10
	skuCatalogLimit       = 200 // SKU 目录上限（防止提示词过长）
	matchSKULimit         = 3   // 问题中最多匹配 3 个 SKU 附明细
	detailLimit           = 20  // 单个 SKU 明细行上限
	llmTemperature        = 0.2
	llmMaxTokens          = 2048
)

const systemPrompt = `你是 WMS 仓储管理系统的库存智能助手。请严格遵守：
1. 只基于下方提供的【库存数据快照】回答，快照来自系统数据库的实时检索结果。
2. 快照里没有的信息，明确说"根据当前数据无法回答"，严禁编造或推算任何数字。
3. 用中文简洁、专业地回答；引用数字时保持原值，不要自行加减。
4. 若问题与库存无关，礼貌说明你只能回答库存相关问题，并给一个库存相关的提问示例。`

// Service AI 库存问答：后端检索真实库存拼入提示词，调用 LLM 生成回答。
// 单轮无状态（历史由前端保存）；严禁让 LLM 生成 SQL。
type Service struct {
	cfg  config.AIConfig
	repo *repository.Repository
	rdb  *redis.Client
	llm  *client.Client
}

func New(cfg config.AIConfig, db *gorm.DB, rdb *redis.Client) *Service {
	return &Service{
		cfg:  cfg,
		repo: repository.New(db),
		rdb:  rdb,
		llm:  client.New(cfg.APIKey, cfg.BaseURL, cfg.TimeoutSeconds),
	}
}

// Chat 处理一次提问：限流 → 检索库存快照 → 拼 Prompt → 主模型调用（失败降级备用）。
func (s *Service) Chat(ctx context.Context, userID int64, question string) (string, error) {
	if s.cfg.APIKey == "" {
		return "", errcode.AIKeyMissing
	}
	if !s.allowRate(ctx, userID) {
		return "", errcode.AIRateLimited
	}

	snapshot, err := s.buildSnapshot(ctx, question)
	if err != nil {
		return "", err
	}

	messages := []client.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "【库存数据快照】\n" + snapshot + "\n\n【用户问题】\n" + question},
	}

	answer, err := s.llm.Chat(ctx, s.cfg.Model, messages, llmTemperature, llmMaxTokens)
	if err == nil {
		return answer, nil
	}
	// 主模型失败（如 glm-4.7-flash 公共池拥堵 1305/429）→ 降级备用模型重试一次
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && (apiErr.HTTPStatus == 401 || apiErr.HTTPStatus == 403) {
		// API Key 无效时备用模型同样会失败，直接返回并记录
		log.WithContext(ctx).Error("ai chat primary model auth failed", "err", err)
		return "", errcode.AIServiceUnavailable
	}
	if s.cfg.BackupModel == "" || s.cfg.BackupModel == s.cfg.Model {
		log.WithContext(ctx).Error("ai chat failed and no backup model", "err", err)
		return "", errcode.AIServiceUnavailable
	}
	log.WithContext(ctx).Warn("ai chat primary model failed, fallback to backup", "err", err)
	answer, backupErr := s.llm.Chat(ctx, s.cfg.BackupModel, messages, llmTemperature, llmMaxTokens)
	if backupErr == nil {
		return answer, nil
	}
	log.WithContext(ctx).Error("ai chat backup model also failed", "primary_err", err, "backup_err", backupErr)
	return "", errcode.AIServiceUnavailable
}

// allowRate 每用户每分钟限流（Redis INCR + EXPIRE）；Redis 不可用时降级放行。
func (s *Service) allowRate(ctx context.Context, userID int64) bool {
	if s.rdb == nil {
		return true
	}
	key := fmt.Sprintf("wms:ai:rl:%d", userID)
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		// Redis 故障：降级不限流（与 bootstrap 的 Redis 降级策略一致）
		return true
	}
	if n == 1 {
		// 仅在键首次创建时设置 TTL；设置失败则删除键，避免留下永不过期的计数
		if err := s.rdb.Expire(ctx, key, time.Minute).Err(); err != nil {
			_ = s.rdb.Del(ctx, key).Err()
		}
	}
	return n <= int64(s.cfg.RateLimitPerMin)
}

// buildSnapshot 检索真实库存并格式化为紧凑文本（供 LLM 阅读）。
func (s *Service) buildSnapshot(ctx context.Context, question string) (string, error) {
	ov, err := s.repo.GetOverview(ctx)
	if err != nil {
		return "", err
	}
	warehouseStock, err := s.repo.ListWarehouseStock(ctx)
	if err != nil {
		return "", err
	}
	topSKU, err := s.repo.TopSKUByStock(ctx, topSKULimit)
	if err != nil {
		return "", err
	}
	lowSKU, err := s.repo.LowAvailableSKU(ctx, lowAvailableThreshold, lowAvailableLimit)
	if err != nil {
		return "", err
	}
	catalog, err := s.repo.ListSKUBrief(ctx, skuCatalogLimit)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "- SKU 总数: %d（库存记录 %d 条）\n", ov.SKUTotal, ov.InventoryRows)
	fmt.Fprintf(&b, "- 库存总量: %d，可用总量: %d，已分配总量: %d\n", ov.StockTotal, ov.AvailableTotal, ov.AllocatedTotal)

	b.WriteString("- 各仓库库存分布:\n")
	if len(warehouseStock) == 0 {
		b.WriteString("  （暂无库存）\n")
	}
	for _, w := range warehouseStock {
		fmt.Fprintf(&b, "  * %s(%s): 库存 %d，可用 %d\n", w.WarehouseName, w.WarehouseCode, w.StockQty, w.AvailableQty)
	}

	b.WriteString("- 库存最多的前 10 个 SKU（按库存总量降序）:\n")
	if len(topSKU) == 0 {
		b.WriteString("  （暂无库存）\n")
	}
	for i, sk := range topSKU {
		fmt.Fprintf(&b, "  %d. %s %s: 库存 %d，可用 %d\n", i+1, sk.SKUCode, sk.SKUName, sk.StockQty, sk.AvailableQty)
	}

	b.WriteString(fmt.Sprintf("- 低可用库存（可用 <= %d，升序，最多 %d 条）:\n", lowAvailableThreshold, lowAvailableLimit))
	if len(lowSKU) == 0 {
		b.WriteString("  （无低可用库存）\n")
	}
	for _, sk := range lowSKU {
		fmt.Fprintf(&b, "  * %s %s: 可用 %d（库存 %d）\n", sk.SKUCode, sk.SKUName, sk.AvailableQty, sk.StockQty)
	}

	b.WriteString(fmt.Sprintf("- SKU 目录（编码 | 名称 | 规格 | 单位，最多 %d 条）:\n", skuCatalogLimit))
	if len(catalog) == 0 {
		b.WriteString("  （暂无 SKU）\n")
	}
	for _, sk := range catalog {
		fmt.Fprintf(&b, "  %s | %s | %s | %s\n", sk.Code, sk.Name, sk.Spec, sk.Unit)
	}

	// 问题中提到的 SKU（编码/名称/条码包含匹配）附上库位+批次明细
	if details := s.matchedSKUDetails(ctx, question, catalog); len(details) > 0 {
		b.WriteString("- 用户问题中提到的 SKU 库存明细（仓库/库位/批次/可用量）:\n")
		b.WriteString(details)
	}
	return b.String(), nil
}

// matchedSKUDetails 从问题中匹配 SKU（最多 matchSKULimit 个），检索其库存明细。
func (s *Service) matchedSKUDetails(ctx context.Context, question string, catalog []repository.SKUBrief) string {
	matched := make([]repository.SKUBrief, 0, matchSKULimit)
	for _, sk := range catalog {
		if strings.Contains(question, sk.Code) ||
			(sk.Name != "" && strings.Contains(question, sk.Name)) ||
			(sk.Barcode != "" && strings.Contains(question, sk.Barcode)) {
			matched = append(matched, sk)
			if len(matched) >= matchSKULimit {
				break
			}
		}
	}
	var b strings.Builder
	for _, sk := range matched {
		details, err := s.repo.ListInventoryDetailBySKU(ctx, sk.ID, detailLimit)
		if err != nil {
			continue // 单个 SKU 明细检索失败不阻塞整体回答
		}
		fmt.Fprintf(&b, "  %s %s:\n", sk.Code, sk.Name)
		if len(details) == 0 {
			b.WriteString("    （该 SKU 暂无库存记录）\n")
		}
		for _, d := range details {
			fmt.Fprintf(&b, "    - 仓库 %s，库位 %s，批次 %s，库存 %d，可用 %d，已分配 %d\n",
				d.WarehouseCode, d.LocationCode, d.BatchNo, d.StockQty, d.AvailableQty, d.AllocatedQty)
		}
	}
	return b.String()
}
