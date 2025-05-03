package cache

import (
	"fmt"
	"math/bits"
	"strconv"
	"testing"
)

// ejectable returns true if the item i is ejectable.
func (c *Cache[K, V]) ejectable(i int) bool {
	bit := uint64(1) << (i % 64)
	return c.bits[i/64].Load()&bit == bit
}

// setEjectable sets the ejectable bit of item i to v.
func (c *Cache[K, V]) setEjectable(i int, v bool) {
	if v {
		c.bits[i/64].Or(uint64(1) << (i % 64))
	} else {
		c.bits[i/64].And(^(uint64(1) << (i % 64)))
	}
}

// check returns an error if the cache is invalid.
func (c *Cache[K, V]) check() error {
	if c.len != len(c.idx) {
		return fmt.Errorf("expect len %d, got %d", c.len, len(c.idx))
	}
	if len(c.items) != cap(c.items) {
		return fmt.Errorf("items len is %d and capacity %d", len(c.items), cap(c.items))
	}
	if c.len < 0 || c.len > cap(c.items) {
		return fmt.Errorf("invalid len %d for capacity %d", c.len, cap(c.items))
	}
	if len(c.bits) != cap(c.bits) {
		return fmt.Errorf("bits len is %d and capacity %d", len(c.bits), cap(c.bits))
	}
	if c.handIdx < 0 || c.handIdx > len(c.bits) {
		return fmt.Errorf("invalid handIdx %d, for capacity %d", c.handIdx, cap(c.bits))
	}
	if exp, got := ^uint64(0)<<bits.TrailingZeros64(c.handMask), c.handMask; exp != got {
		return fmt.Errorf("expect handMask %016x, got %016x", exp, got)
	}
	for k, v := range c.idx {
		if v < 0 || v >= c.len {
			return fmt.Errorf("invalid idx value %d for key %v", v, k)
		}
		if c.items[v].key != k {
			return fmt.Errorf("expect item key %v, got %v", k, c.items[v].key)
		}
	}
	for i := range c.len {
		p := &c.items[i]
		v, ok := c.idx[p.key]
		if !ok {
			return fmt.Errorf("item key %v not found in index", p.key)
		}
		if v != i {
			return fmt.Errorf("item key %v has invalid index %v in idx", p.key, v)
		}
		if exp, got := ^(uint64(1) << (i % 64)), p.bit; exp != got {
			return fmt.Errorf("expect bit %016x, got %016x", exp, got)
		}
	}
	return nil
}

func TestCacheDelete(t *testing.T) {
	const size = 256
	c := New[int, int](size)
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	c.Add(1, 1)
	c.Add(2, 2)
	c.Add(3, 3)
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	if !c.Has(1) {
		t.Fatal("expect to contain 1")
	}

	if c.Has(5) {
		t.Fatal("expect to no contain 5")
	}

	// items: 1, 2, 3
	c.setEjectable(c.idx[3], true)
	if v, ok := c.Delete(1); !ok || v != 1 {
		t.Fatal("expect delete 1 to succeed")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	// items: 3, 2
	c.setEjectable(c.idx[2], false)
	if v, ok := c.Delete(3); !ok || v != 3 {
		t.Fatal("expect delete 3 to succeed")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	// items: 2
	if v, ok := c.Delete(2); !ok || v != 2 {
		t.Fatal("expect delete 2 to succeed")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Fatal("expect to be empty")
	}
}

func TestCacheIter(t *testing.T) {
	const size = 256
	c := New[int, int](size)
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	// fill cache
	for i := range c.Cap() {
		if v, ok := c.Add(i, i); ok || v != 0 {
			t.Fatalf("for %d expect no ejection", i)
		}
		if err := c.check(); err != nil {
			t.Fatal(err)
		}
	}

	m := make([]int, c.Cap())
	for k, v := range c.Items() {
		if k != v {
			t.Fatalf("expect key %d to be equal to value %v", k, v)
		}
		m[k] = v + 1
	}
	for i := range m {
		if m[i] != i+1 {
			t.Fatalf("for %d expect value %d, gor %d", i, i+1, m[i])
		}
	}

	for k := range c.Items() {
		if k == 3 {
			break
		}
	}

}

func TestCacheAdd(t *testing.T) {
	const size = 256
	c := New[int, int](size)
	if err := c.check(); err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Fatal("expect empty")
	}

	// fill cache
	for i := range c.Cap() {
		if v, ok := c.Add(i, i); ok || v != 0 {
			t.Fatalf("for %d expect no ejection", i)
		}
		if err := c.check(); err != nil {
			t.Fatal(err)
		}
	}

	if c.Len() != c.Cap() {
		t.Fatal("expect full")
	}

	for i := range c.items {
		if c.ejectable(i) {
			t.Fatalf("expect item %d to be not ejectable", i)
		}
	}

	// should eject item 253 and replace with 256
	c.setEjectable(253, true)
	if v, ok := c.Add(256, 256); !ok || v != 253 {
		t.Fatalf("expect to eject 253")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(253); ok {
		t.Fatal("expect item 253 to be replaced")
	}
	if v, ok := c.Get(256); !ok || v != 256 {
		t.Fatal("unexpected failure to get item 256")
	}

	if v, ok := c.Delete(256); !ok || v != 256 {
		t.Fatal("expect delete item 256 to succeed")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	if v, ok := c.Add(253, 253); ok || v != 0 {
		t.Fatalf("expect no ejection")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	l := c.Len()
	if v, ok := c.Add(253, 257); !ok || v != 253 {
		t.Fatalf("expect value 253 ejected")
	}
	if c.Len() != l {
		t.Fatalf("expect size is unmodified")
	}
	if v, ok := c.Get(253); !ok || v != 257 {
		t.Fatalf("expect 257, got %v", v)
	}

	// test hand wrapping

	c.Reset()
	if c.Len() != 0 {
		t.Fatal("expect empty")
	}

	// fill cache
	for i := range c.Cap() {
		c.Add(i, i)
		if err := c.check(); err != nil {
			t.Fatal(err)
		}
	}

	c.setEjectable(255, true)
	if v, ok := c.Add(256, 256); !ok || v != 255 {
		t.Fatalf("expect to eject 255")
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	for i := range c.bits {
		c.bits[i].Store(0)
	}
	c.handIdx = len(c.bits) - 1

	c.setEjectable(10, true)
	if v, ok := c.Add(257, 257); !ok || v != 10 {
		t.Fatalf("expect to eject 10, got %v", v)
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}

	for i := range c.bits {
		c.bits[i].Store(0)
	}
	c.handIdx = 1
	c.setEjectable(11, true)
	if v, ok := c.Add(258, 258); !ok || v != 11 {
		t.Fatalf("expect to eject 10, got %v", v)
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkInt(b *testing.B) {

	const size = 10240

	// c1 is referenced as "_int"
	var c1 Cache[int, int]
	c1.Init(size)

	// c2 is referenced as "*int"
	c2 := New[int, int](size)

	// c3 is reference as ".int"
	c3 := &struct {
		c Cache[int, int]
	}{}
	c3.c.Init(size)

	// c4 is reference as "^int"
	c4 := &struct {
		c *Cache[int, int]
	}{}
	c4.c = New[int, int](size)

	sizeStr := strconv.Itoa(size)

	b.Run("Set  int      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Add(i, i)
		}
	})

	b.Run("Set *int      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Add(i, i)
		}
	})

	b.Run("Set .int      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Add(i, i)
		}
	})

	b.Run("Set ^int      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Add(i, i)
		}
	})

	key := c1.items[0].key
	b.Run("Get  int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Get(key)
		}
	})

	key = c2.items[0].key
	b.Run("Get *int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Get(key)
		}
	})

	key = c3.c.items[0].key
	b.Run("Get .int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Get(key)
		}
	})

	key = c4.c.items[0].key
	b.Run("Get ^int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Get(key)
		}
	})

	key = -1
	b.Run("Get  int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Get(key)
		}
	})

	b.Run("Get *int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Get(key)
		}
	})

	b.Run("Get .int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Get(key)
		}
	})

	b.Run("Get ^int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Get(key)
		}
	})

	key = c1.items[0].key
	b.Run("Has  int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Has(key)
		}
	})

	key = c2.items[0].key
	b.Run("Has *int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Has(key)
		}
	})

	key = c3.c.items[0].key
	b.Run("Has .int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Has(key)
		}
	})

	key = c4.c.items[0].key
	b.Run("Has ^int hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Has(key)
		}
	})

	key = -1
	b.Run("Has  int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Has(key)
		}
	})

	b.Run("Has *int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Has(key)
		}
	})

	b.Run("Has .int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Has(key)
		}
	})

	b.Run("Has ^int miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Has(key)
		}
	})
}

var buf = append(make([]byte, 0, 200), "string "...)

func testStr(i int) string {
	buf = strconv.AppendInt(buf[:7], int64(i), 10)
	return string(buf)
}

func BenchmarkStr(b *testing.B) {

	const size = 10240

	// c1 is referenced as "_str"
	var c1 Cache[string, int]
	c1.Init(size)

	// c2 is referenced as "*str"
	c2 := New[string, int](size)

	// c3 is reference as ".str"
	c3 := &struct {
		c Cache[string, int]
	}{}
	c3.c.Init(size)

	// c4 is reference as "^str"
	c4 := &struct {
		c *Cache[string, int]
	}{}
	c4.c = New[string, int](size)

	sizeStr := strconv.Itoa(size)

	b.Run("Set  str      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Add(testStr(i), i)
		}
	})

	b.Run("Set *str      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Add(testStr(i), i)
		}
	})

	b.Run("Set .str      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Add(testStr(i), i)
		}
	})

	b.Run("Set ^str      "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Add(testStr(i), i)
		}
	})

	key := c1.items[0].key
	b.Run("Get  str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Get(key)
		}
	})

	key = c2.items[0].key
	b.Run("Get *str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Get(key)
		}
	})

	key = c3.c.items[0].key
	b.Run("Get .str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Get(key)
		}
	})

	key = c4.c.items[0].key
	b.Run("Get ^str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Get(key)
		}
	})

	key = fmt.Sprintf("string %d", -1)
	b.Run("Get  str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Get(key)
		}
	})

	b.Run("Get *str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Get(key)
		}
	})

	b.Run("Get .str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Get(key)
		}
	})

	b.Run("Get ^str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Get(key)
		}
	})

	key = c1.items[0].key
	b.Run("Has  str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Has(key)
		}
	})

	key = c2.items[0].key
	b.Run("Has *str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Has(key)
		}
	})

	key = c3.c.items[0].key
	b.Run("Has .str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Has(key)
		}
	})

	key = c4.c.items[0].key
	b.Run("Has ^str hit  "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Has(key)
		}
	})

	key = fmt.Sprintf("string %d", -1)
	b.Run("Has  str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Has(key)
		}
	})

	b.Run("Has *str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Has(key)
		}
	})

	b.Run("Has .str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c3.c.Has(key)
		}
	})

	b.Run("Has ^str miss "+sizeStr, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c4.c.Has(key)
		}
	})
}

func BenchmarkFreeAdd(b *testing.B) {
	c1 := New[int, int](10240)
	var k int
	b.ResetTimer()
	b.Run(fmt.Sprintf("Add fast int %d", c1.Cap()), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c1.Add(k, k)
			k++
			if c1.Len() == c1.Cap() {
				c1.len = 0
				clear(c1.idx)
			}
		}
	})

	c2 := New[string, int](10240)
	b.ResetTimer()
	b.Run(fmt.Sprintf("Add fast str %d", c2.Cap()), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c2.Add(testStr(k), k)
			k++
			if c2.Len() == c2.Cap() {
				c2.len = 0
				clear(c2.idx)
			}
		}
	})

}
