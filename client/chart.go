package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"image/color"
	"image/png"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

// GenerateChartPNG creates a PNG chart using Plotly with proper dual Y-axes
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

	// Prepare candlestick data for Plotly
	var candlestickData []map[string]interface{}
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
			candlestickData = append(candlestickData, map[string]interface{}{
				"x":     timestamp.Format("2006-01-02T15:04:05Z07:00"),
				"open":  openPrice,
				"high":  highPrice,
				"low":   lowPrice,
				"close": closePrice,
			})
		}
	}

	// Prepare asset value data
	var assetData []map[string]interface{}
	if len(graphData.AccountValues) > 0 {
		for _, av := range graphData.AccountValues {
			timestamp := time.Unix(av.Timestamp, 0)
			assetData = append(assetData, map[string]interface{}{
				"x": timestamp.Format("2006-01-02T15:04:05Z07:00"),
				"y": av.TotalUSD,
			})
		}
	}

	// Prepare trade data
	var buyTrades, sellTrades []map[string]interface{}
	if len(graphData.Trades) > 0 {
		for _, trade := range graphData.Trades {
			timestamp := time.Unix(trade.ExecutedAt, 0)
			price, _ := strconv.ParseFloat(trade.Price, 64)

			tradePoint := map[string]interface{}{
				"x": timestamp.Format("2006-01-02T15:04:05Z07:00"),
				"y": price,
			}

			if trade.Side == "BUY" {
				buyTrades = append(buyTrades, tradePoint)
			} else {
				sellTrades = append(sellTrades, tradePoint)
			}
		}
	}

	// Create Plotly HTML template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <title>BTC-USDC Trading Chart</title>
    <script src="https://cdn.plot.ly/plotly-latest.min.js"></script>
    <style>
        body { margin: 0; padding: 20px; background: white; font-family: Arial, sans-serif; }
        .chart-container { width: 1200px; height: 800px; margin: 0 auto; }
        .title { text-align: center; margin-bottom: 20px; font-size: 18px; font-weight: bold; }
    </style>
</head>
<body>
    <div class="title">{{.Title}}</div>
    <div class="chart-container" id="chart"></div>
    <script>
        const candlestickData = {{.CandlestickData}};
        const assetData = {{.AssetData}};
        const buyTrades = {{.BuyTrades}};
        const sellTrades = {{.SellTrades}};

        // Prepare traces
        const traces = [];

        // Candlestick trace
        if (candlestickData.length > 0) {
            traces.push({
                x: candlestickData.map(d => d.x),
                open: candlestickData.map(d => d.open),
                high: candlestickData.map(d => d.high),
                low: candlestickData.map(d => d.low),
                close: candlestickData.map(d => d.close),
                type: 'candlestick',
                name: 'BTC Price',
                yaxis: 'y',
                increasing: {line: {color: '#00ff00'}, fillcolor: '#00ff00'},
                decreasing: {line: {color: '#ff0000'}, fillcolor: '#ff0000'},
            });
        }

        // Asset value trace
        if (assetData.length > 0) {
            traces.push({
                x: assetData.map(d => d.x),
                y: assetData.map(d => d.y),
                type: 'scatter',
                mode: 'lines',
                name: 'Asset Value',
                yaxis: 'y2',
                line: {
                    color: '#800080',
                    dash: 'dash',
                    width: 2
                },
                fill: 'none'
            });
        }

        // Buy trades
        if (buyTrades.length > 0) {
            traces.push({
                x: buyTrades.map(d => d.x),
                y: buyTrades.map(d => d.y),
                type: 'scatter',
                mode: 'markers',
                name: 'Buy Trades',
                yaxis: 'y',
                marker: {
                    color: '#00ff00',
                    symbol: 'triangle-up',
                    size: 10
                }
            });
        }

        // Sell trades
        if (sellTrades.length > 0) {
            traces.push({
                x: sellTrades.map(d => d.x),
                y: sellTrades.map(d => d.y),
                type: 'scatter',
                mode: 'markers',
                name: 'Sell Trades',
                yaxis: 'y',
                marker: {
                    color: '#ff0000',
                    symbol: 'triangle-down',
                    size: 10
                }
            });
        }

        const layout = {
            title: '{{.Title}}',
            width: 1200,
            height: 800,
            xaxis: {
                title: 'Time',
                type: 'date'
            },
            yaxis: {
                title: 'BTC Price (USD)',
                side: 'left',
                showgrid: true
            },
            yaxis2: {
                title: 'Asset Value (USD)',
                side: 'right',
                overlaying: 'y',
                showgrid: false
            },
            legend: {
                x: 0,
                y: 1
            },
            margin: {
                l: 80,
                r: 80,
                t: 80,
                b: 80
            }
        };

        const config = {
            responsive: true,
            displayModeBar: false
        };

        Plotly.newPlot('chart', traces, layout, config);
    </script>
</body>
</html>`

	// Prepare template data
	title := fmt.Sprintf("BTC-USDC Trading Chart (%s)", graphData.Period)
	if len(graphData.AccountValues) > 0 {
		firstValue := graphData.AccountValues[0].TotalUSD
		lastValue := graphData.AccountValues[len(graphData.AccountValues)-1].TotalUSD
		valueChange := lastValue - firstValue
		valueChangePct := (valueChange / firstValue) * 100
		title = fmt.Sprintf("BTC-USDC Trading Chart (%s) - Asset Value: $%.2f → $%.2f (%.1f%%)",
			graphData.Period, firstValue, lastValue, valueChangePct)
	}

	// Convert data to JSON strings
	candlestickJSON, _ := json.Marshal(candlestickData)
	assetJSON, _ := json.Marshal(assetData)
	buyTradesJSON, _ := json.Marshal(buyTrades)
	sellTradesJSON, _ := json.Marshal(sellTrades)

	// Create template data
	templateData := map[string]interface{}{
		"Title":           title,
		"CandlestickData": string(candlestickJSON),
		"AssetData":       string(assetJSON),
		"BuyTrades":       string(buyTradesJSON),
		"SellTrades":      string(sellTradesJSON),
	}

	// Parse and execute template
	tmpl, err := template.New("chart").Parse(htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Convert HTML to PNG using Chromedp
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Set timeout
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var pngData []byte
	err = chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+fmt.Sprintf("%x", htmlBuffer.Bytes())),
		chromedp.WaitReady("chart"),
		chromedp.Sleep(2*time.Second), // Wait for chart to render
		chromedp.CaptureScreenshot(&pngData),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to generate PNG: %w", err)
	}

	return pngData, nil
}

// GenerateDualAxisChartPNG creates a PNG chart with proper dual Y-axes
func (c *CoinbaseClient) GenerateDualAxisChartPNG(graphData *GraphData) ([]byte, error) {
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

	// Create a single plot
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

	// Find price range for scaling
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

	// Find asset value range
	var minAsset, maxAsset float64
	if len(graphData.AccountValues) > 0 {
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
	}

	// Calculate scaling factors for dual Y-axis
	priceRange := maxPrice - minPrice
	assetRange := maxAsset - minAsset

	// Scale asset values to be on the same Y-axis as prices
	// We'll use a transformation that maps asset values to a different scale
	var assetScaleFactor float64
	var assetOffset float64

	if assetRange > 0 {
		// Scale asset values to be in the upper portion of the price range
		// This creates a visual separation while keeping them on the same axis
		assetScaleFactor = (priceRange * 0.3) / assetRange // Use 30% of price range
		assetOffset = maxPrice * 0.7                       // Position in upper 30% of chart
	} else {
		assetScaleFactor = 1.0
		assetOffset = maxPrice * 0.8
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

		// Wick line
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

		// Body line
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

	// Add price line
	priceLine, err := plotter.NewLine(candles)
	if err == nil {
		priceLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 100}
		priceLine.Width = vg.Points(0.5)
		p.Add(priceLine)
	}

	// Add EMAs
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

	// Add scaled asset values (dual Y-axis effect)
	if len(graphData.AccountValues) > 0 {
		assetData := make(plotter.XYs, len(graphData.AccountValues))
		for i, accountValue := range graphData.AccountValues {
			assetData[i].X = float64(accountValue.Timestamp)
			// Scale asset value to be visible on the same axis
			assetData[i].Y = (accountValue.TotalUSD * assetScaleFactor) + assetOffset
		}

		assetLine, err := plotter.NewLine(assetData)
		if err == nil {
			assetLine.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
			assetLine.Width = vg.Points(2)
			assetLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
			p.Add(assetLine)
		}

		// Debug logging for asset values
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			fmt.Printf("Asset plot: %d data points, original range: $%.2f - $%.2f, scaled range: %.2f - %.2f\n",
				len(assetData),
				graphData.AccountValues[0].TotalUSD,
				graphData.AccountValues[len(graphData.AccountValues)-1].TotalUSD,
				assetData[0].Y,
				assetData[len(assetData)-1].Y)
		}
	} else {
		// Debug logging when no asset values
		if os.Getenv("LOG_LEVEL") == "DEBUG" {
			fmt.Printf("Asset plot: No asset values available\n")
		}
	}

	// Debug logging for price data
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		fmt.Printf("Price plot: %d candles, range: $%.2f - $%.2f\n",
			len(candles),
			candles[0].Y,
			candles[len(candles)-1].Y)
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
	if len(graphData.AccountValues) > 0 {
		assetLine, _ := plotter.NewLine(plotter.XYs{})
		assetLine.Color = color.RGBA{R: 128, G: 0, B: 128, A: 255}
		assetLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
		p.Legend.Add("Asset Value", assetLine)
	}

	// Add title with asset value information
	if len(graphData.AccountValues) > 0 {
		firstValue := graphData.AccountValues[0].TotalUSD
		lastValue := graphData.AccountValues[len(graphData.AccountValues)-1].TotalUSD
		valueChange := lastValue - firstValue
		valueChangePct := (valueChange / firstValue) * 100

		p.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s) - Asset Value: $%.2f → $%.2f (%.1f%%)",
			graphData.Period, firstValue, lastValue, valueChangePct)
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
