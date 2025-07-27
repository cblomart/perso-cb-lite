package main

import (
	"net/http"
	"strconv"

	"coinbase-base/client"

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

	// Validate size and price
	if _, err := strconv.ParseFloat(req.Size, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid size format",
			"message": "Size must be a valid number",
		})
		return
	}

	if _, err := strconv.ParseFloat(req.Price, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid price format",
			"message": "Price must be a valid number",
		})
		return
	}

	// Validate stop price and limit price if provided
	if req.StopPrice != "" || req.LimitPrice != "" {
		if req.StopPrice == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing stop price",
				"message": "Stop price is required when limit price is provided",
			})
			return
		}
		if req.LimitPrice == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing limit price",
				"message": "Limit price is required when stop price is provided",
			})
			return
		}

		// Validate stop price format
		if _, err := strconv.ParseFloat(req.StopPrice, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid stop price format",
				"message": "Stop price must be a valid number",
			})
			return
		}

		// Validate limit price format
		if _, err := strconv.ParseFloat(req.LimitPrice, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid limit price format",
				"message": "Limit price must be a valid number",
			})
			return
		}
	}

	order, err := h.client.BuyBTC(req.Size, req.Price, req.StopPrice, req.LimitPrice)
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

	if req.StopPrice != "" && req.LimitPrice != "" {
		response["stop_limit_created"] = true
		response["stop_price"] = req.StopPrice
		response["limit_price"] = req.LimitPrice
	}

	c.JSON(http.StatusCreated, response)
}

// SellBTC places a sell order for BTC to USDC, optionally with stop loss protection
func (h *Handlers) SellBTC(c *gin.Context) {
	var req client.TradingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"message": err.Error(),
		})
		return
	}

	// Validate size and price
	if _, err := strconv.ParseFloat(req.Size, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid size format",
			"message": "Size must be a valid number",
		})
		return
	}

	if _, err := strconv.ParseFloat(req.Price, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid price format",
			"message": "Price must be a valid number",
		})
		return
	}

	// Validate stop price and limit price if provided
	if req.StopPrice != "" || req.LimitPrice != "" {
		if req.StopPrice == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing stop price",
				"message": "Stop price is required when limit price is provided",
			})
			return
		}
		if req.LimitPrice == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing limit price",
				"message": "Limit price is required when stop price is provided",
			})
			return
		}

		// Validate stop price format
		if _, err := strconv.ParseFloat(req.StopPrice, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid stop price format",
				"message": "Stop price must be a valid number",
			})
			return
		}

		// Validate limit price format
		if _, err := strconv.ParseFloat(req.LimitPrice, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid limit price format",
				"message": "Limit price must be a valid number",
			})
			return
		}
	}

	order, err := h.client.SellBTC(req.Size, req.Price, req.StopPrice, req.LimitPrice)
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

	if req.StopPrice != "" && req.LimitPrice != "" {
		response["stop_limit_created"] = true
		response["stop_price"] = req.StopPrice
		response["limit_price"] = req.LimitPrice
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
