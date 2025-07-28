package client

import "time"

// Account represents a Coinbase account with simplified structure for BTC/USDC trading
type Account struct {
	Currency         string `json:"currency"`
	AvailableBalance string `json:"available_balance"`
	Hold             string `json:"hold"`
	TradingEnabled   bool   `json:"trading_enabled"`
}

// Order represents a Coinbase order with simplified structure
type Order struct {
	ID           string    `json:"id"`
	ProductID    string    `json:"product_id"`
	Side         string    `json:"side"`
	Type         string    `json:"type"`
	Size         string    `json:"size,omitempty"`
	Price        string    `json:"price,omitempty"`
	StopPrice    string    `json:"stop_price,omitempty"`
	LimitPrice   string    `json:"limit_price,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	FilledSize   string    `json:"filled_size"`
	FilledValue  string    `json:"filled_value"`
	AveragePrice string    `json:"average_price"`
}

// TradingRequest represents a unified trading request with optional stop limit
type TradingRequest struct {
	Size       string  `json:"size"`
	Price      string  `json:"price"`
	StopPrice  string  `json:"stop_price,omitempty"`
	LimitPrice string  `json:"limit_price,omitempty"`
	Percentage float64 `json:"percentage,omitempty"`
}

// CreateOrderRequest represents the request body for creating orders
type CreateOrderRequest struct {
	ProductID  string `json:"product_id"`
	Side       string `json:"side"`
	Type       string `json:"type"`
	Size       string `json:"size,omitempty"`
	Price      string `json:"price,omitempty"`
	StopPrice  string `json:"stop_price,omitempty"`
	LimitPrice string `json:"limit_price,omitempty"`
}

// Candle represents a single candle from the Coinbase API
type Candle struct {
	Start  string `json:"start"`
	Low    string `json:"low"`
	High   string `json:"high"`
	Open   string `json:"open"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

// CandlesResponse represents the response from the candles endpoint
type CandlesResponse struct {
	Candles []Candle `json:"candles"`
}

// OrderBookEntry represents a single entry in the order book
type OrderBookEntry struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// OrderBook represents the order book response from Coinbase API
type OrderBook struct {
	Bids []OrderBookEntry `json:"bids"`
	Asks []OrderBookEntry `json:"asks"`
}

// MarketState represents current market information
type MarketState struct {
	ProductID     string    `json:"product_id"`
	BestBid       string    `json:"best_bid"`
	BestAsk       string    `json:"best_ask"`
	Spread        string    `json:"spread"`
	SpreadPercent string    `json:"spread_percent"`
	LastPrice     string    `json:"last_price"`
	Volume24h     string    `json:"volume_24h"`
	OrderBook     OrderBook `json:"order_book"`
	Timestamp     int64     `json:"timestamp"`
}
