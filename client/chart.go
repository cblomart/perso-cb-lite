package client

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"sort"
	"strconv"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

// GenerateChartPNG creates a sleek PNG chart with two separate graphs
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

	// Calculate overall time range from all data sources
	var allTimestamps []float64

	// Add candle timestamps
	for _, candle := range graphData.Candles {
		timestamp, err := parseTimestamp(candle.Start)
		if err == nil {
			allTimestamps = append(allTimestamps, float64(timestamp.Unix()))
		}
	}

	// Add trade timestamps
	for _, trade := range graphData.Trades {
		allTimestamps = append(allTimestamps, float64(trade.ExecutedAt))
	}

	// Add account value timestamps
	for _, av := range graphData.AccountValues {
		allTimestamps = append(allTimestamps, float64(av.Timestamp))
	}

	// Calculate overall time range
	var minTime, maxTime float64
	if len(allTimestamps) > 0 {
		minTime = allTimestamps[0]
		maxTime = allTimestamps[0]
		for _, ts := range allTimestamps {
			if ts < minTime {
				minTime = ts
			}
			if ts > maxTime {
				maxTime = ts
			}
		}
	}

	// Create a large image to hold both charts
	img := vgimg.New(12*vg.Inch, 10*vg.Inch)
	dc := draw.New(img)

	// Create top chart (BTC Price and Trades) - takes 70% of height
	topChart := plot.New()
	topChart.Title.Text = fmt.Sprintf("BTC-USDC Price Chart (%s)", graphData.Period)
	topChart.X.Label.Text = "Time"
	topChart.Y.Label.Text = "BTC Price (USD)"

	// Set X-axis range for top chart
	if maxTime > minTime {
		topChart.X.Min = minTime
		topChart.X.Max = maxTime
	}

	// Create candlestick data
	candles := make(plotter.XYs, 0, len(graphData.Candles))
	for _, candle := range graphData.Candles {
		timestamp, err := parseTimestamp(candle.Start)
		if err != nil {
			continue
		}

		openPrice, _ := strconv.ParseFloat(candle.Open, 64)
		highPrice, _ := strconv.ParseFloat(candle.High, 64)
		lowPrice, _ := strconv.ParseFloat(candle.Low, 64)
		closePrice, _ := strconv.ParseFloat(candle.Close, 64)

		if openPrice > 0 && highPrice > 0 && lowPrice > 0 && closePrice > 0 {
			candles = append(candles, plotter.XY{
				X: float64(timestamp.Unix()),
				Y: closePrice,
			})
		}
	}

	if len(candles) == 0 {
		return nil, fmt.Errorf("no valid candle data after parsing")
	}

	// Sort candles by time
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].X < candles[j].X
	})

	// Add candlesticks to top chart
	for _, candle := range graphData.Candles {
		timestamp, err := parseTimestamp(candle.Start)
		if err != nil {
			continue
		}

		openPrice, _ := strconv.ParseFloat(candle.Open, 64)
		highPrice, _ := strconv.ParseFloat(candle.High, 64)
		lowPrice, _ := strconv.ParseFloat(candle.Low, 64)
		closePrice, _ := strconv.ParseFloat(candle.Close, 64)

		isBullish := closePrice > openPrice

		// Wick line (high to low)
		wickData := plotter.XYs{
			{X: float64(timestamp.Unix()), Y: highPrice},
			{X: float64(timestamp.Unix()), Y: lowPrice},
		}
		wickLine, err := plotter.NewLine(wickData)
		if err == nil {
			wickLine.Color = color.RGBA{R: 0, G: 0, B: 0, A: 255}
			wickLine.Width = vg.Points(1)
			topChart.Add(wickLine)
		}

		// Body line (open to close)
		bodyData := plotter.XYs{
			{X: float64(timestamp.Unix()) - 0.3, Y: openPrice},
			{X: float64(timestamp.Unix()) + 0.3, Y: closePrice},
		}
		bodyLine, err := plotter.NewLine(bodyData)
		if err == nil {
			if isBullish {
				bodyLine.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
			} else {
				bodyLine.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
			}
			bodyLine.Width = vg.Points(3)
			topChart.Add(bodyLine)
		}
	}

	// Add price line (semi-transparent)
	priceLine, err := plotter.NewLine(candles)
	if err == nil {
		priceLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 100}
		priceLine.Width = vg.Points(0.5)
		topChart.Add(priceLine)
	}

	// Add EMAs if available
	if len(graphData.Indicators.EMA12) > 0 && len(graphData.Indicators.EMA12) == len(graphData.Candles) {
		ema12Data := make(plotter.XYs, 0, len(candles))
		for i, candle := range graphData.Candles {
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
				ema12Line.Color = color.RGBA{R: 255, G: 165, B: 0, A: 255}
				ema12Line.Width = vg.Points(1.5)
				topChart.Add(ema12Line)
			}
		}
	}

	if len(graphData.Indicators.EMA26) > 0 && len(graphData.Indicators.EMA26) == len(graphData.Candles) {
		ema26Data := make(plotter.XYs, 0, len(candles))
		for i, candle := range graphData.Candles {
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
				ema26Line.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
				ema26Line.Width = vg.Points(1.5)
				topChart.Add(ema26Line)
			}
		}
	}

	// Add trade markers
	if len(graphData.Trades) > 0 {
		buyTrades := make(plotter.XYs, 0)
		sellTrades := make(plotter.XYs, 0)

		for _, trade := range graphData.Trades {
			price, _ := strconv.ParseFloat(trade.Price, 64)
			tradePoint := plotter.XY{
				X: float64(trade.ExecutedAt),
				Y: price,
			}

			if trade.Side == "BUY" {
				buyTrades = append(buyTrades, tradePoint)
			} else {
				sellTrades = append(sellTrades, tradePoint)
			}
		}

		if len(buyTrades) > 0 {
			buyScatter, err := plotter.NewScatter(buyTrades)
			if err == nil {
				buyScatter.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
				buyScatter.Shape = draw.TriangleGlyph{}
				buyScatter.Radius = vg.Points(4)
				topChart.Add(buyScatter)
			}
		}

		if len(sellTrades) > 0 {
			sellScatter, err := plotter.NewScatter(sellTrades)
			if err == nil {
				sellScatter.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
				sellScatter.Shape = draw.TriangleGlyph{}
				sellScatter.Radius = vg.Points(4)
				topChart.Add(sellScatter)
			}
		}
	}

	// Format X-axis as time
	topChart.X.Tick.Marker = plot.TimeTicks{Format: "01-02 15:04"}

	// Add legend to top chart
	topChart.Legend.Top = true
	topChart.Legend.Left = true
	topChart.Legend.Add("Price", priceLine)

	if len(graphData.Indicators.EMA12) > 0 {
		ema12Line, _ := plotter.NewLine(plotter.XYs{})
		ema12Line.Color = color.RGBA{R: 255, G: 165, B: 0, A: 255}
		topChart.Legend.Add("EMA12", ema12Line)
	}

	if len(graphData.Indicators.EMA26) > 0 {
		ema26Line, _ := plotter.NewLine(plotter.XYs{})
		ema26Line.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
		topChart.Legend.Add("EMA26", ema26Line)
	}

	if len(graphData.Trades) > 0 {
		buyScatter, _ := plotter.NewScatter(plotter.XYs{})
		buyScatter.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
		buyScatter.Shape = draw.TriangleGlyph{}
		topChart.Legend.Add("Buy", buyScatter)

		sellScatter, _ := plotter.NewScatter(plotter.XYs{})
		sellScatter.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
		sellScatter.Shape = draw.TriangleGlyph{}
		topChart.Legend.Add("Sell", sellScatter)
	}

	// Create bottom chart (Asset Value Line Chart) - takes 30% of height
	bottomChart := plot.New()
	bottomChart.Title.Text = "Total Asset Value Evolution"
	bottomChart.X.Label.Text = "Time"
	bottomChart.Y.Label.Text = "Asset Value (USD)"

	// Set X-axis range for bottom chart (same as top chart)
	if maxTime > minTime {
		bottomChart.X.Min = minTime
		bottomChart.X.Max = maxTime
	}

	// Create line chart data for asset values
	if len(graphData.AccountValues) > 0 {
		// Group asset values by day for cleaner chart
		dailyValues := make(map[string]float64)
		for _, av := range graphData.AccountValues {
			timestamp := time.Unix(av.Timestamp, 0)
			dayKey := timestamp.Format("2006-01-02")
			if dailyValues[dayKey] == 0 || av.TotalUSD > dailyValues[dayKey] {
				dailyValues[dayKey] = av.TotalUSD
			}
		}

		// Convert to sorted data
		var lineData plotter.XYs
		for dayKey, value := range dailyValues {
			timestamp, _ := time.Parse("2006-01-02", dayKey)
			lineData = append(lineData, plotter.XY{
				X: float64(timestamp.Unix()),
				Y: value,
			})
		}

		// Sort by time
		sort.Slice(lineData, func(i, j int) bool {
			return lineData[i].X < lineData[j].X
		})

		// Add line chart
		line, err := plotter.NewLine(lineData)
		if err == nil {
			line.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
			line.Width = vg.Points(3)
			bottomChart.Add(line)
		}

		// Add scatter points for emphasis
		scatter, err := plotter.NewScatter(lineData)
		if err == nil {
			scatter.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
			scatter.Radius = vg.Points(2)
			bottomChart.Add(scatter)
		}
	}

	// Format X-axis as time for bottom chart
	bottomChart.X.Tick.Marker = plot.TimeTicks{Format: "01-02"}

	// Draw top chart (70% of height)
	topCanvas := draw.Canvas{
		Canvas: dc,
		Rectangle: vg.Rectangle{
			Min: vg.Point{X: 0, Y: 3 * vg.Inch}, // Bottom 30% for bottom chart
			Max: vg.Point{X: 12 * vg.Inch, Y: 10 * vg.Inch},
		},
	}
	topChart.Draw(topCanvas)

	// Draw bottom chart (30% of height)
	bottomCanvas := draw.Canvas{
		Canvas: dc,
		Rectangle: vg.Rectangle{
			Min: vg.Point{X: 0, Y: 0},
			Max: vg.Point{X: 12 * vg.Inch, Y: 3 * vg.Inch},
		},
	}
	bottomChart.Draw(bottomCanvas)

	// Add overall title with asset value information to top chart
	if len(graphData.AccountValues) > 0 {
		firstValue := graphData.AccountValues[0].TotalUSD
		lastValue := graphData.AccountValues[len(graphData.AccountValues)-1].TotalUSD
		valueChange := lastValue - firstValue
		valueChangePct := (valueChange / firstValue) * 100

		topChart.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s) - Asset Value: $%.2f → $%.2f (%.1f%%)",
			graphData.Period, firstValue, lastValue, valueChangePct)
	}

	// Convert to PNG bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, img.Image())
	if err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}

	return buf.Bytes(), nil
}
