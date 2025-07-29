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
		LimitLimitIoc *struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
		} `json:"limit_limit_ioc,omitempty"`
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

		// Log calculation details in debug mode
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("BUY calculation: %.2f%% requested, base amount: %.8f %s, trade value: %.2f, fee: %.2f, adjusted trade value: %.2f, order size: %.8f BTC",
				percentage, availableBalance, currency, tradeValue, fee, adjustedTradeValue, orderSize)
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

		// Log calculation details in debug mode
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("SELL calculation: %.2f%% requested, base amount: %.8f %s, trade value: %.2f, fee: %.2f, adjusted BTC: %.8f, order size: %.8f BTC",
				percentage, availableBalance, currency, tradeValue, fee, adjustedBTC, orderSize)
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

	// Find the required account
	var requiredAccount *Account
	for _, account := range accounts {
		if account.Currency == requiredCurrency {
			requiredAccount = &account
			break
		}
	}

	if requiredAccount == nil {
		c.logger.Printf("Warning: Could not find %s account for balance check", requiredCurrency)
		return nil
	}

	availableBalance, _ := strconv.ParseFloat(requiredAccount.AvailableBalance, 64)
	if availableBalance < requiredAmount {
		shortfall := requiredAmount - availableBalance
		return fmt.Errorf("insufficient %s balance: need %.8f, have %.8f (shortfall: %.8f)",
			requiredCurrency, requiredAmount, availableBalance, shortfall)
	}
	return nil
}

// createOrder is a helper function to create market orders
func (c *CoinbaseClient) createOrder(side, size string, price float64) (*Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log order placement in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Placing %s IOC order: size=%s, price=%.8f", side, size, price)
	}

	// Check balance if possible
	if err := c.checkBalance(side, size, fmt.Sprintf("%.8f", price)); err != nil {
		c.logger.Printf("Warning: Could not check balance: %v", err)
	}

	// Generate a unique client order ID
	clientOrderID := uuid.New().String()

	orderReq := CoinbaseCreateOrderRequest{
		ProductID:     c.tradingPair,
		Side:          side,
		ClientOrderID: clientOrderID,
	}

	// Configure market order with IOC (Immediate or Cancel)
	// This ensures the order executes immediately or gets canceled entirely
	orderReq.OrderConfiguration.LimitLimitIoc = &struct {
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
		Type:          "LIMIT_IOC", // Updated to reflect IOC order type
		Size:          size,
		Price:         fmt.Sprintf("%.8f", price),
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Log successful order creation in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully created %s order: %s", side, order.ID)
	}

	// Small pause to allow Coinbase and market to process the IOC order
	// This ensures we get accurate status when we check
	time.Sleep(500 * time.Millisecond) // 500ms pause

	// For IOC orders, immediately check the status to see if it was filled or canceled
	// This gives us immediate feedback on whether the order executed
	orderStatus, err := c.GetOrderStatus(order.ID)
	if err != nil {
		c.logger.Printf("Warning: Could not check order status for %s: %v", order.ID, err)
	} else {
		// Update the order with the actual status
		order.Status = orderStatus.Status
		order.FilledSize = orderStatus.FilledSize
		order.FilledValue = orderStatus.FilledValue
		order.AveragePrice = orderStatus.AverageFilledPrice

		// Log the immediate result
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			if orderStatus.Status == "FILLED" {
				c.logger.Printf("✅ IOC order %s was FILLED: %s @ %s", order.ID, orderStatus.FilledSize, orderStatus.AverageFilledPrice)
			} else if orderStatus.Status == "CANCELED" {
				c.logger.Printf("❌ IOC order %s was CANCELED (no liquidity at limit price)", order.ID)
			} else {
				c.logger.Printf("⚠️ IOC order %s status: %s", order.ID, orderStatus.Status)
			}
		}
	}

	return order, nil
}

// IsOrderSuccessful checks if an IOC order was successfully filled
func (c *CoinbaseClient) IsOrderSuccessful(order *Order) bool {
	return order.Status == "FILLED"
}

// GetOrderResult provides a human-readable result of the order execution
func (c *CoinbaseClient) GetOrderResult(order *Order) string {
	switch order.Status {
	case "FILLED":
		return fmt.Sprintf("✅ Order %s was FILLED: %s @ %s", order.ID, order.FilledSize, order.AveragePrice)
	case "CANCELED":
		return fmt.Sprintf("❌ Order %s was CANCELED (no liquidity at limit price)", order.ID)
	case "PENDING":
		return fmt.Sprintf("⏳ Order %s is still PENDING", order.ID)
	default:
		return fmt.Sprintf("⚠️ Order %s status: %s", order.ID, order.Status)
	}
}

// BuyBTC places a buy order for the configured trading pair
func (c *CoinbaseClient) BuyBTC(size string, price float64) (*Order, error) {
	// Create order
	order, err := c.createOrder("BUY", size, price)
	if err != nil {
		c.logger.Printf("Error creating BUY order: %v", err)
		return nil, fmt.Errorf("failed to create BUY order: %w", err)
	}

	// Log the result
	c.logger.Printf("BUY Order Result: %s", c.GetOrderResult(order))
	return order, nil
}

// SellBTC places a sell order for the configured trading pair
func (c *CoinbaseClient) SellBTC(size string, price float64) (*Order, error) {
	// Create order
	order, err := c.createOrder("SELL", size, price)
	if err != nil {
		c.logger.Printf("Error creating SELL order: %v", err)
		return nil, fmt.Errorf("failed to create SELL order: %w", err)
	}

	// Log the result
	c.logger.Printf("SELL Order Result: %s", c.GetOrderResult(order))
	return order, nil
}

// GetOrderStatus retrieves the status of a specific order
func (c *CoinbaseClient) GetOrderStatus(orderID string) (*CoinbaseOrder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("/orders/historical/%s", orderID)

	respBody, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get order status: %w", err)
	}

	var order CoinbaseOrder
	if err := json.Unmarshal(respBody, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal order status response: %w", err)
	}

	return &order, nil
}

// GetOrders retrieves all orders
func (c *CoinbaseClient) GetOrders() ([]Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log order fetching in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching orders...")
	}

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

	// Log successful order fetch in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched %d orders", len(orders))
	}
	return orders, nil
}

// CancelOrder cancels a specific order
func (c *CoinbaseClient) CancelOrder(orderID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Log order cancellation in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Cancelling order: %s", orderID)
	}

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

	// Log successful cancellation in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully cancelled order: %s", orderID)
	}
	return nil
}

// GetCandles retrieves candle data for the configured trading pair
func (c *CoinbaseClient) GetCandles(start, end, granularity string, limit int) ([]Candle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log candle fetching in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching candles for %s: start=%s, end=%s, granularity=%s", c.tradingPair, start, end, granularity)
	}

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

	// Log successful candle fetch in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched %d candles", len(resp.Candles))
	}
	return resp.Candles, nil
}

// GetOrderBook retrieves the order book for the configured trading pair
func (c *CoinbaseClient) GetOrderBook(limit int) (*OrderBook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log order book fetching in debug mode
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

	// Convert to our simplified structure
	orderBook := &OrderBook{
		Bids: response.Pricebook.Bids,
		Asks: response.Pricebook.Asks,
	}

	// Log successful order book fetch in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched order book with %d bids and %d asks", len(orderBook.Bids), len(orderBook.Asks))
	}
	return orderBook, nil
}

// GetSignal calculates technical indicators and checks for bearish signals
func (c *CoinbaseClient) GetSignal() (*SignalResponse, error) {
	return c.GetSignalWithCandles(300, "FIVE_MINUTE")
}

// GetSignalWithCandles allows customizing candle count and granularity for different use cases
func (c *CoinbaseClient) GetSignalWithCandles(candleCount int, granularity string) (*SignalResponse, error) {
	// Log signal fetching in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching signal data for %s (%d %s candles)...", c.tradingPair, candleCount, granularity)
	}

	// Get candles for technical analysis
	candles, err := c.GetCandles("", "", granularity, candleCount)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch candles: %w", err)
	}

	// Calculate technical indicators
	indicators := calculateTechnicalIndicators(candles)

	// Check for trend changes (not just bearish signals)
	trendChange, currentTrend, triggers := c.detectTrendChange(indicators)

	response := &SignalResponse{
		BearishSignal: currentTrend == "bearish",
		Indicators:    indicators,
		Triggers:      triggers,
		Timestamp:     time.Now().Unix(),
	}

	// Send webhook only if there's a significant trend change
	if trendChange && c.webhookURL != "" {
		if err := c.SendWebhook(response); err != nil {
			c.logger.Printf("Failed to send webhook: %v", err)
		} else {
			// Log webhook success in debug mode
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("Webhook notification sent for trend change: %s → %s", currentTrend, triggers)
			}
		}
	}

	// Log signal calculation completion in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Signal calculation complete: bearish=%v, triggers=%v", response.BearishSignal, triggers)
	}

	return response, nil
}

// GetSignalLightweight is optimized for background polling - uses 5-minute candles with fewer data points
func (c *CoinbaseClient) GetSignalLightweight() (*SignalResponse, error) {
	// Use 5-minute candles for 12-hour trend change detection
	// 144 candles = 12 hours of data (144 * 5 minutes = 720 minutes = 12 hours)
	return c.GetSignalWithCandles(144, "FIVE_MINUTE")
}

// GetMarketState retrieves comprehensive market state information
func (c *CoinbaseClient) GetMarketState(limit int) (*MarketState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log market state fetching in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching market state for %s (limit %d)...", c.tradingPair, limit)
	}

	// Get order book
	orderBook, err := c.GetOrderBook(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}

	// Get product information for last price and volume
	respBody, err := c.makeRequest(ctx, "GET", "/products/"+c.tradingPair, nil)
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

	// Log market state completion in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Market state: Bid=%s, Ask=%s, Spread=%s (%s%%)",
			marketState.BestBid, marketState.BestAsk, marketState.Spread, marketState.SpreadPercent)
	}

	return marketState, nil
}

// GetGraphData retrieves comprehensive data for charting
func (c *CoinbaseClient) GetGraphData(period string) (*GraphData, error) {
	// Determine time range and granularity based on period
	var startTime, endTime time.Time
	var granularity string
	var candleLimit int

	endTime = time.Now()
	switch period {
	case "week":
		startTime = endTime.AddDate(0, 0, -7)
		granularity = "ONE_HOUR" // 1-hour candles for week view
		candleLimit = 168        // 7 days * 24 hours
	case "month":
		startTime = endTime.AddDate(0, -1, 0)
		granularity = "SIX_HOUR" // 6-hour candles for month view
		candleLimit = 120        // ~30 days * 4 candles per day
	default:
		return nil, fmt.Errorf("invalid period: %s (use 'week' or 'month')", period)
	}

	// Log graph data fetching in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching graph data for %s period (%s candles)...", period, granularity)
	}

	// Fetch candles
	candles, err := c.GetCandles(
		fmt.Sprintf("%d", startTime.Unix()),
		fmt.Sprintf("%d", endTime.Unix()),
		granularity,
		candleLimit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch candles: %w", err)
	}

	// Fetch trade history (optional - continue even if it fails)
	trades, err := c.GetTradeHistory(startTime, endTime)
	if err != nil {
		// Log the error but continue with empty trades
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("Warning: Failed to fetch trade history: %v", err)
		}
		trades = []Trade{} // Use empty slice
	}

	// Calculate account values over time (use in-memory tracking)
	accountValues := c.GetAssetValueHistoryForPeriod(startTime, endTime)
	if len(accountValues) == 0 {
		// Fallback to calculated values if no in-memory data
		accountValues, err = c.CalculateAccountValuesOverTime(candles, trades, startTime, endTime)
		if err != nil {
			// Log the error but continue with empty account values
			if os.Getenv("LOG_LEVEL") == "DEBUG" {
				c.logger.Printf("Warning: Failed to calculate account values: %v", err)
			}
			accountValues = []AccountValue{} // Use empty slice
		}
	} else {
		// Log successful use of in-memory asset values
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			c.logger.Printf("Using %d in-memory asset value points", len(accountValues))
		}
	}

	// Calculate technical indicators from candles
	indicators := c.CalculateIndicatorsForGraph(candles)

	// Create summary from all available data
	summary := c.CalculateGraphSummary(candles, trades, accountValues)

	graphData := &GraphData{
		Period:        period,
		StartTime:     startTime.Unix(),
		EndTime:       endTime.Unix(),
		Candles:       candles,
		Trades:        trades,
		AccountValues: accountValues,
		Indicators:    indicators,
		Summary:       summary,
	}

	// Log successful graph data fetch in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully generated graph data: %d candles with indicators", len(candles))
		if len(candles) > 0 {
			firstCandle := candles[0]
			lastCandle := candles[len(candles)-1]

			// Parse timestamps consistently (Unix timestamps)
			firstTimeUnix, _ := strconv.ParseInt(firstCandle.Start, 10, 64)
			lastTimeUnix, _ := strconv.ParseInt(lastCandle.Start, 10, 64)
			firstTime := time.Unix(firstTimeUnix, 0)
			lastTime := time.Unix(lastTimeUnix, 0)

			c.logger.Printf("Time range: %s to %s",
				firstTime.Format("2006-01-02 15:04:05"),
				lastTime.Format("2006-01-02 15:04:05"))
			c.logger.Printf("Price range: $%s to $%s", firstCandle.Close, lastCandle.Close)
		}
	}

	return graphData, nil
}

// CalculateGraphSummary calculates summary statistics for the graph
func (c *CoinbaseClient) CalculateGraphSummary(candles []Candle, trades []Trade, accountValues []AccountValue) struct {
	TotalTrades    int     `json:"total_trades"`
	BuyTrades      int     `json:"buy_trades"`
	SellTrades     int     `json:"sell_trades"`
	TotalVolume    float64 `json:"total_volume"`
	TotalFees      float64 `json:"total_fees"`
	StartingValue  float64 `json:"starting_value"`
	EndingValue    float64 `json:"ending_value"`
	ValueChange    float64 `json:"value_change"`
	ValueChangePct float64 `json:"value_change_pct"`
	BestPrice      float64 `json:"best_price"`
	WorstPrice     float64 `json:"worst_price"`
	AveragePrice   float64 `json:"average_price"`
} {
	summary := struct {
		TotalTrades    int     `json:"total_trades"`
		BuyTrades      int     `json:"buy_trades"`
		SellTrades     int     `json:"sell_trades"`
		TotalVolume    float64 `json:"total_volume"`
		TotalFees      float64 `json:"total_fees"`
		StartingValue  float64 `json:"starting_value"`
		EndingValue    float64 `json:"ending_value"`
		ValueChange    float64 `json:"value_change"`
		ValueChangePct float64 `json:"value_change_pct"`
		BestPrice      float64 `json:"best_price"`
		WorstPrice     float64 `json:"worst_price"`
		AveragePrice   float64 `json:"average_price"`
	}{}

	// Trade statistics
	summary.TotalTrades = len(trades)
	var totalVolume, totalFees float64
	for _, trade := range trades {
		if trade.Side == "BUY" {
			summary.BuyTrades++
		} else {
			summary.SellTrades++
		}

		volume, _ := strconv.ParseFloat(trade.FilledValue, 64)
		fee, _ := strconv.ParseFloat(trade.Fee, 64)
		totalVolume += volume
		totalFees += fee
	}
	summary.TotalVolume = totalVolume
	summary.TotalFees = totalFees

	// Price statistics from candles
	var prices []float64
	for _, candle := range candles {
		price, _ := strconv.ParseFloat(candle.Close, 64)
		prices = append(prices, price)
	}

	if len(prices) > 0 {
		summary.BestPrice = prices[0]
		summary.WorstPrice = prices[0]
		var sum float64
		for _, price := range prices {
			if price > summary.BestPrice {
				summary.BestPrice = price
			}
			if price < summary.WorstPrice {
				summary.WorstPrice = price
			}
			sum += price
		}
		summary.AveragePrice = sum / float64(len(prices))
	}

	// Account value statistics
	if len(accountValues) > 0 {
		summary.StartingValue = accountValues[0].TotalUSD
		summary.EndingValue = accountValues[len(accountValues)-1].TotalUSD
		summary.ValueChange = summary.EndingValue - summary.StartingValue
		if summary.StartingValue > 0 {
			summary.ValueChangePct = (summary.ValueChange / summary.StartingValue) * 100
		}
	}

	return summary
}

// GetTradeHistory retrieves completed trades within a time range
func (c *CoinbaseClient) GetTradeHistory(startTime, endTime time.Time) ([]Trade, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log trade history fetching in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Fetching trade history from %s to %s...",
			startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))
	}

	// Use the fills endpoint to get completed trades
	endpoint := fmt.Sprintf("/fills?product_id=%s&start_sequence_timestamp=%d&end_sequence_timestamp=%d&limit=100",
		c.tradingPair,
		startTime.Unix(), endTime.Unix())

	respBody, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		c.logger.Printf("Error fetching trade history: %v", err)
		return nil, fmt.Errorf("failed to fetch trade history: %w", err)
	}

	var resp struct {
		Fills []struct {
			EntryID      string `json:"entry_id"`
			TradeID      string `json:"trade_id"`
			OrderID      string `json:"order_id"`
			ProductID    string `json:"product_id"`
			Side         string `json:"side"`
			Size         string `json:"size"`
			Price        string `json:"price"`
			Fee          string `json:"fee"`
			CreatedAt    string `json:"created_at"`
			UserID       string `json:"user_id"`
			ProfileID    string `json:"profile_id"`
			LiquidityInd string `json:"liquidity_ind"`
			UsdValue     string `json:"usd_value"`
		} `json:"fills"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trade history: %w", err)
	}

	var trades []Trade
	for _, fill := range resp.Fills {
		// Parse timestamps
		createdAt, _ := time.Parse(time.RFC3339, fill.CreatedAt)

		trades = append(trades, Trade{
			ID:          fill.TradeID,
			ProductID:   fill.ProductID,
			Side:        fill.Side,
			Size:        fill.Size,
			Price:       fill.Price,
			FilledSize:  fill.Size,
			FilledValue: fill.UsdValue,
			Fee:         fill.Fee,
			CreatedAt:   createdAt.Unix(),
			ExecutedAt:  createdAt.Unix(),
		})
	}

	// Log successful trade history fetch in debug mode
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		c.logger.Printf("Successfully fetched %d trades", len(trades))
	}

	return trades, nil
}

// CalculateAccountValuesOverTime calculates account values at each candle timestamp
func (c *CoinbaseClient) CalculateAccountValuesOverTime(candles []Candle, trades []Trade, startTime, endTime time.Time) ([]AccountValue, error) {
	// Get current account balances
	accounts, err := c.GetAccounts()
	if err != nil {
		return nil, fmt.Errorf("failed to get current accounts: %w", err)
	}

	// Find BTC and USDC accounts
	var btcAccount, usdcAccount *Account
	for i := range accounts {
		if accounts[i].Currency == "BTC" {
			btcAccount = &accounts[i]
		} else if accounts[i].Currency == "USDC" {
			usdcAccount = &accounts[i]
		}
	}

	if btcAccount == nil || usdcAccount == nil {
		return nil, fmt.Errorf("missing BTC or USDC accounts")
	}

	// Calculate account values at each candle timestamp
	var accountValues []AccountValue

	// Start with current balances and work backwards
	currentBTC, _ := strconv.ParseFloat(btcAccount.AvailableBalance, 64)
	currentUSDC, _ := strconv.ParseFloat(usdcAccount.AvailableBalance, 64)

	// Process trades in reverse chronological order to calculate historical balances
	tradeIndex := len(trades) - 1

	for i := len(candles) - 1; i >= 0; i-- {
		candle := candles[i]
		// Parse the start time from the candle (Unix timestamp)
		candleTimeUnix, err := strconv.ParseInt(candle.Start, 10, 64)
		if err != nil {
			// Skip invalid timestamps
			continue
		}
		candleTime := time.Unix(candleTimeUnix, 0)

		// Process trades that happened before this candle
		for tradeIndex >= 0 && time.Unix(trades[tradeIndex].ExecutedAt, 0).After(candleTime) {
			trade := trades[tradeIndex]

			// Reverse the trade effect
			size, _ := strconv.ParseFloat(trade.Size, 64)
			price, _ := strconv.ParseFloat(trade.Price, 64)
			fee, _ := strconv.ParseFloat(trade.Fee, 64)

			if trade.Side == "BUY" {
				// Reverse buy: remove BTC, add back USDC
				currentBTC -= size
				currentUSDC += (size * price) + fee
			} else {
				// Reverse sell: add back BTC, remove USDC
				currentBTC += size
				currentUSDC -= (size * price) - fee
			}

			tradeIndex--
		}

		// Calculate total USD value using candle price
		price, _ := strconv.ParseFloat(candle.Close, 64)
		totalUSD := currentUSDC + (currentBTC * price)

		accountValues = append([]AccountValue{{
			Timestamp: candleTime.Unix(),
			BTC:       currentBTC,
			USDC:      currentUSDC,
			TotalUSD:  totalUSD,
		}}, accountValues...)
	}

	return accountValues, nil
}

// CalculateIndicatorsForGraph calculates technical indicators for each candle
func (c *CoinbaseClient) CalculateIndicatorsForGraph(candles []Candle) struct {
	EMA12  []float64 `json:"ema_12"`
	EMA26  []float64 `json:"ema_26"`
	RSI    []float64 `json:"rsi"`
	MACD   []float64 `json:"macd"`
	Signal []float64 `json:"signal"`
} {
	if len(candles) < 26 {
		return struct {
			EMA12  []float64 `json:"ema_12"`
			EMA26  []float64 `json:"ema_26"`
			RSI    []float64 `json:"rsi"`
			MACD   []float64 `json:"macd"`
			Signal []float64 `json:"signal"`
		}{}
	}

	// Extract close prices
	prices := make([]float64, len(candles))
	for i, candle := range candles {
		prices[i], _ = strconv.ParseFloat(candle.Close, 64)
	}

	// Calculate indicators for each point
	ema12 := make([]float64, len(prices))
	ema26 := make([]float64, len(prices))
	rsi := make([]float64, len(prices))
	macd := make([]float64, len(prices))
	signal := make([]float64, len(prices))

	// Calculate EMA12 and EMA26 for each point
	for i := 0; i < len(prices); i++ {
		if i >= 11 { // Need at least 12 points for EMA12
			ema12[i] = calculateEMA(prices[:i+1], 12)
		}
		if i >= 25 { // Need at least 26 points for EMA26
			ema26[i] = calculateEMA(prices[:i+1], 26)
		}
	}

	// Calculate RSI for each point
	for i := 0; i < len(prices); i++ {
		if i >= 14 { // Need at least 15 points for RSI(14)
			rsi[i] = calculateRSI(prices[:i+1], 14)
		}
	}

	// Calculate MACD and Signal for each point
	for i := 0; i < len(prices); i++ {
		if i >= 25 { // Need at least 26 points for MACD
			macdVal, signalVal := calculateMACD(prices[:i+1])
			macd[i] = macdVal
			signal[i] = signalVal
		}
	}

	return struct {
		EMA12  []float64 `json:"ema_12"`
		EMA26  []float64 `json:"ema_26"`
		RSI    []float64 `json:"rsi"`
		MACD   []float64 `json:"macd"`
		Signal []float64 `json:"signal"`
	}{
		EMA12:  ema12,
		EMA26:  ema26,
		RSI:    rsi,
		MACD:   macd,
		Signal: signal,
	}
}
