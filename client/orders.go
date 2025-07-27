package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CoinbaseOrder represents the raw order response from Coinbase API
type CoinbaseOrder struct {
	OrderID            string `json:"order_id"`
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	Status             string `json:"status"`
	CreatedTime        string `json:"created_time"`
	FilledSize         string `json:"filled_size"`
	FilledValue        string `json:"filled_value"`
	AverageFilledPrice string `json:"average_filled_price"`
	OrderConfiguration struct {
		LimitLimitGtc *struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
		} `json:"limit_limit_gtc,omitempty"`
		StopLimitStopLimitGtc *struct {
			BaseSize      string `json:"base_size"`
			LimitPrice    string `json:"limit_price"`
			StopPrice     string `json:"stop_price"`
			StopDirection string `json:"stop_direction"`
		} `json:"stop_limit_stop_limit_gtc,omitempty"`
	} `json:"order_configuration"`
}

// CreateOrderRequest represents the request for creating orders
type CoinbaseCreateOrderRequest struct {
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	OrderConfiguration struct {
		LimitLimitGtc *struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
		} `json:"limit_limit_gtc,omitempty"`
		StopLimitStopLimitGtc *struct {
			BaseSize      string `json:"base_size"`
			LimitPrice    string `json:"limit_price"`
			StopPrice     string `json:"stop_price"`
			StopDirection string `json:"stop_direction"`
		} `json:"stop_limit_stop_limit_gtc,omitempty"`
	} `json:"order_configuration"`
}

// CreateOrderResponse represents the response from creating an order
type CreateOrderResponse struct {
	OrderID string `json:"order_id"`
}

// OrdersResponse represents the response from the orders endpoint
type OrdersResponse struct {
	Orders []CoinbaseOrder `json:"orders"`
}

// checkBalance validates if there are sufficient funds for the order
func (c *CoinbaseClient) checkBalance(side, size, price string) error {
	accounts, err := c.GetAccounts()
	if err != nil {
		c.logger.Printf("Warning: Could not check balance: %v", err)
		return nil // Don't fail the order if we can't check balance
	}

	// Calculate required amount
	var requiredAmount float64
	var requiredCurrency string

	if side == "BUY" {
		// For BUY orders, we need quote currency (e.g., USDC)
		sizeFloat, _ := strconv.ParseFloat(size, 64)
		priceFloat, _ := strconv.ParseFloat(price, 64)
		requiredAmount = sizeFloat * priceFloat
		requiredCurrency = strings.Split(c.tradingPair, "-")[1] // Quote currency
	} else {
		// For SELL orders, we need base currency (e.g., BTC)
		sizeFloat, _ := strconv.ParseFloat(size, 64)
		requiredAmount = sizeFloat
		requiredCurrency = strings.Split(c.tradingPair, "-")[0] // Base currency
	}

	// Find the required currency account
	for _, account := range accounts {
		if account.Currency == requiredCurrency {
			availableBalance, _ := strconv.ParseFloat(account.AvailableBalance, 64)
			if availableBalance < requiredAmount {
				return fmt.Errorf("insufficient %s balance: need %.8f, have %.8f",
					requiredCurrency, requiredAmount, availableBalance)
			}
			c.logger.Printf("Balance check passed: %.8f %s available for %s order",
				availableBalance, requiredCurrency, side)
			return nil
		}
	}

	c.logger.Printf("Warning: Could not find %s account for balance check", requiredCurrency)
	return nil // Don't fail if we can't find the account
}

// createOrder is a helper function to create orders with common logic
func (c *CoinbaseClient) createOrder(side, size, price, stopPrice, limitPrice string) (*Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Placing %s order: size=%s, price=%s", side, size, price)

	// Check balance before placing order
	if err := c.checkBalance(side, size, price); err != nil {
		return nil, fmt.Errorf("balance check failed: %w", err)
	}

	orderReq := CoinbaseCreateOrderRequest{
		ProductID: c.tradingPair,
		Side:      side,
	}

	// Configure order type
	if stopPrice != "" && limitPrice != "" {
		c.logger.Printf("Creating stop limit order: stop=%s, limit=%s", stopPrice, limitPrice)

		// Determine stop direction based on order side
		stopDirection := "STOP_DIRECTION_STOP_DOWN" // Default for SELL
		if side == "BUY" {
			stopDirection = "STOP_DIRECTION_STOP_UP"
		}

		orderReq.OrderConfiguration.StopLimitStopLimitGtc = &struct {
			BaseSize      string `json:"base_size"`
			LimitPrice    string `json:"limit_price"`
			StopPrice     string `json:"stop_price"`
			StopDirection string `json:"stop_direction"`
		}{
			BaseSize:      size,
			LimitPrice:    limitPrice,
			StopPrice:     stopPrice,
			StopDirection: stopDirection,
		}
	} else {
		orderReq.OrderConfiguration.LimitLimitGtc = &struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
		}{
			BaseSize:   size,
			LimitPrice: price,
		}
	}

	respBody, err := c.makeRequest(ctx, "POST", "/orders", orderReq)
	if err != nil {
		c.logger.Printf("Error creating %s order: %v", side, err)
		return nil, fmt.Errorf("failed to create %s order: %w", side, err)
	}

	var resp CreateOrderResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal create order response: %w", err)
	}

	// Create order response
	order := &Order{
		ID:        resp.OrderID,
		ProductID: c.tradingPair,
		Side:      side,
		Type:      "LIMIT",
		Size:      size,
		Price:     price,
		Status:    "PENDING",
		CreatedAt: time.Now(),
	}

	if stopPrice != "" && limitPrice != "" {
		order.StopPrice = stopPrice
		order.LimitPrice = limitPrice
		order.Type = "STOP_LIMIT"
	}

	c.logger.Printf("Successfully created %s order: %s", side, order.ID)
	return order, nil
}

// BuyBTC places a buy order for the configured trading pair, optionally with stop limit
func (c *CoinbaseClient) BuyBTC(size, price, stopPrice, limitPrice string) (*Order, error) {
	return c.createOrder("BUY", size, price, stopPrice, limitPrice)
}

// SellBTC places a sell order for the configured trading pair, optionally with stop limit
func (c *CoinbaseClient) SellBTC(size, price, stopPrice, limitPrice string) (*Order, error) {
	return c.createOrder("SELL", size, price, stopPrice, limitPrice)
}

// GetOrders retrieves all orders
func (c *CoinbaseClient) GetOrders() ([]Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Fetching orders...")

	// Use the correct endpoint from Coinbase API documentation
	// Filter for open orders only (active orders that can be canceled/modified)
	endpoint := fmt.Sprintf("/orders/historical/batch?product_ids=%s&order_status=OPEN&limit=100", c.tradingPair)

	respBody, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		c.logger.Printf("Error fetching orders: %v", err)
		return nil, fmt.Errorf("failed to fetch orders: %w", err)
	}

	// Debug: Log the raw response
	c.logger.Printf("Raw orders response: %s", string(respBody))

	var resp OrdersResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		c.logger.Printf("Failed to unmarshal orders response: %v", err)
		return nil, fmt.Errorf("failed to unmarshal orders response: %w", err)
	}

	// Convert to our simplified structure
	var orders []Order
	for _, order := range resp.Orders {
		// Extract order details based on configuration type
		var size, price, stopPrice, limitPrice string
		var orderType string

		if order.OrderConfiguration.LimitLimitGtc != nil {
			size = order.OrderConfiguration.LimitLimitGtc.BaseSize
			price = order.OrderConfiguration.LimitLimitGtc.LimitPrice
			orderType = "LIMIT"
		} else if order.OrderConfiguration.StopLimitStopLimitGtc != nil {
			size = order.OrderConfiguration.StopLimitStopLimitGtc.BaseSize
			limitPrice = order.OrderConfiguration.StopLimitStopLimitGtc.LimitPrice
			stopPrice = order.OrderConfiguration.StopLimitStopLimitGtc.StopPrice
			orderType = "STOP_LIMIT"
		}

		// Parse the created time
		createdAt := time.Now() // Default fallback
		if order.CreatedTime != "" {
			if parsed, err := time.Parse(time.RFC3339, order.CreatedTime); err == nil {
				createdAt = parsed
			}
		}

		orders = append(orders, Order{
			ID:           order.OrderID,
			ProductID:    order.ProductID,
			Side:         order.Side,
			Type:         orderType,
			Size:         size,
			Price:        price,
			StopPrice:    stopPrice,
			LimitPrice:   limitPrice,
			Status:       order.Status,
			CreatedAt:    createdAt,
			FilledSize:   order.FilledSize,
			FilledValue:  order.FilledValue,
			AveragePrice: order.AverageFilledPrice,
		})
	}

	c.logger.Printf("Successfully fetched %d orders", len(orders))
	return orders, nil
}

// CancelOrder cancels a specific order
func (c *CoinbaseClient) CancelOrder(orderID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Cancelling order: %s", orderID)

	cancelReq := struct {
		OrderIDs []string `json:"order_ids"`
	}{
		OrderIDs: []string{orderID},
	}

	_, err := c.makeRequest(ctx, "POST", "/orders/batch_cancel", cancelReq)
	if err != nil {
		c.logger.Printf("Error cancelling order %s: %v", orderID, err)
		return fmt.Errorf("failed to cancel order %s: %w", orderID, err)
	}

	c.logger.Printf("Successfully cancelled order: %s", orderID)
	return nil
}

// GetCandles retrieves candle data for the configured trading pair
func (c *CoinbaseClient) GetCandles(start, end, granularity string, limit int) ([]Candle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Fetching candles for %s: start=%s, end=%s, granularity=%s", c.tradingPair, start, end, granularity)

	// Build query parameters
	params := fmt.Sprintf("?start=%s&end=%s&granularity=%s", start, end, granularity)
	if limit > 0 {
		params += fmt.Sprintf("&limit=%d", limit)
	}

	endpoint := fmt.Sprintf("/products/%s/candles%s", c.tradingPair, params)

	respBody, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		c.logger.Printf("Error fetching candles: %v", err)
		return nil, fmt.Errorf("failed to fetch candles: %w", err)
	}

	var resp CandlesResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal candles response: %w", err)
	}

	c.logger.Printf("Successfully fetched %d candles", len(resp.Candles))
	return resp.Candles, nil
}

// GetOrderBook retrieves the order book for the configured trading pair
func (c *CoinbaseClient) GetOrderBook(level int) (*OrderBook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Fetching order book for %s (level %d)...", c.tradingPair, level)

	endpoint := fmt.Sprintf("/products/%s/book?level=%d", c.tradingPair, level)

	respBody, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		c.logger.Printf("Error fetching order book: %v", err)
		return nil, fmt.Errorf("failed to fetch order book: %w", err)
	}

	var orderBook OrderBook
	if err := json.Unmarshal(respBody, &orderBook); err != nil {
		return nil, fmt.Errorf("failed to unmarshal order book response: %w", err)
	}

	c.logger.Printf("Successfully fetched order book with %d bids and %d asks", len(orderBook.Bids), len(orderBook.Asks))
	return &orderBook, nil
}

// GetMarketState retrieves comprehensive market state information
func (c *CoinbaseClient) GetMarketState(depth int) (*MarketState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Fetching market state for %s (depth %d)...", c.tradingPair, depth)

	// Get order book
	orderBook, err := c.GetOrderBook(depth)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}

	// Get product information for last price and volume
	productEndpoint := fmt.Sprintf("/products/%s", c.tradingPair)
	respBody, err := c.makeRequest(ctx, "GET", productEndpoint, nil)
	if err != nil {
		c.logger.Printf("Error fetching product info: %v", err)
		return nil, fmt.Errorf("failed to fetch product info: %w", err)
	}

	var productInfo struct {
		ProductID string `json:"product_id"`
		LastPrice string `json:"last_price"`
		Volume24h string `json:"volume_24h"`
	}

	if err := json.Unmarshal(respBody, &productInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal product info: %w", err)
	}

	// Calculate best bid and ask
	var bestBid, bestAsk string
	if len(orderBook.Bids) > 0 {
		bestBid = orderBook.Bids[0].Price
	}
	if len(orderBook.Asks) > 0 {
		bestAsk = orderBook.Asks[0].Price
	}

	// Calculate spread
	var spread, spreadPercent string
	if bestBid != "" && bestAsk != "" {
		bidFloat, _ := strconv.ParseFloat(bestBid, 64)
		askFloat, _ := strconv.ParseFloat(bestAsk, 64)
		spreadValue := askFloat - bidFloat
		spreadPercentValue := (spreadValue / bidFloat) * 100

		spread = fmt.Sprintf("%.8f", spreadValue)
		spreadPercent = fmt.Sprintf("%.4f", spreadPercentValue)
	}

	marketState := &MarketState{
		ProductID:     c.tradingPair,
		BestBid:       bestBid,
		BestAsk:       bestAsk,
		Spread:        spread,
		SpreadPercent: spreadPercent,
		LastPrice:     productInfo.LastPrice,
		Volume24h:     productInfo.Volume24h,
		OrderBook:     *orderBook,
		Timestamp:     time.Now().Unix(),
	}

	c.logger.Printf("Market state: Bid=%s, Ask=%s, Spread=%s (%s%%)",
		bestBid, bestAsk, spread, spreadPercent)

	return marketState, nil
}
