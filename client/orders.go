package client

import (
	"context"
	"encoding/json"
	"fmt"
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
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
			StopPrice  string `json:"stop_price"`
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
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
			StopPrice  string `json:"stop_price"`
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

// createOrder is a helper function to create orders with common logic
func (c *CoinbaseClient) createOrder(side, size, price, stopPrice, limitPrice string) (*Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.logger.Printf("Placing %s order: size=%s, price=%s", side, size, price)

	orderReq := CoinbaseCreateOrderRequest{
		ProductID: c.tradingPair,
		Side:      side,
	}

	// Configure order type
	if stopPrice != "" && limitPrice != "" {
		c.logger.Printf("Creating stop limit order: stop=%s, limit=%s", stopPrice, limitPrice)
		orderReq.OrderConfiguration.StopLimitStopLimitGtc = &struct {
			BaseSize   string `json:"base_size"`
			LimitPrice string `json:"limit_price"`
			StopPrice  string `json:"stop_price"`
		}{
			BaseSize:   size,
			LimitPrice: limitPrice,
			StopPrice:  stopPrice,
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
