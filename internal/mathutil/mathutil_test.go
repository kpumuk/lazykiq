package mathutil_test

import (
	"testing"

	"github.com/kpumuk/lazykiq/internal/mathutil"
)

func TestClamp_Int(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  int
		low  int
		high int
		want int
	}{
		{
			name: "value within range",
			val:  5,
			low:  0,
			high: 10,
			want: 5,
		},
		{
			name: "value below minimum",
			val:  -5,
			low:  0,
			high: 10,
			want: 0,
		},
		{
			name: "value above maximum",
			val:  15,
			low:  0,
			high: 10,
			want: 10,
		},
		{
			name: "value equals minimum",
			val:  0,
			low:  0,
			high: 10,
			want: 0,
		},
		{
			name: "value equals maximum",
			val:  10,
			low:  0,
			high: 10,
			want: 10,
		},
		{
			name: "negative range",
			val:  -5,
			low:  -10,
			high: -1,
			want: -5,
		},
		{
			name: "value below negative range",
			val:  -15,
			low:  -10,
			high: -1,
			want: -10,
		},
		{
			name: "value above negative range",
			val:  0,
			low:  -10,
			high: -1,
			want: -1,
		},
		{
			name: "single value range",
			val:  5,
			low:  7,
			high: 7,
			want: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mathutil.Clamp(tt.val, tt.low, tt.high)
			if got != tt.want {
				t.Errorf("Clamp(%d, %d, %d) = %d, want %d", tt.val, tt.low, tt.high, got, tt.want)
			}
		})
	}
}

func TestClamp_Float64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  float64
		low  float64
		high float64
		want float64
	}{
		{
			name: "value within range",
			val:  5.5,
			low:  0.0,
			high: 10.0,
			want: 5.5,
		},
		{
			name: "value below minimum",
			val:  -2.5,
			low:  0.0,
			high: 10.0,
			want: 0.0,
		},
		{
			name: "value above maximum",
			val:  15.7,
			low:  0.0,
			high: 10.0,
			want: 10.0,
		},
		{
			name: "fractional bounds",
			val:  5.5,
			low:  2.3,
			high: 7.8,
			want: 5.5,
		},
		{
			name: "value below fractional minimum",
			val:  1.0,
			low:  2.3,
			high: 7.8,
			want: 2.3,
		},
		{
			name: "value above fractional maximum",
			val:  9.0,
			low:  2.3,
			high: 7.8,
			want: 7.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mathutil.Clamp(tt.val, tt.low, tt.high)
			if got != tt.want {
				t.Errorf("Clamp(%f, %f, %f) = %f, want %f", tt.val, tt.low, tt.high, got, tt.want)
			}
		})
	}
}

func TestClamp_Int64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  int64
		low  int64
		high int64
		want int64
	}{
		{
			name: "large values within range",
			val:  1000000,
			low:  0,
			high: 10000000,
			want: 1000000,
		},
		{
			name: "large value below minimum",
			val:  -1000000,
			low:  0,
			high: 10000000,
			want: 0,
		},
		{
			name: "large value above maximum",
			val:  20000000,
			low:  0,
			high: 10000000,
			want: 10000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mathutil.Clamp(tt.val, tt.low, tt.high)
			if got != tt.want {
				t.Errorf("Clamp(%d, %d, %d) = %d, want %d", tt.val, tt.low, tt.high, got, tt.want)
			}
		})
	}
}

// BenchmarkClamp_Int benchmarks the Clamp function with integers.
func BenchmarkClamp_Int(b *testing.B) {
	for i := range b.N {
		_ = mathutil.Clamp(i, 0, 100)
	}
}

// BenchmarkClamp_Float64 benchmarks the Clamp function with floats.
func BenchmarkClamp_Float64(b *testing.B) {
	for i := range b.N {
		_ = mathutil.Clamp(float64(i), 0.0, 100.0)
	}
}
