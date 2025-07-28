package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"coinbase-base/client"

	"os"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	client *client.CoinbaseClient
}

func NewHandlers(client *client.CoinbaseClient) *Handlers {
	return &Handlers{
		client: client,
	}
}

// GetAccounts returns all accounts
func (h *Handlers) GetAccounts(c *gin.Context) {
	accounts, err := h.client.GetAccounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch accounts",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// BuyBTC places a buy order for BTC with USDC, optionally with stop loss protection
func (h *Handlers) BuyBTC(c *gin.Context) {
	var req client.TradingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"message": err.Error(),
		})
		return
	}

	// Validate price first (required for all orders)
	if req.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Missing price",
			"message": "Price is required for market orders",
		})
		return
	}

	// Handle percentage-based order size calculation
	if req.Percentage > 0 {
		calculatedSize, err := h.client.CalculateOrderSizeByPercentage("BUY", req.Percentage, fmt.Sprintf("%.8f", req.Price))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to calculate order size by percentage",
				"message": err.Error(),
			})
			return
		}

		req.Size = calculatedSize
	}

	// Validate size
	if _, err := strconv.ParseFloat(req.Size, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid size format",
			"message": "Size must be a valid number",
		})
		return
	}

	order, err := h.client.BuyBTC(req.Size, req.Price)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to place buy order",
			"message": err.Error(),
		})
		return
	}

	response := gin.H{
		"message": "Buy order placed successfully",
		"order":   order,
	}

	c.JSON(http.StatusCreated, response)
}

// SellBTC places a sell order for BTC to USDC
func (h *Handlers) SellBTC(c *gin.Context) {
	var req client.TradingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"message": err.Error(),
		})
		return
	}

	// Validate price first (required for all orders)
	if req.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Missing price",
			"message": "Price is required for market orders",
		})
		return
	}

	// Handle percentage-based order size calculation
	if req.Percentage > 0 {
		// For SELL orders, we need the price to calculate fees correctly
		calculatedSize, err := h.client.CalculateOrderSizeByPercentage("SELL", req.Percentage, fmt.Sprintf("%.8f", req.Price))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Failed to calculate order size by percentage",
				"message": err.Error(),
			})
			return
		}

		req.Size = calculatedSize
	}

	// Validate size
	if _, err := strconv.ParseFloat(req.Size, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid size format",
			"message": "Size must be a valid number",
		})
		return
	}

	order, err := h.client.SellBTC(req.Size, req.Price)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to place sell order",
			"message": err.Error(),
		})
		return
	}

	response := gin.H{
		"message": "Sell order placed successfully",
		"order":   order,
	}

	c.JSON(http.StatusCreated, response)
}

// GetOrders returns all orders (including stop limit orders)
func (h *Handlers) GetOrders(c *gin.Context) {
	orders, err := h.client.GetOrders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch orders",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"count":  len(orders),
	})
}

// CancelOrder cancels a specific order (including stop limit orders)
func (h *Handlers) CancelOrder(c *gin.Context) {
	orderID := c.Param("order_id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Missing order ID",
			"message": "Order ID is required",
		})
		return
	}

	err := h.client.CancelOrder(orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to cancel order",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Order cancelled successfully",
		"order_id": orderID,
	})
}

// CancelAllOrders cancels all open orders
func (h *Handlers) CancelAllOrders(c *gin.Context) {
	// Get all orders first
	orders, err := h.client.GetOrders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch orders",
			"message": err.Error(),
		})
		return
	}

	// Filter for open orders
	var openOrders []client.Order
	for _, order := range orders {
		if order.Status == "OPEN" || order.Status == "PENDING" {
			openOrders = append(openOrders, order)
		}
	}

	if len(openOrders) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":         "No open orders to cancel",
			"cancelled_count": 0,
		})
		return
	}

	// Cancel each open order
	var cancelledOrders []string
	var failedOrders []string

	for _, order := range openOrders {
		err := h.client.CancelOrder(order.ID)
		if err != nil {
			failedOrders = append(failedOrders, order.ID)
		} else {
			cancelledOrders = append(cancelledOrders, order.ID)
		}
	}

	response := gin.H{
		"message":          "Cancel all orders completed",
		"cancelled_count":  len(cancelledOrders),
		"failed_count":     len(failedOrders),
		"cancelled_orders": cancelledOrders,
	}

	if len(failedOrders) > 0 {
		response["failed_orders"] = failedOrders
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// GetCandles retrieves candle data for the configured trading pair
func (h *Handlers) GetCandles(c *gin.Context) {
	// Get query parameters
	start := c.Query("start")
	end := c.Query("end")
	granularity := c.Query("granularity")
	limitStr := c.Query("limit")
	period := c.Query("period")

	// Handle preset periods
	if period != "" {
		start, end, granularity = h.getPresetPeriod(period)
		if start == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid period",
				"message": "Period must be one of: last_hour, last_day, last_week, last_month, last_year",
			})
			return
		}
	}

	// Validate required parameters
	if start == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Missing start parameter",
			"message": "Start timestamp is required (or use period parameter)",
		})
		return
	}

	if end == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Missing end parameter",
			"message": "End timestamp is required (or use period parameter)",
		})
		return
	}

	if granularity == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Missing granularity parameter",
			"message": "Granularity is required (or use period parameter)",
		})
		return
	}

	// Parse limit parameter
	limit := 0
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	// Validate that we won't exceed 350 candles
	if limit > 350 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Limit too high",
			"message": "Limit cannot exceed 350 candles (Coinbase API limit)",
		})
		return
	}

	// Validate granularity
	validGranularities := map[string]bool{
		"UNKNOWN_GRANULARITY": true,
		"ONE_MINUTE":          true,
		"FIVE_MINUTE":         true,
		"FIFTEEN_MINUTE":      true,
		"THIRTY_MINUTE":       true,
		"ONE_HOUR":            true,
		"TWO_HOUR":            true,
		"SIX_HOUR":            true,
		"ONE_DAY":             true,
	}

	if !validGranularities[granularity] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid granularity",
			"message": "Granularity must be one of: UNKNOWN_GRANULARITY, ONE_MINUTE, FIVE_MINUTE, FIFTEEN_MINUTE, THIRTY_MINUTE, ONE_HOUR, TWO_HOUR, SIX_HOUR, ONE_DAY",
		})
		return
	}

	candles, err := h.client.GetCandles(start, end, granularity, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch candles",
			"message": err.Error(),
		})
		return
	}

	response := gin.H{
		"product_id":  h.client.GetTradingPair(),
		"start":       start,
		"end":         end,
		"granularity": granularity,
		"candles":     candles,
	}

	if period != "" {
		response["period"] = period
	}

	c.JSON(http.StatusOK, response)
}

// GetMarketState retrieves current market state with bid/ask and order book
func (h *Handlers) GetMarketState(c *gin.Context) {
	// Get limit parameter (default to 10)
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid limit parameter",
			"message": "Limit must be between 1 and 100 (number of bid/ask entries)",
		})
		return
	}

	marketState, err := h.client.GetMarketState(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch market state",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"product_id":     marketState.ProductID,
		"best_bid":       marketState.BestBid,
		"best_ask":       marketState.BestAsk,
		"spread":         marketState.Spread,
		"spread_percent": marketState.SpreadPercent,
		"last_price":     marketState.LastPrice,
		"volume_24h":     marketState.Volume24h,
		"order_book":     marketState.OrderBook,
		"timestamp":      marketState.Timestamp,
		"limit":          limit,
	})
}

// getPresetPeriod returns start, end, and granularity for preset periods
// Ensures we stay within the 350 candle limit
func (h *Handlers) getPresetPeriod(period string) (string, string, string) {
	now := time.Now()

	switch period {
	case "last_hour":
		// 60 minutes = 60 candles (within limit)
		start := now.Add(-1 * time.Hour).Unix()
		return fmt.Sprintf("%d", start), fmt.Sprintf("%d", now.Unix()), "ONE_MINUTE"
	case "last_day":
		// 24 hours * 4 (15-min intervals) = 96 candles (within limit)
		start := now.AddDate(0, 0, -1).Unix()
		return fmt.Sprintf("%d", start), fmt.Sprintf("%d", now.Unix()), "FIFTEEN_MINUTE"
	case "last_week":
		// 7 days * 4 (6-hour intervals) = 28 candles (within limit)
		start := now.AddDate(0, 0, -7).Unix()
		return fmt.Sprintf("%d", start), fmt.Sprintf("%d", now.Unix()), "SIX_HOUR"
	case "last_month":
		// 30 days * 24 (hourly intervals) = 720 candles (exceeds limit)
		// Use 6-hour intervals: 30 days * 4 = 120 candles (well within limit)
		start := now.AddDate(0, -1, 0).Unix()
		return fmt.Sprintf("%d", start), fmt.Sprintf("%d", now.Unix()), "SIX_HOUR"
	case "last_year":
		// 365 days (daily intervals) = 365 candles (exceeds limit)
		// Limit to 350 days to stay within API limit
		start := now.AddDate(0, 0, -350).Unix()
		return fmt.Sprintf("%d", start), fmt.Sprintf("%d", now.Unix()), "ONE_DAY"
	default:
		return "", "", ""
	}
}

// GetPerformance returns performance statistics
func (h *Handlers) GetPerformance(c *gin.Context) {
	stats := h.client.GetPerformanceStats()
	c.JSON(http.StatusOK, gin.H{
		"performance": stats,
		"timestamp":   time.Now().Format(time.RFC3339),
	})
}

// GetSignal calculates technical indicators and checks for bearish signals
func (h *Handlers) GetSignal(c *gin.Context) {
	signal, err := h.client.GetSignal()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to calculate signal",
			"message": err.Error(),
		})
		return
	}

	// Return 200 OK if bearish signal detected, 204 No Content otherwise
	if signal.BearishSignal {
		c.JSON(http.StatusOK, gin.H{
			"signal": signal,
		})
	} else {
		c.Status(http.StatusNoContent)
	}
}

// GetGraph returns a PNG chart image for Telegram
func (h *Handlers) GetGraph(c *gin.Context) {
	// Get period from query parameter (default to week)
	period := c.DefaultQuery("period", "week")
	if period != "week" && period != "month" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid period",
			"message": "Period must be 'week' or 'month'",
		})
		return
	}

	// Get graph data from client
	graphData, err := h.client.GetGraphData(period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch graph data",
			"message": err.Error(),
		})
		return
	}

	// Generate PNG chart with dual Y-axes
	pngData, err := h.client.GenerateChartPNG(graphData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to generate chart",
			"details": err.Error(),
		})
		return
	}

	// Set headers for PNG image
	c.Header("Content-Type", "image/png")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=btc-usdc-chart-%s.png", period))
	c.Header("Cache-Control", "public, max-age=300") // Cache for 5 minutes

	// Return PNG data
	c.Data(http.StatusOK, "image/png", pngData)
}

// CheckSignal performs a manual signal check and returns detailed results
func (h *Handlers) CheckSignal(c *gin.Context) {
	// Track asset value before checking signals
	if err := h.client.TrackAssetValue(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to track asset value",
			"message": err.Error(),
		})
		return
	}

	// Get signal using lightweight method
	signal, err := h.client.GetSignalLightweight()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to calculate signal",
			"message": err.Error(),
		})
		return
	}

	// Determine if webhook would be sent
	webhookURL := os.Getenv("WEBHOOK_URL")
	webhookWouldBeSent := webhookURL != "" && signal.BearishSignal

	// Return detailed signal information
	c.JSON(http.StatusOK, gin.H{
		"signal": gin.H{
			"bearish_signal": signal.BearishSignal,
			"triggers":       signal.Triggers,
			"timestamp":      signal.Timestamp,
			"indicators":     signal.Indicators,
		},
		"webhook": gin.H{
			"configured":    webhookURL != "",
			"url":           webhookURL,
			"would_be_sent": webhookWouldBeSent,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
