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

// GenerateChartPNG creates a sleek PNG chart with dual Y-axis effect using gonum/plot
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
	p.Y.Label.Text = "BTC Price (USD)"

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

	// Calculate price range for scaling
	var minPrice, maxPrice float64
	if len(candles) > 0 {
		minPrice = candles[0].Y
		maxPrice = candles[0].Y
		for _, candle := range candles {
			if candle.Y < minPrice {
				minPrice = candle.Y
			}
			if candle.Y > maxPrice {
				maxPrice = candle.Y
			}
		}
	}

	// Calculate asset value range and scaling
	var assetScaleFactor float64
	var assetOffset float64
	var assetData plotter.XYs

	if len(graphData.AccountValues) > 0 {
		var minAsset, maxAsset float64
		minAsset = graphData.AccountValues[0].TotalUSD
		maxAsset = graphData.AccountValues[0].TotalUSD

		for _, av := range graphData.AccountValues {
			if av.TotalUSD < minAsset {
				minAsset = av.TotalUSD
			}
			if av.TotalUSD > maxAsset {
				maxAsset = av.TotalUSD
			}
		}

		// Scale asset values to fit in the upper portion of the price range
		priceRange := maxPrice - minPrice
		assetRange := maxAsset - minAsset

		if assetRange > 0 {
			// Use 25% of the price range for asset values
			assetScaleFactor = (priceRange * 0.25) / assetRange
			assetOffset = maxPrice * 0.75 // Position in upper 25%
		} else {
			assetScaleFactor = 1.0
			assetOffset = maxPrice * 0.8
		}

		// Create scaled asset data
		assetData = make(plotter.XYs, len(graphData.AccountValues))
		for i, av := range graphData.AccountValues {
			assetData[i].X = float64(av.Timestamp)
			assetData[i].Y = (av.TotalUSD * assetScaleFactor) + assetOffset
		}
	}

	// Add candlesticks
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
			p.Add(wickLine)
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
			p.Add(bodyLine)
		}
	}

	// Add price line (semi-transparent)
	priceLine, err := plotter.NewLine(candles)
	if err == nil {
		priceLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 100}
		priceLine.Width = vg.Points(0.5)
		p.Add(priceLine)
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
				p.Add(ema12Line)
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
				p.Add(ema26Line)
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
				p.Add(buyScatter)
			}
		}

		if len(sellTrades) > 0 {
			sellScatter, err := plotter.NewScatter(sellTrades)
			if err == nil {
				sellScatter.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
				sellScatter.Shape = draw.TriangleGlyph{}
				sellScatter.Radius = vg.Points(4)
				p.Add(sellScatter)
			}
		}
	}

	// Add scaled asset value line (dual Y-axis effect)
	if len(assetData) > 0 {
		assetLine, err := plotter.NewLine(assetData)
		if err == nil {
			assetLine.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
			assetLine.Width = vg.Points(2)
			assetLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
			p.Add(assetLine)
		}
	}

	// Update title with asset value information
	if len(graphData.AccountValues) > 0 {
		firstValue := graphData.AccountValues[0].TotalUSD
		lastValue := graphData.AccountValues[len(graphData.AccountValues)-1].TotalUSD
		valueChange := lastValue - firstValue
		valueChangePct := (valueChange / firstValue) * 100

		p.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s) - Asset Value: $%.2f → $%.2f (%.1f%%)",
			graphData.Period, firstValue, lastValue, valueChangePct)
	}

	// Format X-axis as time
	p.X.Tick.Marker = plot.TimeTicks{Format: "01-02 15:04"}

	// Add legend
	p.Legend.Top = true
	p.Legend.Left = true
	p.Legend.Add("Price", priceLine)

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

	if len(assetData) > 0 {
		assetLine, _ := plotter.NewLine(plotter.XYs{})
		assetLine.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
		assetLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
		p.Legend.Add("Asset Value", assetLine)
	}

	// Create the image
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
