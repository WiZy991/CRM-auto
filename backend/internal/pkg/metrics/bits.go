package metrics

import (
	"math"
	"runtime"
)

// Преобразования float64 <-> uint64 нужны, потому что atomic-типов для
// float в стандартной библиотеке нет: сумма гистограммы хранится как биты.
func float64bits(v float64) uint64 { return math.Float64bits(v) }
func bitsFloat64(v uint64) float64 { return math.Float64frombits(v) }

func runtimeVersion() string { return runtime.Version() }
