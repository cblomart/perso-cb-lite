package client

import (
	"context"
	"math"
	"strconv"
	"sync"
)

// calculateEMA calculates Exponential Moving Average with optimized performance
func calculateEMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}

	// Use more efficient EMA calculation
	multiplier := 2.0 / float64(period+1)

	// Start with SMA of first 'period' prices for better accuracy
	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	ema := sum / float64(period)

	// Calculate EMA for remaining prices
	for i := period; i < len(prices); i++ {
		ema = (prices[i] * multiplier) + (ema * (1 - multiplier))
	}

	return ema
}

// calculateMACD calculates MACD and Signal line with optimized performance
func calculateMACD(prices []float64) (float64, float64) {
	if len(prices) < 26 {
		return 0, 0
	}

	// Calculate EMA12 and EMA26 for the entire dataset (more efficient)
	ema12 := calculateEMA(prices, 12)
	ema26 := calculateEMA(prices, 26)
	macd := ema12 - ema26

	// For signal line, we only need MACD values from position 26 onwards
	// Calculate MACD values more efficiently by reusing EMA calculations
	macdValues := make([]float64, 0, len(prices)-26)

	// Use sliding window approach for better performance
	for i := 26; i < len(prices); i++ {
		// Calculate EMA12 and EMA26 for the window ending at position i
		windowPrices := prices[:i+1]
		windowEMA12 := calculateEMA(windowPrices, 12)
		windowEMA26 := calculateEMA(windowPrices, 26)
		macdValues = append(macdValues, windowEMA12-windowEMA26)
	}

	// Calculate signal line as EMA9 of MACD values
	signalLine := calculateEMA(macdValues, 9)
	return macd, signalLine
}

// calculateRSI calculates Relative Strength Index with optimized performance
func calculateRSI(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 50 // Neutral RSI if not enough data
	}

	// Calculate initial gains and losses more efficiently
	var gains, losses float64
	for i := 1; i <= period; i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains += change
		} else {
			losses += math.Abs(change)
		}
	}

	// Handle edge case
	if losses == 0 {
		return 100
	}

	// Calculate initial averages
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// Use exponential smoothing for better performance
	multiplier := 1.0 / float64(period)

	// Calculate RSI for remaining periods
	for i := period + 1; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		var gain, loss float64

		if change > 0 {
			gain = change
		} else {
			loss = math.Abs(change)
		}

		avgGain = (avgGain * (1 - multiplier)) + (gain * multiplier)
		avgLoss = (avgLoss * (1 - multiplier)) + (loss * multiplier)
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateADX calculates Average Directional Index
func calculateADX(highs, lows []float64, period int) float64 {
	if len(highs) < period+1 || len(lows) < period+1 {
		return 0
	}

	var plusDM, minusDM, trueRange float64

	for i := 1; i <= period; i++ {
		// True Range
		tr1 := highs[i] - lows[i]
		tr2 := math.Abs(highs[i] - highs[i-1])
		tr3 := math.Abs(lows[i] - lows[i-1])
		tr := math.Max(tr1, math.Max(tr2, tr3))
		trueRange += tr

		// Directional Movement
		upMove := highs[i] - highs[i-1]
		downMove := lows[i-1] - lows[i]

		if upMove > downMove && upMove > 0 {
			plusDM += upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM += downMove
		}
	}

	if trueRange == 0 {
		return 0
	}

	plusDI := (plusDM / trueRange) * 100
	minusDI := (minusDM / trueRange) * 100
	dx := math.Abs(plusDI-minusDI) / (plusDI + minusDI) * 100

	return dx
}

// calculatePriceDropPct calculates percentage change over specified period
func calculatePriceDropPct(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 0
	}

	oldPrice := prices[len(prices)-period-1]
	newPrice := prices[len(prices)-1]

	if oldPrice == 0 {
		return 0
	}

	return ((newPrice - oldPrice) / oldPrice) * 100
}

// detectVolumeSpike detects if last candle volume is > 2x average volume
func detectVolumeSpike(volumes []float64) (bool, float64, float64) {
	if len(volumes) < 2 {
		return false, 0, 0
	}

	lastVolume := volumes[len(volumes)-1]

	// Calculate average volume excluding the last candle
	var sum float64
	for i := 0; i < len(volumes)-1; i++ {
		sum += volumes[i]
	}
	averageVolume := sum / float64(len(volumes)-1)

	volumeSpike := lastVolume > (averageVolume * 2)
	return volumeSpike, averageVolume, lastVolume
}

// calculateTechnicalIndicatorsParallel calculates all technical indicators in parallel with early termination
func calculateTechnicalIndicatorsParallel(candles []Candle) TechnicalIndicators {
	if len(candles) < 50 { // Reduced minimum for lightweight mode
		return TechnicalIndicators{}
	}

	// Extract prices and volumes
	prices := make([]float64, len(candles))
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	volumes := make([]float64, len(candles))

	for i, candle := range candles {
		close, _ := strconv.ParseFloat(candle.Close, 64)
		high, _ := strconv.ParseFloat(candle.High, 64)
		low, _ := strconv.ParseFloat(candle.Low, 64)
		volume, _ := strconv.ParseFloat(candle.Volume, 64)

		prices[i] = close
		highs[i] = high
		lows[i] = low
		volumes[i] = volume
	}

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create channels for streaming results
	type indicatorResult struct {
		name  string
		value interface{}
	}
	resultChan := make(chan indicatorResult, 12) // Buffer for all indicators

	// Create channels for early signal detection
	signalChan := make(chan bool, 1)
	indicatorsChan := make(chan TechnicalIndicators, 1)

	// Calculate indicators in parallel with early termination
	var wg sync.WaitGroup

	// MACD and Signal Line (high priority - often triggers first)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			macd, signalLine := calculateMACD(prices)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"macd", macd}:
			}
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"signalLine", signalLine}:
			}
		}
	}()

	// EMA12 (high priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			ema12 := calculateEMA(prices, 12)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"ema12", ema12}:
			}
		}
	}()

	// EMA26 (high priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			ema26 := calculateEMA(prices, 26)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"ema26", ema26}:
			}
		}
	}()

	// EMA200 (lower priority - takes longer)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			ema200 := calculateEMA(prices, 200)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"ema200", ema200}:
			}
		}
	}()

	// RSI (medium priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			rsi := calculateRSI(prices, 14)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"rsi", rsi}:
			}
		}
	}()

	// ADX (medium priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			adx := calculateADX(highs, lows, 14)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"adx", adx}:
			}
		}
	}()

	// Price drop percentage over 12 hours (for trend change detection)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			priceDropPeriod := 144 // 12 hours for 5-minute candles (144 * 5 minutes = 720 minutes = 12 hours)
			priceDropPct4h := calculatePriceDropPct(prices, priceDropPeriod)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"priceDropPct12h", priceDropPct4h}:
			}
		}
	}()

	// Volume Spike Detection (medium priority)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			volumeSpike, avgVolume, lastVolume := detectVolumeSpike(volumes)
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"volumeSpike", volumeSpike}:
			}
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"averageVolume", avgVolume}:
			}
			select {
			case <-ctx.Done():
				return
			case resultChan <- indicatorResult{"lastVolume", lastVolume}:
			}
		}
	}()

	// Stream processor that checks for signals as they arrive
	go func() {
		indicators := TechnicalIndicators{
			CurrentPrice: prices[len(prices)-1],
		}
		completedIndicators := 0
		totalIndicators := 8 // Total number of indicator groups

		for result := range resultChan {
			// Store the result
			switch result.name {
			case "macd":
				indicators.MACD = result.value.(float64)
			case "signalLine":
				indicators.SignalLine = result.value.(float64)
			case "ema12":
				indicators.EMA12 = result.value.(float64)
			case "ema26":
				indicators.EMA26 = result.value.(float64)
			case "ema200":
				indicators.EMA200 = result.value.(float64)
			case "rsi":
				indicators.RSI = result.value.(float64)
			case "adx":
				indicators.ADX = result.value.(float64)
			case "priceDropPct12h":
				indicators.PriceDropPct12h = result.value.(float64)
			case "volumeSpike":
				indicators.VolumeSpike = result.value.(bool)
			case "averageVolume":
				indicators.AverageVolume = result.value.(float64)
			case "lastVolume":
				indicators.LastVolume = result.value.(float64)
			}

			// Check if we have enough indicators to detect a signal
			if completedIndicators < totalIndicators {
				completedIndicators++
			}

			// Check for early signal detection (after we have key indicators)
			if completedIndicators >= 4 { // Check after we have MACD, EMA12, EMA26, RSI
				bearishSignal, _ := checkBearishSignals(indicators)
				if bearishSignal {
					// Signal detected! Cancel other calculations and send result
					cancel()
					select {
					case signalChan <- true:
					default:
					}
					select {
					case indicatorsChan <- indicators:
					default:
					}
					return
				}
			}
		}

		// All calculations completed, send final result
		select {
		case signalChan <- false:
		default:
		}
		select {
		case indicatorsChan <- indicators:
		default:
		}
	}()

	// Wait for all calculations to complete or early termination
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Wait for result
	select {
	case <-signalChan:
		return <-indicatorsChan
	}
}

// calculateTechnicalIndicators calculates all technical indicators from candle data
func calculateTechnicalIndicators(candles []Candle) TechnicalIndicators {
	// Use parallel calculation for better performance
	return calculateTechnicalIndicatorsParallel(candles)
}

// checkBearishSignals checks if any bearish trend change signals are triggered
func checkBearishSignals(indicators TechnicalIndicators) (bool, []string) {
	var triggers []string

	// Strong bearish MACD crossover (trend change signal)
	if indicators.MACD < indicators.SignalLine && indicators.MACD < 0 {
		triggers = append(triggers, "MACD_BEARISH_CROSSOVER")
	}

	// EMA12 crosses below EMA26 (trend reversal signal)
	if indicators.EMA12 < indicators.EMA26 {
		triggers = append(triggers, "EMA_BEARISH_CROSSOVER")
	}

	// RSI momentum breakdown (trend change signal)
	if indicators.RSI < 40 && indicators.RSI < 50 {
		triggers = append(triggers, "RSI_MOMENTUM_BREAKDOWN")
	}

	// Significant price drop (trend reversal signal)
	if indicators.PriceDropPct12h < -5 {
		triggers = append(triggers, "PRICE_TREND_REVERSAL")
	}

	// Price breaks below EMA200 with momentum (major trend change)
	if indicators.CurrentPrice < indicators.EMA200 && indicators.RSI < 45 {
		triggers = append(triggers, "MAJOR_TREND_BREAKDOWN")
	}

	// Strong bearish trend with volume confirmation
	if indicators.ADX > 25 && indicators.MACD < indicators.SignalLine && indicators.VolumeSpike {
		triggers = append(triggers, "STRONG_BEARISH_TREND")
	}

	// Multiple bearish signals confirming trend change
	bearishCount := 0
	if indicators.MACD < indicators.SignalLine {
		bearishCount++
	}
	if indicators.EMA12 < indicators.EMA26 {
		bearishCount++
	}
	if indicators.RSI < 45 {
		bearishCount++
	}
	if indicators.CurrentPrice < indicators.EMA200 {
		bearishCount++
	}

	// If 3+ bearish signals align, it's a trend change
	if bearishCount >= 3 {
		triggers = append(triggers, "MULTIPLE_BEARISH_SIGNALS")
	}

	return len(triggers) > 0, triggers
}
