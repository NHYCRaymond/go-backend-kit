package decimal

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// Money represents a monetary amount with decimal precision
type Money struct {
	decimal.Decimal
}

// NewMoney creates a new Money instance from a float64
func NewMoney(value float64) Money {
	return Money{decimal.NewFromFloat(value)}
}

// NewMoneyFromString creates a new Money instance from a string
func NewMoneyFromString(value string) (Money, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return Money{}, err
	}
	return Money{d}, nil
}

// NewMoneyFromInt creates a new Money instance from an int64
func NewMoneyFromInt(value int64) Money {
	return Money{decimal.NewFromInt(value)}
}

// NewMoneyFromDecimal creates a new Money instance from a decimal.Decimal
func NewMoneyFromDecimal(d decimal.Decimal) Money {
	return Money{d}
}

// Zero returns a Money instance with value 0
func Zero() Money {
	return Money{decimal.Zero}
}

// Add adds two Money values
func (m Money) Add(other Money) Money {
	return Money{m.Decimal.Add(other.Decimal)}
}

// Sub subtracts two Money values
func (m Money) Sub(other Money) Money {
	return Money{m.Decimal.Sub(other.Decimal)}
}

// Mul multiplies Money by a decimal
func (m Money) Mul(multiplier decimal.Decimal) Money {
	return Money{m.Decimal.Mul(multiplier)}
}

// MulFloat multiplies Money by a float64 value
func (m Money) MulFloat(multiplier float64) Money {
	return Money{m.Decimal.Mul(decimal.NewFromFloat(multiplier))}
}

// Div divides Money by a decimal
func (m Money) Div(divisor decimal.Decimal) Money {
	return Money{m.Decimal.Div(divisor)}
}

// DivFloat divides Money by a float64 value
func (m Money) DivFloat(divisor float64) Money {
	return Money{m.Decimal.Div(decimal.NewFromFloat(divisor))}
}

// GreaterThan checks if this Money is greater than another
func (m Money) GreaterThan(other Money) bool {
	return m.Decimal.GreaterThan(other.Decimal)
}

// GreaterThanOrEqual checks if this Money is greater than or equal to another
func (m Money) GreaterThanOrEqual(other Money) bool {
	return m.Decimal.GreaterThanOrEqual(other.Decimal)
}

// LessThan checks if this Money is less than another
func (m Money) LessThan(other Money) bool {
	return m.Decimal.LessThan(other.Decimal)
}

// LessThanOrEqual checks if this Money is less than or equal to another
func (m Money) LessThanOrEqual(other Money) bool {
	return m.Decimal.LessThanOrEqual(other.Decimal)
}

// Equal checks if two Money values are equal
func (m Money) Equal(other Money) bool {
	return m.Decimal.Equal(other.Decimal)
}

// NotEqual checks if two Money values are not equal
func (m Money) NotEqual(other Money) bool {
	return !m.Decimal.Equal(other.Decimal)
}

// IsZero checks if the Money value is zero
func (m Money) IsZero() bool {
	return m.Decimal.IsZero()
}

// IsNegative checks if the Money value is negative
func (m Money) IsNegative() bool {
	return m.Decimal.IsNegative()
}

// IsPositive checks if the Money value is positive
func (m Money) IsPositive() bool {
	return m.Decimal.IsPositive()
}

// Abs returns the absolute value of Money
func (m Money) Abs() Money {
	return Money{m.Decimal.Abs()}
}

// Neg returns the negative value of Money
func (m Money) Neg() Money {
	return Money{m.Decimal.Neg()}
}

// Round rounds the Money to the specified number of decimal places
func (m Money) Round(places int32) Money {
	return Money{m.Decimal.Round(places)}
}

// RoundBank rounds using banker's rounding (round to nearest even)
func (m Money) RoundBank(places int32) Money {
	return Money{m.Decimal.RoundBank(places)}
}

// Floor returns the largest integer value less than or equal to the Money
func (m Money) Floor() Money {
	return Money{m.Decimal.Floor()}
}

// Ceil returns the smallest integer value greater than or equal to the Money
func (m Money) Ceil() Money {
	return Money{m.Decimal.Ceil()}
}

// ToFloat64 converts Money to float64 (use with caution due to precision loss)
func (m Money) ToFloat64() float64 {
	f, _ := m.Decimal.Float64()
	return f
}

// String returns the string representation with 2 decimal places
func (m Money) String() string {
	return m.Decimal.StringFixed(2)
}

// StringFixed returns the string representation with specified decimal places
func (m Money) StringFixed(places int32) string {
	return m.Decimal.StringFixed(places)
}

// Value implements the driver.Valuer interface for database storage
func (m Money) Value() (driver.Value, error) {
	return m.Decimal.String(), nil
}

// Scan implements the sql.Scanner interface for database retrieval
func (m *Money) Scan(value interface{}) error {
	if value == nil {
		*m = Zero()
		return nil
	}

	switch v := value.(type) {
	case string:
		d, err := decimal.NewFromString(v)
		if err != nil {
			return err
		}
		*m = Money{d}
	case []byte:
		d, err := decimal.NewFromString(string(v))
		if err != nil {
			return err
		}
		*m = Money{d}
	case int64:
		*m = Money{decimal.NewFromInt(v)}
	case float64:
		*m = Money{decimal.NewFromFloat(v)}
	default:
		return fmt.Errorf("cannot scan %T into Money", value)
	}

	return nil
}

// MarshalJSON implements json.Marshaler
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, m.String())), nil
}

// UnmarshalJSON implements json.Unmarshaler
func (m *Money) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	if str == "null" {
		*m = Zero()
		return nil
	}

	// Try to parse as number first
	if f, err := strconv.ParseFloat(str, 64); err == nil {
		*m = NewMoney(f)
		return nil
	}

	// Try to parse as decimal string
	d, err := decimal.NewFromString(str)
	if err != nil {
		return err
	}
	*m = Money{d}
	return nil
}

// Min returns the smaller of two Money values
func Min(a, b Money) Money {
	if a.LessThan(b) {
		return a
	}
	return b
}

// Max returns the larger of two Money values
func Max(a, b Money) Money {
	if a.GreaterThan(b) {
		return a
	}
	return b
}

// Sum calculates the sum of multiple Money values
func Sum(values ...Money) Money {
	sum := Zero()
	for _, v := range values {
		sum = sum.Add(v)
	}
	return sum
}

// Average calculates the average of multiple Money values
func Average(values ...Money) Money {
	if len(values) == 0 {
		return Zero()
	}

	sum := Sum(values...)
	return sum.DivFloat(float64(len(values)))
}

// Percentage calculates percentage of Money (e.g., 15% of $100)
func (m Money) Percentage(percent float64) Money {
	return m.MulFloat(percent / 100)
}

// PercentageOf calculates what percentage this Money is of another
func (m Money) PercentageOf(total Money) float64 {
	if total.IsZero() {
		return 0
	}
	return m.Div(total.Decimal).Mul(decimal.NewFromInt(100)).InexactFloat64()
}

// Split splits Money evenly among n parts, with remainder going to first parts
func (m Money) Split(n int) []Money {
	if n <= 0 {
		return []Money{}
	}

	// Calculate base amount for each part
	baseAmount := m.DivFloat(float64(n)).Floor()
	results := make([]Money, n)

	// Distribute base amount
	distributed := Zero()
	for i := 0; i < n; i++ {
		results[i] = baseAmount
		distributed = distributed.Add(baseAmount)
	}

	// Distribute remainder penny by penny
	remainder := m.Sub(distributed)
	pennies := remainder.Mul(decimal.NewFromInt(100)).IntPart()

	for i := 0; i < int(pennies) && i < n; i++ {
		results[i] = results[i].Add(NewMoney(0.01))
	}

	return results
}

// Allocate allocates Money proportionally based on ratios
func (m Money) Allocate(ratios []int) []Money {
	if len(ratios) == 0 {
		return []Money{}
	}

	// Calculate total ratio
	totalRatio := 0
	for _, ratio := range ratios {
		totalRatio += ratio
	}

	if totalRatio == 0 {
		return make([]Money, len(ratios))
	}

	results := make([]Money, len(ratios))
	allocated := Zero()

	// Allocate based on ratios
	for i, ratio := range ratios {
		amount := m.MulFloat(float64(ratio) / float64(totalRatio))
		results[i] = amount.Round(2)
		allocated = allocated.Add(results[i])
	}

	// Handle rounding differences
	diff := m.Sub(allocated)
	if !diff.IsZero() {
		// Add difference to the largest allocation
		maxIdx := 0
		for i := 1; i < len(results); i++ {
			if results[i].GreaterThan(results[maxIdx]) {
				maxIdx = i
			}
		}
		results[maxIdx] = results[maxIdx].Add(diff)
	}

	return results
}
