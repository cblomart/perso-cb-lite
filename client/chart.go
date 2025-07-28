package client

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"sort"
	"strconv"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

// GenerateChartPNG creates a PNG chart from graph data
func (c *CoinbaseClient) GenerateChartPNG(graphData *GraphData) ([]byte, error) {
	// Validate input data
	if len(graphData.Candles) == 0 {
		return nil, fmt.Errorf("no candle data available")
	}

	// Helper function to parse timestamps consistently
	parseTimestamp := func(timeStr string) (time.Time, error) {
		// Try RFC3339 first
		if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
			return t, nil
		}
		// Try Unix timestamp
		if unixTime, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
			return time.Unix(unixTime, 0), nil
		}
		return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timeStr)
	}

	// Create a new plot
	p := plot.New()
	p.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s)", graphData.Period)
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Price (USD)"

	// Create candlestick data with consistent time parsing
	candles := make(plotter.XYs, 0, len(graphData.Candles))
	for _, candle := range graphData.Candles {
		// Parse timestamp consistently
		timestamp, err := parseTimestamp(candle.Start)
		if err != nil {
			continue
		}

		// Parse all OHLC values
		openPrice, err := strconv.ParseFloat(candle.Open, 64)
		if err != nil {
			continue
		}
		highPrice, err := strconv.ParseFloat(candle.High, 64)
		if err != nil {
			continue
		}
		lowPrice, err := strconv.ParseFloat(candle.Low, 64)
		if err != nil {
			continue
		}
		closePrice, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil {
			continue
		}

		// Only add valid data points
		if openPrice > 0 && highPrice > 0 && lowPrice > 0 && closePrice > 0 {
			candles = append(candles, plotter.XY{
				X: float64(timestamp.Unix()),
				Y: closePrice, // Use close price for positioning
			})
		}
	}

	// Check if we have valid data
	if len(candles) == 0 {
		return nil, fmt.Errorf("no valid candle data after parsing")
	}

	// Debug logging for timeline issues
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		if len(candles) > 0 {
			firstTime := time.Unix(int64(candles[0].X), 0)
			lastTime := time.Unix(int64(candles[len(candles)-1].X), 0)
			fmt.Printf("Chart timeline: %s to %s (%d candles)\n",
				firstTime.Format("2006-01-02 15:04:05"),
				lastTime.Format("2006-01-02 15:04:05"),
				len(candles))

			// Show sample candle timestamps
			if len(candles) >= 3 {
				fmt.Printf("Sample candle timestamps: %s, %s, %s\n",
					time.Unix(int64(candles[0].X), 0).Format("2006-01-02 15:04:05"),
					time.Unix(int64(candles[len(candles)/2].X), 0).Format("2006-01-02 15:04:05"),
					time.Unix(int64(candles[len(candles)-1].X), 0).Format("2006-01-02 15:04:05"))
			}
		}
		if len(graphData.Trades) > 0 {
			firstTrade := time.Unix(graphData.Trades[0].ExecutedAt, 0)
			lastTrade := time.Unix(graphData.Trades[len(graphData.Trades)-1].ExecutedAt, 0)
			fmt.Printf("Trade timeline: %s to %s (%d trades)\n",
				firstTrade.Format("2006-01-02 15:04:05"),
				lastTrade.Format("2006-01-02 15:04:05"),
				len(graphData.Trades))

			// Show sample trade timestamps
			if len(graphData.Trades) >= 3 {
				fmt.Printf("Sample trade timestamps: %s, %s, %s\n",
					time.Unix(graphData.Trades[0].ExecutedAt, 0).Format("2006-01-02 15:04:05"),
					time.Unix(graphData.Trades[len(graphData.Trades)/2].ExecutedAt, 0).Format("2006-01-02 15:04:05"),
					time.Unix(graphData.Trades[len(graphData.Trades)-1].ExecutedAt, 0).Format("2006-01-02 15:04:05"))
			}
		}
		if len(graphData.AccountValues) > 0 {
			firstValue := time.Unix(graphData.AccountValues[0].Timestamp, 0)
			lastValue := time.Unix(graphData.AccountValues[len(graphData.AccountValues)-1].Timestamp, 0)
			fmt.Printf("Account timeline: %s to %s (%d values)\n",
				firstValue.Format("2006-01-02 15:04:05"),
				lastValue.Format("2006-01-02 15:04:05"),
				len(graphData.AccountValues))

			// Show sample account value timestamps
			if len(graphData.AccountValues) >= 3 {
				fmt.Printf("Sample account timestamps: %s, %s, %s\n",
					time.Unix(graphData.AccountValues[0].Timestamp, 0).Format("2006-01-02 15:04:05"),
					time.Unix(graphData.AccountValues[len(graphData.AccountValues)/2].Timestamp, 0).Format("2006-01-02 15:04:05"),
					time.Unix(graphData.AccountValues[len(graphData.AccountValues)-1].Timestamp, 0).Format("2006-01-02 15:04:05"))
			}
		}
	}

	// Sort candles by time to ensure proper drawing
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].X < candles[j].X
	})

	// Create candlestick visualization using lines and points
	for _, candle := range graphData.Candles {
		// Parse timestamp consistently
		timestamp, err := parseTimestamp(candle.Start)
		if err != nil {
			continue
		}

		// Parse OHLC values
		openPrice, _ := strconv.ParseFloat(candle.Open, 64)
		highPrice, _ := strconv.ParseFloat(candle.High, 64)
		lowPrice, _ := strconv.ParseFloat(candle.Low, 64)
		closePrice, _ := strconv.ParseFloat(candle.Close, 64)

		// Determine if candle is bullish (close > open) or bearish (close < open)
		isBullish := closePrice > openPrice

		// Create wick line (high to low)
		wickData := plotter.XYs{
			{X: float64(timestamp.Unix()), Y: highPrice},
			{X: float64(timestamp.Unix()), Y: lowPrice},
		}

		wickLine, err := plotter.NewLine(wickData)
		if err == nil {
			wickLine.Color = color.RGBA{R: 0, G: 0, B: 0, A: 255} // Black wick
			wickLine.Width = vg.Points(1)
			p.Add(wickLine)
		}

		// Create body lines (open to close)
		bodyData := plotter.XYs{
			{X: float64(timestamp.Unix()) - 0.3, Y: openPrice},
			{X: float64(timestamp.Unix()) + 0.3, Y: closePrice},
		}

		bodyLine, err := plotter.NewLine(bodyData)
		if err == nil {
			if isBullish {
				bodyLine.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255} // Green for bullish
			} else {
				bodyLine.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red for bearish
			}
			bodyLine.Width = vg.Points(3) // Thicker body
			p.Add(bodyLine)
		}
	}

	// Add a simple line chart for reference (can be removed if not needed)
	candleLine, err := plotter.NewLine(candles)
	if err == nil {
		candleLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 100} // Semi-transparent blue
		candleLine.Width = vg.Points(0.5)
		p.Add(candleLine)
	}

	// Add trade markers with consistent time parsing
	if len(graphData.Trades) > 0 {
		buyTrades := make(plotter.XYs, 0)
		sellTrades := make(plotter.XYs, 0)

		for _, trade := range graphData.Trades {
			price, _ := strconv.ParseFloat(trade.Price, 64)
			tradePoint := plotter.XY{
				X: float64(trade.ExecutedAt), // Already Unix timestamp
				Y: price,
			}

			if trade.Side == "BUY" {
				buyTrades = append(buyTrades, tradePoint)
			} else {
				sellTrades = append(sellTrades, tradePoint)
			}
		}

		// Add buy markers (green triangles)
		if len(buyTrades) > 0 {
			buyScatter, err := plotter.NewScatter(buyTrades)
			if err == nil {
				buyScatter.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255} // Green
				buyScatter.Shape = draw.TriangleGlyph{}
				buyScatter.Radius = vg.Points(4)
				p.Add(buyScatter)
			}
		}

		// Add sell markers (red triangles)
		if len(sellTrades) > 0 {
			sellScatter, err := plotter.NewScatter(sellTrades)
			if err == nil {
				sellScatter.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red
				sellScatter.Shape = draw.TriangleGlyph{}
				sellScatter.Radius = vg.Points(4)
				p.Add(sellScatter)
			}
		}
	}

	// Add account value line (already Unix timestamps)
	if len(graphData.AccountValues) > 0 {
		// Create secondary plot for account values
		accountData := make(plotter.XYs, len(graphData.AccountValues))
		for i, accountValue := range graphData.AccountValues {
			accountData[i].X = float64(accountValue.Timestamp) // Already Unix timestamp
			accountData[i].Y = accountValue.TotalUSD
		}

		accountLine, err := plotter.NewLine(accountData)
		if err == nil {
			accountLine.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255} // Purple
			accountLine.Width = vg.Points(2)
			accountLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)} // Dashed line
			p.Add(accountLine)
		}
	}

	// Add EMA12 if available and has matching data points
	if len(graphData.Indicators.EMA12) > 0 && len(graphData.Indicators.EMA12) == len(graphData.Candles) {
		ema12Data := make(plotter.XYs, 0, len(candles))
		for i, candle := range graphData.Candles {
			// Parse timestamp the same way as above
			timestamp, err := parseTimestamp(candle.Start)
			if err != nil {
				continue
			}

			ema12Value := graphData.Indicators.EMA12[i]
			if ema12Value > 0 {
				ema12Data = append(ema12Data, plotter.XY{
					X: float64(timestamp.Unix()),
					Y: ema12Value,
				})
			}
		}

		if len(ema12Data) > 0 {
			ema12Line, err := plotter.NewLine(ema12Data)
			if err == nil {
				ema12Line.Color = color.RGBA{R: 255, G: 165, B: 0, A: 255} // Orange
				ema12Line.Width = vg.Points(1.5)
				p.Add(ema12Line)
			}
		}
	}

	// Add EMA26 if available and has matching data points
	if len(graphData.Indicators.EMA26) > 0 && len(graphData.Indicators.EMA26) == len(graphData.Candles) {
		ema26Data := make(plotter.XYs, 0, len(candles))
		for i, candle := range graphData.Candles {
			// Parse timestamp the same way as above
			timestamp, err := parseTimestamp(candle.Start)
			if err != nil {
				continue
			}

			ema26Value := graphData.Indicators.EMA26[i]
			if ema26Value > 0 {
				ema26Data = append(ema26Data, plotter.XY{
					X: float64(timestamp.Unix()),
					Y: ema26Value,
				})
			}
		}

		if len(ema26Data) > 0 {
			ema26Line, err := plotter.NewLine(ema26Data)
			if err == nil {
				ema26Line.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red
				ema26Line.Width = vg.Points(1.5)
				p.Add(ema26Line)
			}
		}
	}

	// Add trade markers (simplified - no trades in basic chart)
	// Trade markers are removed since we're not fetching trade history

	// Add account value line (simplified - no account values in basic chart)
	// Account value line is removed since we're not calculating account values

	// Format X-axis as time with better tick marks
	p.X.Tick.Marker = plot.TimeTicks{Format: "01-02 15:04"}

	// Add legend
	p.Legend.Top = true
	p.Legend.Left = true
	p.Legend.Add("Price", candleLine)
	if len(graphData.Indicators.EMA12) > 0 {
		ema12Line, _ := plotter.NewLine(plotter.XYs{})
		ema12Line.Color = color.RGBA{R: 255, G: 165, B: 0, A: 255}
		p.Legend.Add("EMA12", ema12Line)
	}
	if len(graphData.Indicators.EMA26) > 0 {
		ema26Line, _ := plotter.NewLine(plotter.XYs{})
		ema26Line.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
		p.Legend.Add("EMA26", ema26Line)
	}
	if len(graphData.Trades) > 0 {
		buyScatter, _ := plotter.NewScatter(plotter.XYs{})
		buyScatter.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
		buyScatter.Shape = draw.TriangleGlyph{}
		p.Legend.Add("Buy", buyScatter)

		sellScatter, _ := plotter.NewScatter(plotter.XYs{})
		sellScatter.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
		sellScatter.Shape = draw.TriangleGlyph{}
		p.Legend.Add("Sell", sellScatter)
	}
	if len(graphData.AccountValues) > 0 {
		accountLine, _ := plotter.NewLine(plotter.XYs{})
		accountLine.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
		accountLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
		p.Legend.Add("Account Value", accountLine)
	}

	// Add summary text with actual data validation
	summaryText := fmt.Sprintf("Period: %s | Candles: %d | Trades: %d | Value: $%.2f",
		graphData.Period,
		len(candles),
		len(graphData.Trades),
		graphData.Summary.EndingValue)

	// Update the title to include summary information
	p.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s) - %s", graphData.Period, summaryText)

	// Create the image with specific dimensions
	img := vgimg.New(12*vg.Inch, 8*vg.Inch)
	dc := draw.New(img)
	p.Draw(dc)

	// Convert to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, img.Image())
	if err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}

	return buf.Bytes(), nil
}
