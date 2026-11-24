package controller

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
)

const catalogTokenUnit = 1_000_000

type catalogUnitRate struct {
	metric    string
	quantity  int
	price     decimal.Decimal
	tokenType string
	condition string
	boundary  string
}

type modelCatalogSchedule struct {
	source string
	rates  []catalogUnitRate
}

func getModelCatalogSchedule(name string, protocols []string, taskRoutes bool, routes map[string][]model.ModelCatalogRoute) modelCatalogSchedule {
	billingName := helper.ResolveBillingModelName(name)
	mode := billing_setting.GetBillingMode(billingName)
	expression, _ := billing_setting.GetBillingExpr(billingName)
	fixedPrice, hasFixedPrice := ratio_setting.GetModelPrice(billingName, false)
	input, hasRatio, _ := ratio_setting.GetModelRatio(billingName)
	output := ratio_setting.GetCompletionRatio(billingName)
	cache, _ := ratio_setting.GetCacheRatio(billingName)
	cacheWrite, _ := ratio_setting.GetCreateCacheRatio(billingName)
	schedule := modelCatalogSchedule{source: fmt.Sprintf("%s\x00%s\x00%s", billingName, mode, expression)}
	if mode != billing_setting.BillingModeTieredExpr || taskRoutes {
		schedule.source += fmt.Sprintf("\x00%t:%g\x00%t:%g:%g:%g:%g:%g", hasFixedPrice, fixedPrice, hasRatio, input, output, cache, cacheWrite, common.QuotaPerUnit)
	}
	if taskRoutes {
		// A task may select a plugin override or derive the charge from media
		// facts whose units differ from the public catalog metrics.
		var sources []string
		hasOverride := false
		for _, groupRoutes := range routes {
			for _, route := range groupRoutes {
				if route.PluginKey == "" {
					continue
				}
				if expr, _, ok := billing_setting.ResolveTaskBillingExpr(route.PluginKey, name, ""); ok {
					sources = append(sources, route.PluginKey+"\x00"+expr)
					hasOverride = true
				}
			}
		}
		slices.Sort(sources)
		schedule.source += "\x00task\x00" + strings.Join(slices.Compact(sources), "\x00")
		if !hasOverride && mode != billing_setting.BillingModeTieredExpr && hasFixedPrice && finiteCatalogPrice(fixedPrice) {
			schedule.rates = []catalogUnitRate{{metric: "task", quantity: 1, price: decimal.NewFromFloat(fixedPrice), condition: "pricing", boundary: "base_before_request_multipliers"}}
		}
		return schedule
	}

	image := slices.Contains(protocols, "image_generations") || slices.Contains(protocols, "image_edits")
	if mode == billing_setting.BillingModeTieredExpr {
		schedule.rates = catalogExpressionRates(expression)
		return schedule
	}
	if hasFixedPrice && finiteCatalogPrice(fixedPrice) {
		metric := "task"
		if image {
			// These legacy model families apply request-dependent size/quality
			// multipliers to their fixed quote.
			if strings.HasPrefix(strings.ToLower(name), "dall-e") {
				return schedule
			}
			metric = "image"
		}
		schedule.rates = []catalogUnitRate{{metric: metric, quantity: 1, price: decimal.NewFromFloat(fixedPrice)}}
		return schedule
	}
	if hasFixedPrice || !hasRatio || image || !finiteCatalogPrice(input) || !finiteCatalogPrice(output) ||
		!finiteCatalogPrice(cache) || !finiteCatalogPrice(cacheWrite) || !finiteCatalogPrice(common.QuotaPerUnit) || common.QuotaPerUnit == 0 {
		return schedule
	}
	inputPrice := decimal.NewFromFloat(input).Mul(decimal.NewFromInt(catalogTokenUnit)).Div(decimal.NewFromFloat(common.QuotaPerUnit))
	schedule.rates = []catalogUnitRate{
		{metric: "input_tokens", quantity: catalogTokenUnit, price: inputPrice},
		{metric: "output_tokens", quantity: catalogTokenUnit, price: inputPrice.Mul(decimal.NewFromFloat(output))},
		{metric: "cached_input_tokens", quantity: catalogTokenUnit, price: inputPrice.Mul(decimal.NewFromFloat(cache))},
	}
	return schedule
}

func finiteCatalogPrice(amount float64) bool {
	return amount >= 0 && !math.IsInf(amount, 0) && !math.IsNaN(amount)
}

// catalogExpressionRates projects rates when all pricing branches can be expressed
// with public units and qualifiers.
func catalogExpressionRates(expression string) []catalogUnitRate {
	version, body := billingexpr.ParseExprVersion(expression)
	if version != 1 {
		return nil
	}
	tree, err := parser.Parse(body)
	if err != nil {
		return nil
	}
	root := tree.Node
	if clause, ok := root.(*ast.ConditionalNode); ok {
		comparison, ok := clause.Cond.(*ast.BinaryNode)
		if !ok || comparison.Operator != "<=" {
			return nil
		}
		variable, ok := comparison.Left.(*ast.IdentifierNode)
		limit, hasLimit := comparison.Right.(*ast.IntegerNode)
		if !ok || variable.Value != "len" || !hasLimit || limit.Value < 0 {
			return nil
		}
		lower, upper := catalogRateLeaf(clause.Exp1), catalogRateLeaf(clause.Exp2)
		if len(lower) == 0 || len(upper) == 0 {
			return nil
		}
		bound := strconv.Itoa(limit.Value)
		for i := range lower {
			lower[i].condition, lower[i].boundary = "context_tokens_lte", bound
		}
		for i := range upper {
			upper[i].condition, upper[i].boundary = "context_tokens_gt", bound
		}
		return append(lower, upper...)
	}
	return catalogRateLeaf(root)
}

func catalogRateLeaf(root ast.Node) []catalogUnitRate {
	metric := "task"
	if product, ok := root.(*ast.BinaryNode); ok && product.Operator == "*" {
		if identifier, ok := product.Right.(*ast.IdentifierNode); ok && identifier.Value == "image_count" {
			root, metric = product.Left, "image"
		} else if identifier, ok := product.Left.(*ast.IdentifierNode); ok && identifier.Value == "image_count" {
			root, metric = product.Right, "image"
		}
	}
	if tier, ok := root.(*ast.CallNode); ok {
		if function, ok := tier.Callee.(*ast.IdentifierNode); !ok || function.Value != "tier" || len(tier.Arguments) != 2 {
			return nil
		}
		if _, ok := tier.Arguments[0].(*ast.StringNode); !ok {
			return nil
		}
		root = tier.Arguments[1]
		if fixed, ok := root.(*ast.CallNode); ok {
			if identifier, ok := fixed.Callee.(*ast.IdentifierNode); ok && identifier.Value == "fixed" && len(fixed.Arguments) == 1 {
				if amount, ok := catalogPriceLiteral(fixed.Arguments[0]); ok {
					return []catalogUnitRate{{metric: metric, quantity: 1, price: amount}}
				}
			}
			return nil
		}
	}
	if metric == "image" {
		return nil
	}
	return catalogTokenRates(root)
}

func catalogTokenRates(node ast.Node) []catalogUnitRate {
	if binary, ok := node.(*ast.BinaryNode); ok && binary.Operator == "+" {
		left, right := catalogTokenRates(binary.Left), catalogTokenRates(binary.Right)
		if len(left) == 0 || len(right) == 0 {
			return nil
		}
		for _, rate := range right {
			idx := slices.IndexFunc(left, func(other catalogUnitRate) bool {
				return other.metric == rate.metric && other.tokenType == rate.tokenType
			})
			if idx < 0 {
				left = append(left, rate)
			} else {
				left[idx].price = left[idx].price.Add(rate.price)
			}
		}
		return left
	}
	amount := decimal.NewFromInt(1)
	if binary, ok := node.(*ast.BinaryNode); ok && binary.Operator == "*" {
		var ok bool
		amount, ok = catalogPriceLiteral(binary.Right)
		node = binary.Left
		if !ok {
			amount, ok = catalogPriceLiteral(binary.Left)
			node = binary.Right
		}
		if !ok {
			return nil
		}
	}
	quantity, ok := node.(*ast.IdentifierNode)
	if !ok {
		return nil
	}
	metric, tokenType := "", ""
	switch quantity.Value {
	case "p":
		metric = "input_tokens"
	case "c":
		metric = "output_tokens"
	case "cr":
		metric = "cached_input_tokens"
	case "cc", "cc1h", "cc12h":
		metric, tokenType = "input_tokens", map[string]string{"cc": "cache_write", "cc1h": "cache_write_1h", "cc12h": "cache_write_12h"}[quantity.Value]
	case "img":
		metric, tokenType = "input_tokens", "image"
	case "img_cr":
		metric, tokenType = "cached_input_tokens", "image"
	case "img_o":
		metric, tokenType = "output_tokens", "image"
	case "ai":
		metric, tokenType = "input_tokens", "audio"
	case "ao":
		metric, tokenType = "output_tokens", "audio"
	default:
		return nil
	}
	return []catalogUnitRate{{metric: metric, quantity: catalogTokenUnit, price: amount, tokenType: tokenType}}
}

func catalogPriceLiteral(node ast.Node) (decimal.Decimal, bool) {
	switch value := node.(type) {
	case *ast.IntegerNode:
		return decimal.NewFromInt(int64(value.Value)), value.Value >= 0
	case *ast.FloatNode:
		if finiteCatalogPrice(value.Value) {
			return decimal.NewFromFloat(value.Value), true
		}
	}
	return decimal.Zero, false
}
