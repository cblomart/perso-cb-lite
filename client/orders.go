package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
	} `json:"order_configuration"`
}

// CreateOrderRequest represents the request for creating orders
type CoinbaseCreateOrderRequest struct {
	ProductID          string `json:"product_id"`
	Side               string `json:"side"`
	ClientOrderID      string `json:"client_order_id"`
	OrderConfiguration struct {
		LimitLimitGtc *struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
		} `json:"limit_limit_gtc,omitempty"`
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

// calculateCoinbaseFee calculates the total fee for a given trade amount
func (c *CoinbaseClient) calculateCoinbaseFee(tradeAmount float64) float64 {
	// 0.50% spread per transaction
	spreadFee := tradeAmount * 0.005

	// Flat fee based on trade amount
	var flatFee float64
	switch {
	case tradeAmount <= 10:
		flatFee = 0.99
	case tradeAmount <= 25:
		flatFee = 1.49
	case tradeAmount <= 50:
		flatFee = 1.99
	case tradeAmount <= 200:
		flatFee = 2.99
	default:
		// Trades over $200 incur a 1.49% fee
		flatFee = tradeAmount * 0.0149
	}

	return spreadFee + flatFee
}

// CalculateOrderSizeByPercentage calculates the order size based on a percentage of available balance
// Includes actual Coinbase fees (0.50% spread + tiered flat fees) to ensure the order can be placed successfully
func (c *CoinbaseClient) CalculateOrderSizeByPercentage(side string, percentage float64, price string) (string, error) {
	// Validate percentage
	if percentage <= 0 || percentage > 100 {
		return "", fmt.Errorf("percentage must be between 0 and 100")
	}

	accounts, err := c.GetAccounts()
	if err != nil {
		return "", fmt.Errorf("failed to fetch accounts: %w", err)
	}

	var availableBalance float64
	var currency string

	if side == "BUY" {
		// For BUY orders, we need quote currency (e.g., USDC)
		currency = strings.Split(c.tradingPair, "-")[1] // Quote currency
	} else {
		// For SELL orders, we need base currency (e.g., BTC)
		currency = strings.Split(c.tradingPair, "-")[0] // Base currency
	}

	// Find the required currency account
	for _, account := range accounts {
		if account.Currency == currency {
			availableBalance, _ = strconv.ParseFloat(account.AvailableBalance, 64)
			break
		}
	}

	if availableBalance <= 0 {
		return "", fmt.Errorf("no available %s balance", currency)
	}

	// Calculate the base amount to use based on percentage
	baseAmount := availableBalance * (percentage / 100.0)

	var orderSize float64
	if side == "BUY" {
		// For BUY orders, we need to calculate the trade value first
		priceFloat, err := strconv.ParseFloat(price, 64)
		if err != nil {
			return "", fmt.Errorf("invalid price format: %w", err)
		}
		if priceFloat <= 0 {
			return "", fmt.Errorf("price must be greater than 0")
		}

		// Calculate the maximum BTC we can buy with the available amount
		maxBTC := baseAmount / priceFloat
		tradeValue := maxBTC * priceFloat

		// Calculate the fee for this trade
		fee := c.calculateCoinbaseFee(tradeValue)

		// Adjust the trade value to account for fees
		adjustedTradeValue := tradeValue - fee
		orderSize = adjustedTradeValue / priceFloat

		// Log the calculation for transparency (only in debug mode)
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("BUY calculation: %.2f%% requested, base amount: %.8f %s, trade value: %.2f, fee: %.2f, adjusted trade value: %.2f, order size: %.8f BTC",
				percentage, baseAmount, currency, tradeValue, fee, adjustedTradeValue, orderSize)
		}
	} else {
		// For SELL orders, calculate the trade value and adjust for fees
		priceFloat, err := strconv.ParseFloat(price, 64)
		if err != nil {
			return "", fmt.Errorf("invalid price format: %w", err)
		}
		if priceFloat <= 0 {
			return "", fmt.Errorf("price must be greater than 0")
		}

		// Calculate the trade value
		tradeValue := baseAmount * priceFloat

		// Calculate the fee for this trade
		fee := c.calculateCoinbaseFee(tradeValue)

		// Adjust the BTC amount to account for fees
		adjustedBTC := baseAmount - (fee / priceFloat)
		orderSize = adjustedBTC

		// Log the calculation for transparency (only in debug mode)
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("SELL calculation: %.2f%% requested, base amount: %.8f %s, trade value: %.2f, fee: %.2f, adjusted BTC: %.8f, order size: %.8f BTC",
				percentage, baseAmount, currency, tradeValue, fee, adjustedBTC, orderSize)
		}
	}

	// Format to 8 decimal places (standard for crypto)
	return fmt.Sprintf("%.8f", orderSize), nil
}

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
				shortfall := requiredAmount - availableBalance
				return fmt.Errorf("insufficient %s balance: need %.8f, have %.8f (shortfall: %.8f)",
					requiredCurrency, requiredAmount, availableBalance, shortfall)
			}
			return nil
		}
	}

	c.logger.Printf("Warning: Could not find %s account for balance check", requiredCurrency)
	return nil // Don't fail if we can't find the account
}

// createOrder is a helper function to create market orders
func (c *CoinbaseClient) createOrder(side, size string, price float64) (*Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Placing %s market order: size=%s, price=%.8f", side, size, price)
	}

	// Check balance before placing order
	if err := c.checkBalance(side, size, fmt.Sprintf("%.8f", price)); err != nil {
		return nil, fmt.Errorf("balance check failed: %w", err)
	}

	// Generate a unique client order ID
	clientOrderID := uuid.New().String()

	orderReq := CoinbaseCreateOrderRequest{
		ProductID:     c.tradingPair,
		Side:          side,
		ClientOrderID: clientOrderID,
	}

	// Configure market order
	orderReq.OrderConfiguration.LimitLimitGtc = &struct {
		BaseSize   string `json:"base_size"`
		LimitPrice string `json:"limit_price"`
	}{
		BaseSize:   size,
		LimitPrice: fmt.Sprintf("%.8f", price),
	}

	respBody, err := c.makeRequest(ctx, "POST", "/orders", orderReq)
	if err != nil {
		c.logger.Printf("Error creating %s order: %v", side, err)
		return nil, fmt.Errorf("failed to create %s order: %w", side, err)
	}

	// Check for error response from Coinbase
	var errorResp struct {
		ErrorResponse struct {
			Error                string `json:"error"`
			ErrorDetails         string `json:"error_details"`
			Message              string `json:"message"`
			PreviewFailureReason string `json:"preview_failure_reason"`
		} `json:"error_response"`
		Success bool `json:"success"`
	}

	if err := json.Unmarshal(respBody, &errorResp); err == nil && !errorResp.Success {
		// Order failed with error response
		errorMsg := errorResp.ErrorResponse.Message
		if errorMsg == "" {
			errorMsg = errorResp.ErrorResponse.Error
		}
		if errorResp.ErrorResponse.PreviewFailureReason != "" {
			errorMsg = fmt.Sprintf("%s (Preview: %s)", errorMsg, errorResp.ErrorResponse.PreviewFailureReason)
		}
		return nil, fmt.Errorf("order failed: %s", errorMsg)
	}

	var resp CreateOrderResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal create order response: %w", err)
	}

	// Create order response
	order := &Order{
		ID:            resp.OrderID,
		ClientOrderID: clientOrderID,
		ProductID:     c.tradingPair,
		Side:          side,
		Type:          "MARKET",
		Size:          size,
		Price:         fmt.Sprintf("%.8f", price),
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully created %s order: %s", side, order.ID)
	}
	return order, nil
}

// BuyBTC places a buy order for the configured trading pair
func (c *CoinbaseClient) BuyBTC(size string, price float64) (*Order, error) {
	return c.createOrder("BUY", size, price)
}

// SellBTC places a sell order for the configured trading pair
func (c *CoinbaseClient) SellBTC(size string, price float64) (*Order, error) {
	return c.createOrder("SELL", size, price)
}

// GetOrders retrieves all orders
func (c *CoinbaseClient) GetOrders() ([]Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	// Debug: Log the raw response (only in debug mode)
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Raw orders response: %s", string(respBody))
	}

	var resp OrdersResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		c.logger.Printf("Failed to unmarshal orders response: %v", err)
		return nil, fmt.Errorf("failed to unmarshal orders response: %w", err)
	}

	// Convert to our simplified structure
	var orders []Order
	for _, order := range resp.Orders {
		// Extract order details based on configuration type
		var size, price string
		var orderType string

		if order.OrderConfiguration.LimitLimitGtc != nil {
			size = order.OrderConfiguration.LimitLimitGtc.BaseSize
			price = order.OrderConfiguration.LimitLimitGtc.LimitPrice
			orderType = "MARKET"
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
			Status:       order.Status,
			CreatedAt:    createdAt,
			FilledSize:   order.FilledSize,
			FilledValue:  order.FilledValue,
			AveragePrice: order.AverageFilledPrice,
		})
	}

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched %d orders", len(orders))
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched %d candles", len(resp.Candles))
	}
	return resp.Candles, nil
}

// GetOrderBook retrieves the order book for the configured trading pair
func (c *CoinbaseClient) GetOrderBook(limit int) (*OrderBook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching order book for %s (limit %d)...", c.tradingPair, limit)
	}

	// Validate limit (reasonable range for order book)
	if limit > 100 {
		limit = 100
	} else if limit < 1 {
		limit = 10
	}

	endpoint := fmt.Sprintf("/product_book?product_id=%s&limit=%d", c.tradingPair, limit)

	respBody, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		c.logger.Printf("Error fetching order book: %v", err)
		return nil, fmt.Errorf("failed to fetch order book: %w", err)
	}

	// Parse the response structure from Coinbase API
	var response struct {
		Pricebook struct {
			ProductID string           `json:"product_id"`
			Bids      []OrderBookEntry `json:"bids"`
			Asks      []OrderBookEntry `json:"asks"`
			Time      string           `json:"time"`
		} `json:"pricebook"`
		Last           string `json:"last"`
		MidMarket      string `json:"mid_market"`
		SpreadBps      string `json:"spread_bps"`
		SpreadAbsolute string `json:"spread_absolute"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal order book response: %w", err)
	}

	orderBook := &OrderBook{
		Bids: response.Pricebook.Bids,
		Asks: response.Pricebook.Asks,
	}

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched order book with %d bids and %d asks", len(orderBook.Bids), len(orderBook.Asks))
	}
	return orderBook, nil
}

// GetMarketState retrieves comprehensive market state information
func (c *CoinbaseClient) GetMarketState(limit int) (*MarketState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Only log in debug mode for performance
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching market state for %s (limit %d)...", c.tradingPair, limit)
	}

	// Get order book
	orderBook, err := c.GetOrderBook(limit)
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
