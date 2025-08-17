package decimal

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMoney(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"from float", 123.45, "123.45"},
		{"from int", int64(100), "100.00"},
		{"from string", "99.99", "99.99"},
		{"from zero", 0.0, "0.00"},
		{"from negative", -50.25, "-50.25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Money
			switch v := tt.input.(type) {
			case float64:
				m = NewMoney(v)
			case int64:
				m = NewMoneyFromInt(v)
			case string:
				var err error
				m, err = NewMoneyFromString(v)
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expected, m.String())
		})
	}
}

func TestMoneyArithmetic(t *testing.T) {
	t.Run("Addition", func(t *testing.T) {
		a := NewMoney(10.50)
		b := NewMoney(5.25)
		result := a.Add(b)
		assert.Equal(t, "15.75", result.String())
	})

	t.Run("Subtraction", func(t *testing.T) {
		a := NewMoney(20.00)
		b := NewMoney(7.50)
		result := a.Sub(b)
		assert.Equal(t, "12.50", result.String())
	})

	t.Run("Multiplication", func(t *testing.T) {
		m := NewMoney(15.00)
		result := m.MulFloat(1.5)
		assert.Equal(t, "22.50", result.String())
	})

	t.Run("Division", func(t *testing.T) {
		m := NewMoney(100.00)
		result := m.DivFloat(4)
		assert.Equal(t, "25.00", result.String())
	})
}

func TestMoneyComparison(t *testing.T) {
	a := NewMoney(10.00)
	b := NewMoney(20.00)
	c := NewMoney(10.00)

	assert.True(t, a.LessThan(b))
	assert.True(t, b.GreaterThan(a))
	assert.True(t, a.Equal(c))
	assert.True(t, a.NotEqual(b))
	assert.True(t, a.LessThanOrEqual(b))
	assert.True(t, a.LessThanOrEqual(c))
	assert.True(t, b.GreaterThanOrEqual(a))
	assert.True(t, b.GreaterThanOrEqual(b))
}

func TestMoneyProperties(t *testing.T) {
	zero := Zero()
	positive := NewMoney(10.00)
	negative := NewMoney(-10.00)

	assert.True(t, zero.IsZero())
	assert.False(t, positive.IsZero())

	assert.True(t, negative.IsNegative())
	assert.False(t, positive.IsNegative())

	assert.True(t, positive.IsPositive())
	assert.False(t, negative.IsPositive())
	assert.False(t, zero.IsPositive())
}

func TestMoneyRounding(t *testing.T) {
	m := NewMoney(10.5678)

	assert.Equal(t, "10.57", m.Round(2).String())
	assert.Equal(t, "10.568", m.Round(3).StringFixed(3))

	// Test banker's rounding
	m1 := NewMoney(2.545)
	m2 := NewMoney(2.535)
	assert.Equal(t, "2.54", m1.RoundBank(2).String())
	assert.Equal(t, "2.54", m2.RoundBank(2).String())

	// Test floor and ceil
	m3 := NewMoney(10.75)
	assert.Equal(t, "10.00", m3.Floor().String())
	assert.Equal(t, "11.00", m3.Ceil().String())
}

func TestMoneyJSON(t *testing.T) {
	t.Run("Marshal", func(t *testing.T) {
		m := NewMoney(123.45)
		data, err := json.Marshal(m)
		require.NoError(t, err)
		assert.Equal(t, `"123.45"`, string(data))
	})

	t.Run("Unmarshal from string", func(t *testing.T) {
		var m Money
		err := json.Unmarshal([]byte(`"123.45"`), &m)
		require.NoError(t, err)
		assert.Equal(t, "123.45", m.String())
	})

	t.Run("Unmarshal from number", func(t *testing.T) {
		var m Money
		err := json.Unmarshal([]byte(`123.45`), &m)
		require.NoError(t, err)
		assert.Equal(t, "123.45", m.String())
	})

	t.Run("Unmarshal null", func(t *testing.T) {
		var m Money
		err := json.Unmarshal([]byte(`null`), &m)
		require.NoError(t, err)
		assert.True(t, m.IsZero())
	})
}

func TestMoneyDatabase(t *testing.T) {
	t.Run("Value", func(t *testing.T) {
		m := NewMoney(123.45)
		value, err := m.Value()
		require.NoError(t, err)
		assert.Equal(t, "123.45", value)
	})

	t.Run("Scan string", func(t *testing.T) {
		var m Money
		err := m.Scan("123.45")
		require.NoError(t, err)
		assert.Equal(t, "123.45", m.String())
	})

	t.Run("Scan bytes", func(t *testing.T) {
		var m Money
		err := m.Scan([]byte("123.45"))
		require.NoError(t, err)
		assert.Equal(t, "123.45", m.String())
	})

	t.Run("Scan int64", func(t *testing.T) {
		var m Money
		err := m.Scan(int64(100))
		require.NoError(t, err)
		assert.Equal(t, "100.00", m.String())
	})

	t.Run("Scan float64", func(t *testing.T) {
		var m Money
		err := m.Scan(float64(123.45))
		require.NoError(t, err)
		assert.Equal(t, "123.45", m.String())
	})

	t.Run("Scan nil", func(t *testing.T) {
		var m Money
		err := m.Scan(nil)
		require.NoError(t, err)
		assert.True(t, m.IsZero())
	})

	t.Run("Scan invalid type", func(t *testing.T) {
		var m Money
		err := m.Scan(struct{}{})
		assert.Error(t, err)
	})
}

func TestMoneyHelperFunctions(t *testing.T) {
	t.Run("Min", func(t *testing.T) {
		a := NewMoney(10.00)
		b := NewMoney(20.00)
		assert.Equal(t, a, Min(a, b))
		assert.Equal(t, a, Min(b, a))
	})

	t.Run("Max", func(t *testing.T) {
		a := NewMoney(10.00)
		b := NewMoney(20.00)
		assert.Equal(t, b, Max(a, b))
		assert.Equal(t, b, Max(b, a))
	})

	t.Run("Sum", func(t *testing.T) {
		values := []Money{
			NewMoney(10.00),
			NewMoney(20.00),
			NewMoney(30.00),
		}
		result := Sum(values...)
		assert.Equal(t, "60.00", result.String())
	})

	t.Run("Average", func(t *testing.T) {
		values := []Money{
			NewMoney(10.00),
			NewMoney(20.00),
			NewMoney(30.00),
		}
		result := Average(values...)
		assert.Equal(t, "20.00", result.String())
	})

	t.Run("Average empty", func(t *testing.T) {
		result := Average()
		assert.True(t, result.IsZero())
	})
}

func TestMoneyPercentage(t *testing.T) {
	t.Run("Percentage calculation", func(t *testing.T) {
		m := NewMoney(100.00)
		result := m.Percentage(15)
		assert.Equal(t, "15.00", result.String())
	})

	t.Run("PercentageOf", func(t *testing.T) {
		part := NewMoney(25.00)
		total := NewMoney(100.00)
		percent := part.PercentageOf(total)
		assert.Equal(t, float64(25), percent)
	})

	t.Run("PercentageOf zero", func(t *testing.T) {
		part := NewMoney(25.00)
		total := Zero()
		percent := part.PercentageOf(total)
		assert.Equal(t, float64(0), percent)
	})
}

func TestMoneySplit(t *testing.T) {
	t.Run("Even split", func(t *testing.T) {
		m := NewMoney(100.00)
		parts := m.Split(4)
		require.Len(t, parts, 4)
		for _, part := range parts {
			assert.Equal(t, "25.00", part.String())
		}
	})

	t.Run("Uneven split", func(t *testing.T) {
		m := NewMoney(100.00)
		parts := m.Split(3)
		require.Len(t, parts, 3)

		// First gets the extra penny
		assert.Equal(t, "33.34", parts[0].String())
		assert.Equal(t, "33.33", parts[1].String())
		assert.Equal(t, "33.33", parts[2].String())

		// Total should equal original
		total := Sum(parts...)
		assert.Equal(t, m.String(), total.String())
	})

	t.Run("Split zero", func(t *testing.T) {
		m := NewMoney(100.00)
		parts := m.Split(0)
		assert.Empty(t, parts)
	})
}

func TestMoneyAllocate(t *testing.T) {
	t.Run("Simple allocation", func(t *testing.T) {
		m := NewMoney(100.00)
		ratios := []int{1, 1, 1}
		parts := m.Allocate(ratios)

		require.Len(t, parts, 3)
		assert.Equal(t, "33.33", parts[0].String())
		assert.Equal(t, "33.33", parts[1].String())
		assert.Equal(t, "33.34", parts[2].String()) // Gets rounding difference

		// Total should equal original
		total := Sum(parts...)
		assert.Equal(t, m.String(), total.String())
	})

	t.Run("Weighted allocation", func(t *testing.T) {
		m := NewMoney(100.00)
		ratios := []int{50, 30, 20}
		parts := m.Allocate(ratios)

		require.Len(t, parts, 3)
		assert.Equal(t, "50.00", parts[0].String())
		assert.Equal(t, "30.00", parts[1].String())
		assert.Equal(t, "20.00", parts[2].String())
	})

	t.Run("Empty ratios", func(t *testing.T) {
		m := NewMoney(100.00)
		parts := m.Allocate([]int{})
		assert.Empty(t, parts)
	})

	t.Run("Zero ratios", func(t *testing.T) {
		m := NewMoney(100.00)
		parts := m.Allocate([]int{0, 0, 0})
		require.Len(t, parts, 3)
		for _, part := range parts {
			assert.True(t, part.IsZero())
		}
	})
}

func TestMoneyEdgeCases(t *testing.T) {
	t.Run("Very small values", func(t *testing.T) {
		m := NewMoney(0.001)
		assert.Equal(t, "0.00", m.String())
		assert.Equal(t, "0.001", m.StringFixed(3))
	})

	t.Run("Large values", func(t *testing.T) {
		m := NewMoney(1000000000.99)
		assert.Equal(t, "1000000000.99", m.String())
	})

	t.Run("Precision preservation", func(t *testing.T) {
		// This would fail with float64
		a := NewMoney(0.1)
		b := NewMoney(0.2)
		result := a.Add(b)
		expected := NewMoney(0.3)
		assert.True(t, result.Equal(expected))
	})
}

func BenchmarkMoneyCreation(b *testing.B) {
	b.Run("NewMoney", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewMoney(123.45)
		}
	})

	b.Run("NewMoneyFromString", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = NewMoneyFromString("123.45")
		}
	})
}

func BenchmarkMoneyArithmetic(b *testing.B) {
	m1 := NewMoney(100.00)
	m2 := NewMoney(50.00)

	b.Run("Add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = m1.Add(m2)
		}
	})

	b.Run("Multiply", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = m1.MulFloat(1.5)
		}
	})
}

func BenchmarkMoneyJSON(b *testing.B) {
	m := NewMoney(123.45)
	data, _ := json.Marshal(m)

	b.Run("Marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = json.Marshal(m)
		}
	})

	b.Run("Unmarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var m2 Money
			_ = json.Unmarshal(data, &m2)
		}
	})
}

// Example usage
func ExampleMoney_Split() {
	total := NewMoney(100.00)
	parts := total.Split(3)

	for i, part := range parts {
		fmt.Printf("Part %d: %s\n", i+1, part.String())
	}
	// Output:
	// Part 1: 33.34
	// Part 2: 33.33
	// Part 3: 33.33
}

func ExampleMoney_Allocate() {
	budget := NewMoney(1000.00)
	// Allocate: 50% to development, 30% to marketing, 20% to operations
	allocations := budget.Allocate([]int{50, 30, 20})

	fmt.Printf("Development: %s\n", allocations[0].String())
	fmt.Printf("Marketing: %s\n", allocations[1].String())
	fmt.Printf("Operations: %s\n", allocations[2].String())
	// Output:
	// Development: 500.00
	// Marketing: 300.00
	// Operations: 200.00
}

// Ensure Money implements required interfaces
var (
	_ driver.Valuer    = Money{}
	_ json.Marshaler   = Money{}
	_ json.Unmarshaler = &Money{}
)

// TestNewMoneyFromDecimal tests creating Money from decimal.Decimal
func TestNewMoneyFromDecimal(t *testing.T) {
	d := decimal.NewFromFloat(123.45)
	m := NewMoneyFromDecimal(d)
	assert.Equal(t, "123.45", m.String())
}
