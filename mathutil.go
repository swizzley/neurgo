package neurgo

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"
)

type NormalizeParams struct {
	SourceRangeStart float64
	SourceRangeEnd   float64
	TargetRangeStart float64
	TargetRangeEnd   float64
}

func NormalizeInRange(params NormalizeParams, value float64) float64 {

	// Warning: this function makes a lot of assumpts about the
	// values being passed, and will only work for those values
	// or values that are very similar.

	// shift the value to the left, so that instead of having a
	// range between 0 and 31 (for example), it will have a range
	// of -15.5 - 15.5
	sourceRangeDelta := params.SourceRangeEnd - params.SourceRangeStart
	halfDelta := sourceRangeDelta / 2.0
	value = value - halfDelta

	// now figure out the scaling factor between the source and target range
	targetRangeDelta := params.TargetRangeEnd - params.TargetRangeStart
	scalingFactor := targetRangeDelta / sourceRangeDelta

	// and scale the value by that scaling factor
	value = value * scalingFactor

	return value
}

func SafeScalarInverse(x float64) float64 {
	if x == 0 {
		x += 0.000000001
	}
	return 1.0 / x
}

// http://en.wikipedia.org/wiki/Residual_sum_of_squares
func SumOfSquaresError(expected []float64, actual []float64) float64 {

	result := float64(0)
	if len(expected) != len(actual) {
		msg := fmt.Sprintf("vector lengths dont match (%d != %d)", len(expected), len(actual))
		panic(msg)
	}

	for i, expectedVal := range expected {
		actualVal := actual[i]
		delta := actualVal - expectedVal
		deltaSquared := math.Pow(delta, 2)
		result += deltaSquared
	}

	return result
}

func EqualsWithMaxDelta(x, y, maxDelta float64) bool {
	delta := math.Abs(x - y)
	return delta <= maxDelta
}

func vectorEqualsWithMaxDelta(xValues, yValues []float64, maxDelta float64) bool {
	equals := true
	for i, x := range xValues {
		y := yValues[i]
		if !EqualsWithMaxDelta(x, y, maxDelta) {
			equals = false
		}
	}
	return equals
}

func VectorEquals(xValues, yValues []float64) bool {
	for i, x := range xValues {
		y := yValues[i]
		if x != y {
			return false
		}
	}
	return true
}

func IntModuloProper(x, y int) bool {
	if x > 0 && math.Mod(float64(x), float64(y)) == 0 {
		return true
	}
	return false
}

func RandomInRange(min, max float64) float64 {

	return rand.Float64()*(max-min) + min
}

// return a random number between min and max - 1
// eg, if you call it with 0,1 it will always return 0
// if you call it between 0,2 it will return 0 or 1
func RandomIntInRange(min, max int) int {
	if min == max {
		log.Printf("warn: min==max (%v == %v)", min, max)
		return min
	}
	return rand.Intn(max-min) + min
}

func SeedRandom() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func RandomBias() float64 {
	return RandomInRange(-1*math.Pi, math.Pi)
}

func RandomWeight() float64 {
	return RandomInRange(-1*math.Pi, math.Pi)
}

func RandomWeights(length int) []float64 {
	weights := []float64{}
	for i := 0; i < length; i++ {
		weights = append(weights, RandomInRange(-1*math.Pi, math.Pi))
	}
	return weights
}

func FixedWeights(length int, weight float64) []float64 {
	weights := []float64{}
	for i := 0; i < length; i++ {
		weights = append(weights, weight)
	}
	return weights
}

func Saturate(parameter, lowerBound, upperBound float64) float64 {
	if parameter < lowerBound {
		return lowerBound
	}
	if parameter > upperBound {
		return upperBound
	}
	return parameter
}

func Average(xs []float64) float64 {
	total := float64(0)
	for _, x := range xs {
		total += x
	}
	return total / float64(len(xs))
}
