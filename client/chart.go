package client

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
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
	// Create a new plot
	p := plot.New()
	p.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s)", graphData.Period)
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Price (USD)"

	// Create candlestick data
	candles := make(plotter.XYs, len(graphData.Candles))
	for i, candle := range graphData.Candles {
		// Parse timestamp
		timestamp, _ := time.Parse(time.RFC3339, candle.Start)
		closePrice, _ := strconv.ParseFloat(candle.Close, 64)

		candles[i].X = float64(timestamp.Unix())
		candles[i].Y = closePrice
	}

	// Add candlestick line
	candleLine, err := plotter.NewLine(candles)
	if err != nil {
		return nil, fmt.Errorf("failed to create candle line: %w", err)
	}
	candleLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 255} // Blue
	candleLine.Width = vg.Points(1)
	p.Add(candleLine)

	// Add EMA12 if available
	if len(graphData.Indicators.EMA12) > 0 {
		ema12Data := make(plotter.XYs, len(graphData.Candles))
		for i, candle := range graphData.Candles {
			timestamp, _ := time.Parse(time.RFC3339, candle.Start)
			ema12Data[i].X = float64(timestamp.Unix())
			ema12Data[i].Y = graphData.Indicators.EMA12[i]
		}

		ema12Line, err := plotter.NewLine(ema12Data)
		if err == nil {
			ema12Line.Color = color.RGBA{R: 255, G: 165, B: 0, A: 255} // Orange
			ema12Line.Width = vg.Points(1.5)
			p.Add(ema12Line)
		}
	}

	// Add EMA26 if available
	if len(graphData.Indicators.EMA26) > 0 {
		ema26Data := make(plotter.XYs, len(graphData.Candles))
		for i, candle := range graphData.Candles {
			timestamp, _ := time.Parse(time.RFC3339, candle.Start)
			ema26Data[i].X = float64(timestamp.Unix())
			ema26Data[i].Y = graphData.Indicators.EMA26[i]
		}

		ema26Line, err := plotter.NewLine(ema26Data)
		if err == nil {
			ema26Line.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red
			ema26Line.Width = vg.Points(1.5)
			p.Add(ema26Line)
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

	// Add account value line (secondary Y-axis)
	if len(graphData.AccountValues) > 0 {
		// Create secondary plot for account values
		accountData := make(plotter.XYs, len(graphData.AccountValues))
		for i, accountValue := range graphData.AccountValues {
			accountData[i].X = float64(accountValue.Timestamp)
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

	// Format X-axis as time
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
