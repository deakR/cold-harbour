package engine

import (
	"math"
	"strconv"
)

func numericInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		if uint64(v) <= math.MaxInt64 {
			return int64(v), true
		}
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v <= math.MaxInt64 {
			return int64(v), true
		}
	case float32:
		f := float64(v)
		if f >= math.MinInt64 && f <= math.MaxInt64 && math.Trunc(f) == f {
			return int64(f), true
		}
	case float64:
		if v >= math.MinInt64 && v <= math.MaxInt64 && math.Trunc(v) == v {
			return int64(v), true
		}
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	}
	return 0, false
}
