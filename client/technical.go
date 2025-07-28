package client

import (
	"math"
	"strconv"
)

// calculateEMA calculates Exponential Moving Average
func calculateEMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}

	multiplier := 2.0 / float64(period+1)
	ema := prices[0] // Start with first price

	for i := 1; i < len(prices); i++ {
		ema = (prices[i] * multiplier) + (ema * (1 - multiplier))
	}

	return ema
}

// calculateMACD calculates MACD and Signal line
func calculateMACD(prices []float64) (float64, float64) {
	if len(prices) < 26 {
		return 0, 0
	}

	// Calculate EMA12 and EMA26 for the entire dataset
	ema12 := calculateEMA(prices, 12)
	ema26 := calculateEMA(prices, 26)
	macd := ema12 - ema26

	// Calculate MACD values for signal line calculation
	macdValues := make([]float64, 0)
	for i := 26; i < len(prices); i++ {
		// Calculate EMA12 and EMA26 for each point from 26 onwards
		ema12 := calculateEMA(prices[:i+1], 12)
		ema26 := calculateEMA(prices[:i+1], 26)
		macdValues = append(macdValues, ema12-ema26)
	}

	// Calculate signal line as EMA9 of MACD values
	signalLine := calculateEMA(macdValues, 9)
	return macd, signalLine
}

// calculateRSI calculates Relative Strength Index
func calculateRSI(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 50 // Neutral RSI if not enough data
	}

	var gains, losses float64
	for i := 1; i <= period; i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains += change
		} else {
			losses += math.Abs(change)
		}
	}

	if losses == 0 {
		return 100
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)
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

// calculateTechnicalIndicators calculates all technical indicators from candle data
func calculateTechnicalIndicators(candles []Candle) TechnicalIndicators {
	if len(candles) < 200 {
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

	// Calculate indicators
	macd, signalLine := calculateMACD(prices)
	ema12 := calculateEMA(prices, 12)
	ema26 := calculateEMA(prices, 26)
	ema200 := calculateEMA(prices, 200)
	rsi := calculateRSI(prices, 14)
	adx := calculateADX(highs, lows, 14)
	priceDropPct4h := calculatePriceDropPct(prices, 48) // 4 hours = 48 * 5min candles (from most recent)
	volumeSpike, avgVolume, lastVolume := detectVolumeSpike(volumes)

	return TechnicalIndicators{
		MACD:           macd,
		SignalLine:     signalLine,
		EMA12:          ema12,
		EMA26:          ema26,
		EMA200:         ema200,
		RSI:            rsi,
		ADX:            adx,
		PriceDropPct4h: priceDropPct4h,
		VolumeSpike:    volumeSpike,
		CurrentPrice:   prices[len(prices)-1],
		AverageVolume:  avgVolume,
		LastVolume:     lastVolume,
	}
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
	if indicators.PriceDropPct4h < -5 {
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
