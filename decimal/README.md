# Money/Decimal Package

A precise monetary calculation package for Go, built on top of [shopspring/decimal](https://github.com/shopspring/decimal) to avoid floating-point precision issues.

## Features

- **Precision**: Exact decimal arithmetic, no floating-point errors
- **Database Support**: Implements `sql.Scanner` and `driver.Valuer` interfaces
- **JSON Support**: Seamless JSON marshaling/unmarshaling
- **Currency Operations**: Common operations like split, allocate, percentage
- **Type Safety**: Strongly typed Money type prevents mixing with regular numbers
- **Rounding**: Multiple rounding methods including banker's rounding

## Installation

```bash
go get github.com/yourusername/go-backend-kit
```

## Usage

### Creating Money Values

```go
import "github.com/yourusername/go-backend-kit/decimal"

// From float
m1 := decimal.NewMoney(99.99)

// From string
m2, err := decimal.NewMoneyFromString("123.45")

// From int
m3 := decimal.NewMoneyFromInt(100)

// Zero value
m4 := decimal.Zero()
```

### Basic Arithmetic

```go
price := decimal.NewMoney(100.00)
tax := decimal.NewMoney(8.25)

// Addition
total := price.Add(tax) // 108.25

// Subtraction
discount := decimal.NewMoney(10.00)
discounted := total.Sub(discount) // 98.25

// Multiplication
quantity := 3
subtotal := price.MulFloat(float64(quantity)) // 300.00

// Division
perPerson := subtotal.DivFloat(4) // 75.00
```

### Comparisons

```go
a := decimal.NewMoney(10.00)
b := decimal.NewMoney(20.00)

if a.LessThan(b) {
    fmt.Println("a is less than b")
}

if a.Equal(decimal.NewMoney(10.00)) {
    fmt.Println("a equals 10.00")
}

// Other comparisons: GreaterThan, LessThanOrEqual, GreaterThanOrEqual, NotEqual
```

### Properties

```go
m := decimal.NewMoney(-10.50)

m.IsNegative() // true
m.IsPositive() // false
m.IsZero()     // false

abs := m.Abs()    // 10.50
neg := m.Neg()    // 10.50 (double negative)
```

### Rounding

```go
m := decimal.NewMoney(10.5678)

// Standard rounding
rounded := m.Round(2) // 10.57

// Banker's rounding (round to even)
m1 := decimal.NewMoney(2.545)
bankerRounded := m1.RoundBank(2) // 2.54

// Floor and Ceil
floor := m.Floor() // 10.00
ceil := m.Ceil()   // 11.00
```

### Percentage Operations

```go
price := decimal.NewMoney(100.00)

// Calculate 15% of price
tax := price.Percentage(15) // 15.00

// Calculate what percentage 25 is of 100
part := decimal.NewMoney(25.00)
percent := part.PercentageOf(price) // 25.0
```

### Splitting Money

```go
// Split $100 among 3 people
total := decimal.NewMoney(100.00)
parts := total.Split(3)
// parts[0] = 33.34 (gets the extra penny)
// parts[1] = 33.33
// parts[2] = 33.33
```

### Allocating Money

```go
// Allocate $1000 based on ratios
budget := decimal.NewMoney(1000.00)
// 50% to development, 30% to marketing, 20% to operations
allocations := budget.Allocate([]int{50, 30, 20})
// allocations[0] = 500.00
// allocations[1] = 300.00
// allocations[2] = 200.00
```

### JSON Serialization

```go
type Invoice struct {
    Total    decimal.Money `json:"total"`
    Subtotal decimal.Money `json:"subtotal"`
    Tax      decimal.Money `json:"tax"`
}

invoice := Invoice{
    Total:    decimal.NewMoney(108.25),
    Subtotal: decimal.NewMoney(100.00),
    Tax:      decimal.NewMoney(8.25),
}

// Marshals to: {"total":"108.25","subtotal":"100.00","tax":"8.25"}
data, _ := json.Marshal(invoice)

// Unmarshaling works with both string and number formats
json.Unmarshal([]byte(`{"total":"108.25"}`), &invoice)
json.Unmarshal([]byte(`{"total":108.25}`), &invoice)
```

### Database Storage

```go
// Works with database/sql
type Product struct {
    ID    int
    Name  string
    Price decimal.Money
}

// In your SQL
// CREATE TABLE products (
//     id INT PRIMARY KEY,
//     name VARCHAR(255),
//     price DECIMAL(19,4)
// );

// Scanning from database
row := db.QueryRow("SELECT price FROM products WHERE id = ?", productID)
var price decimal.Money
err := row.Scan(&price)

// Inserting into database
_, err = db.Exec("INSERT INTO products (name, price) VALUES (?, ?)", 
    "Widget", decimal.NewMoney(19.99))
```

### Helper Functions

```go
// Min and Max
cheapest := decimal.Min(price1, price2)
mostExpensive := decimal.Max(price1, price2)

// Sum multiple values
prices := []decimal.Money{
    decimal.NewMoney(10.00),
    decimal.NewMoney(20.00),
    decimal.NewMoney(30.00),
}
total := decimal.Sum(prices...) // 60.00

// Average
average := decimal.Average(prices...) // 20.00
```

## Best Practices

1. **Always use Money for monetary values**: Never use `float64` for money
2. **String representation**: Use `String()` for display (2 decimal places)
3. **Database storage**: Use `DECIMAL` or `NUMERIC` column types
4. **Rounding**: Be explicit about when and how you round
5. **Division**: Be careful with division - consider using `Split()` or `Allocate()` for distributing money

## Performance Considerations

- Money operations are slower than native float64 due to precision guarantees
- For high-performance scenarios, batch operations when possible
- The underlying decimal library is well-optimized for common cases

## Common Patterns

### Shopping Cart Total
```go
type CartItem struct {
    Price    decimal.Money
    Quantity int
}

func CalculateTotal(items []CartItem, taxRate float64) decimal.Money {
    subtotal := decimal.Zero()
    
    for _, item := range items {
        subtotal = subtotal.Add(item.Price.MulFloat(float64(item.Quantity)))
    }
    
    tax := subtotal.Percentage(taxRate)
    return subtotal.Add(tax)
}
```

### Currency Conversion
```go
func Convert(amount decimal.Money, rate float64) decimal.Money {
    return amount.MulFloat(rate).Round(2)
}
```

### Discount Application
```go
func ApplyDiscount(price decimal.Money, discountPercent float64) decimal.Money {
    discount := price.Percentage(discountPercent)
    return price.Sub(discount)
}
```

## Thread Safety

The Money type is immutable and safe for concurrent use. All operations return new Money values rather than modifying the receiver.