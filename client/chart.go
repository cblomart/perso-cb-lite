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

	// Add trade markers (simplified - no trades in basic chart)
	// Trade markers are removed since we're not fetching trade history

	// Add account value line (simplified - no account values in basic chart)
	// Account value line is removed since we're not calculating account values

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

	// Add summary text
	summaryText := fmt.Sprintf("Period: %s | Candles: %d | Price Range: $%.2f - $%.2f",
		graphData.Period,
		len(graphData.Candles),
		graphData.Summary.WorstPrice,
		graphData.Summary.BestPrice)

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
