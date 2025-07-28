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

// GenerateChartPNG creates a PNG chart from graph data
func (c *CoinbaseClient) GenerateChartPNG(graphData *GraphData) ([]byte, error) {
	// Validate input data
	if len(graphData.Candles) == 0 {
		return nil, fmt.Errorf("no candle data available")
	}

	// Create a new plot
	p := plot.New()
	p.Title.Text = fmt.Sprintf("BTC-USDC Trading Chart (%s)", graphData.Period)
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Price (USD)"

	// Create candlestick data with proper time parsing
	candles := make(plotter.XYs, 0, len(graphData.Candles))
	for _, candle := range graphData.Candles {
		// Parse timestamp - try multiple formats
		var timestamp time.Time
		var err error

		// Try RFC3339 first
		timestamp, err = time.Parse(time.RFC3339, candle.Start)
		if err != nil {
			// Try Unix timestamp
			if unixTime, parseErr := strconv.ParseInt(candle.Start, 10, 64); parseErr == nil {
				timestamp = time.Unix(unixTime, 0)
			} else {
				// Skip invalid timestamps
				continue
			}
		}

		closePrice, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil {
			// Skip invalid prices
			continue
		}

		// Only add valid data points
		if closePrice > 0 {
			candles = append(candles, plotter.XY{
				X: float64(timestamp.Unix()),
				Y: closePrice,
			})
		}
	}

	// Check if we have valid data
	if len(candles) == 0 {
		return nil, fmt.Errorf("no valid candle data after parsing")
	}

	// Sort candles by time to ensure proper line drawing
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].X < candles[j].X
	})

	// Add candlestick line
	candleLine, err := plotter.NewLine(candles)
	if err != nil {
		return nil, fmt.Errorf("failed to create candle line: %w", err)
	}
	candleLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 255} // Blue
	candleLine.Width = vg.Points(1)
	p.Add(candleLine)

	// Add EMA12 if available and has matching data points
	if len(graphData.Indicators.EMA12) > 0 && len(graphData.Indicators.EMA12) == len(graphData.Candles) {
		ema12Data := make(plotter.XYs, 0, len(candles))
		for i, candle := range graphData.Candles {
			// Parse timestamp the same way as above
			var timestamp time.Time
			if t, err := time.Parse(time.RFC3339, candle.Start); err == nil {
				timestamp = t
			} else if unixTime, parseErr := strconv.ParseInt(candle.Start, 10, 64); parseErr == nil {
				timestamp = time.Unix(unixTime, 0)
			} else {
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
			var timestamp time.Time
			if t, err := time.Parse(time.RFC3339, candle.Start); err == nil {
				timestamp = t
			} else if unixTime, parseErr := strconv.ParseInt(candle.Start, 10, 64); parseErr == nil {
				timestamp = time.Unix(unixTime, 0)
			} else {
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

	// Add summary text with actual data validation
	summaryText := fmt.Sprintf("Period: %s | Candles: %d | Price Range: $%.2f - $%.2f",
		graphData.Period,
		len(candles),
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
