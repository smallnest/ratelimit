// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3 with static-linking exception.
// See LICENCE file for details.

package ratelimit

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type rateLimitSuite struct{}

var _ = gc.Suite(rateLimitSuite{})

type takeReq struct {
	time       time.Duration
	count      int64
	expectWait time.Duration
}

var takeTests = []struct {
	about        string
	fillInterval time.Duration
	capacity     int64
	reqs         []takeReq
}{{
	about:        "serial requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      0,
		expectWait: 0,
	}, {
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       0,
		count:      1,
		expectWait: 250 * time.Millisecond,
	}, {
		time:       250 * time.Millisecond,
		count:      1,
		expectWait: 250 * time.Millisecond,
	}},
}, {
	about:        "concurrent requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       0,
		count:      2,
		expectWait: 500 * time.Millisecond,
	}, {
		time:       0,
		count:      2,
		expectWait: 1000 * time.Millisecond,
	}, {
		time:       0,
		count:      1,
		expectWait: 1250 * time.Millisecond,
	}},
}, {
	about:        "more than capacity",
	fillInterval: 1 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       20 * time.Millisecond,
		count:      15,
		expectWait: 5 * time.Millisecond,
	}},
}, {
	about:        "sub-quantum time",
	fillInterval: 10 * time.Millisecond,
	capacity:     10,
	reqs: []takeReq{{
		time:       0,
		count:      10,
		expectWait: 0,
	}, {
		time:       7 * time.Millisecond,
		count:      1,
		expectWait: 3 * time.Millisecond,
	}, {
		time:       8 * time.Millisecond,
		count:      1,
		expectWait: 12 * time.Millisecond,
	}},
}, {
	about:        "within capacity",
	fillInterval: 10 * time.Millisecond,
	capacity:     5,
	reqs: []takeReq{{
		time:       0,
		count:      5,
		expectWait: 0,
	}, {
		time:       60 * time.Millisecond,
		count:      5,
		expectWait: 0,
	}, {
		time:       60 * time.Millisecond,
		count:      1,
		expectWait: 10 * time.Millisecond,
	}, {
		time:       80 * time.Millisecond,
		count:      2,
		expectWait: 10 * time.Millisecond,
	}},
}}

var availTests = []struct {
	about        string
	capacity     int64
	fillInterval time.Duration
	take         int64
	sleep        time.Duration

	expectCountAfterTake  int64
	expectCountAfterSleep int64
}{{
	about:                 "should fill tokens after interval",
	capacity:              5,
	fillInterval:          time.Second,
	take:                  5,
	sleep:                 time.Second,
	expectCountAfterTake:  0,
	expectCountAfterSleep: 1,
}, {
	about:                 "should fill tokens plus existing count",
	capacity:              2,
	fillInterval:          time.Second,
	take:                  1,
	sleep:                 time.Second,
	expectCountAfterTake:  1,
	expectCountAfterSleep: 2,
}, {
	about:                 "shouldn't fill before interval",
	capacity:              2,
	fillInterval:          2 * time.Second,
	take:                  1,
	sleep:                 time.Second,
	expectCountAfterTake:  1,
	expectCountAfterSleep: 1,
}, {
	about:                 "should fill only once after 1*interval before 2*interval",
	capacity:              2,
	fillInterval:          2 * time.Second,
	take:                  1,
	sleep:                 3 * time.Second,
	expectCountAfterTake:  1,
	expectCountAfterSleep: 2,
}}

func (rateLimitSuite) TestTake(c *gc.C) {
	for i, test := range takeTests {
		tb := NewBucket(test.fillInterval, test.capacity, 1, test.capacity)
		for j, req := range test.reqs {
			d, ok := tb.take(tb.startTime.Add(req.time), req.count, infinityDuration)
			c.Assert(ok, gc.Equals, true)
			if d != req.expectWait {
				c.Fatalf("test %d.%d, %s, got %v want %v", i, j, test.about, d, req.expectWait)
			}
		}
	}
}

func (rateLimitSuite) TestTakeMaxDuration(c *gc.C) {
	for i, test := range takeTests {
		tb := NewBucket(test.fillInterval, test.capacity, 1, test.capacity)
		for j, req := range test.reqs {
			if req.expectWait > 0 {
				d, ok := tb.take(tb.startTime.Add(req.time), req.count, req.expectWait-1)
				c.Assert(ok, gc.Equals, false)
				c.Assert(d, gc.Equals, time.Duration(0))
			}
			d, ok := tb.take(tb.startTime.Add(req.time), req.count, req.expectWait)
			c.Assert(ok, gc.Equals, true)
			if d != req.expectWait {
				c.Fatalf("test %d.%d, %s, got %v want %v", i, j, test.about, d, req.expectWait)
			}
		}
	}
}

type takeAvailableReq struct {
	time   time.Duration
	count  int64
	expect int64
}

var takeAvailableTests = []struct {
	about        string
	fillInterval time.Duration
	capacity     int64
	reqs         []takeAvailableReq
}{{
	about:        "serial requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  0,
		expect: 0,
	}, {
		time:   0,
		count:  10,
		expect: 10,
	}, {
		time:   0,
		count:  1,
		expect: 0,
	}, {
		time:   250 * time.Millisecond,
		count:  1,
		expect: 1,
	}},
}, {
	about:        "concurrent requests",
	fillInterval: 250 * time.Millisecond,
	capacity:     10,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  5,
		expect: 5,
	}, {
		time:   0,
		count:  2,
		expect: 2,
	}, {
		time:   0,
		count:  5,
		expect: 3,
	}, {
		time:   0,
		count:  1,
		expect: 0,
	}},
}, {
	about:        "more than capacity",
	fillInterval: 1 * time.Millisecond,
	capacity:     10,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  10,
		expect: 10,
	}, {
		time:   20 * time.Millisecond,
		count:  15,
		expect: 10,
	}},
}, {
	about:        "within capacity",
	fillInterval: 10 * time.Millisecond,
	capacity:     5,
	reqs: []takeAvailableReq{{
		time:   0,
		count:  5,
		expect: 5,
	}, {
		time:   60 * time.Millisecond,
		count:  5,
		expect: 5,
	}, {
		time:   70 * time.Millisecond,
		count:  1,
		expect: 1,
	}},
}}

func (rateLimitSuite) TestTakeAvailable(c *gc.C) {
	for i, test := range takeAvailableTests {
		tb := NewBucket(test.fillInterval, test.capacity, 1, test.capacity)
		for j, req := range test.reqs {
			d := tb.takeAvailable(tb.startTime.Add(req.time), req.count)
			if d != req.expect {
				c.Fatalf("test %d.%d, %s, got %v want %v", i, j, test.about, d, req.expect)
			}
		}
	}
}

func (rateLimitSuite) TestPanics(c *gc.C) {
	c.Assert(func() { NewBucket(0, 1, 1, 1) }, gc.PanicMatches, "token bucket fill interval is not > 0")
	c.Assert(func() { NewBucket(-2, 1, 1, 1) }, gc.PanicMatches, "token bucket fill interval is not > 0")
	c.Assert(func() { NewBucket(1, 0, 1, 0) }, gc.PanicMatches, "token bucket capacity is not > 0")
	c.Assert(func() { NewBucket(1, -2, 1, -2) }, gc.PanicMatches, "token bucket capacity is not > 0")
}

func isCloseTo(x, y, tolerance float64) bool {
	return math.Abs(x-y)/y < tolerance
}

func (rateLimitSuite) TestRate(c *gc.C) {
	tb := NewBucket(1, 1, 1, 1)
	if !isCloseTo(tb.Rate(), 1e9, 0.00001) {
		c.Fatalf("got %v want 1e9", tb.Rate())
	}
	tb = NewBucket(2*time.Second, 1, 1, 1)
	if !isCloseTo(tb.Rate(), 0.5, 0.00001) {
		c.Fatalf("got %v want 0.5", tb.Rate())
	}
	tb = NewBucket(100*time.Millisecond, 1, 5, 1)
	if !isCloseTo(tb.Rate(), 50, 0.00001) {
		c.Fatalf("got %v want 50", tb.Rate())
	}
}

func TestAvailable(t *testing.T) {
	for i, tt := range availTests {
		tb := NewBucket(tt.fillInterval, tt.capacity, 1, tt.capacity)
		if c := tb.takeAvailable(tb.startTime, tt.take); c != tt.take {
			t.Fatalf("#%d: %s, take = %d, want = %d", i, tt.about, c, tt.take)
		}
		if c := tb.available(tb.startTime); c != tt.expectCountAfterTake {
			t.Fatalf("#%d: %s, after take, available = %d, want = %d", i, tt.about, c, tt.expectCountAfterTake)
		}
		if c := tb.available(tb.startTime.Add(tt.sleep)); c != tt.expectCountAfterSleep {
			t.Fatalf("#%d: %s, after some time it should fill in new tokens, available = %d, want = %d",
				i, tt.about, c, tt.expectCountAfterSleep)
		}
	}

}

func TestNoBonusTokenAfterBucketIsFull(t *testing.T) {
	tb := NewBucket(time.Second*1, 100, 20, 100)
	curAvail := tb.Available()
	if curAvail != 100 {
		t.Fatalf("initially: actual available = %d, expected = %d", curAvail, 100)
	}

	time.Sleep(time.Second * 5)

	curAvail = tb.Available()
	if curAvail != 100 {
		t.Fatalf("after pause: actual available = %d, expected = %d", curAvail, 100)
	}

	cnt := tb.TakeAvailable(100)
	if cnt != 100 {
		t.Fatalf("taking: actual taken count = %d, expected = %d", cnt, 100)
	}

	curAvail = tb.Available()
	if curAvail != 0 {
		t.Fatalf("after taken: actual available = %d, expected = %d", curAvail, 0)
	}
}

func BenchmarkWait(b *testing.B) {
	tb := NewBucket(1, 16*1024, 1, 16*1024)
	for i := b.N - 1; i >= 0; i-- {
		tb.Wait(1)
	}
}

func TestNew(t *testing.T) {
	bucket := New(10000)

	assert.Equal(t, bucket.Capacity(), int64(10000))
	assert.Equal(t, bucket.Rate(), float64(10000))
	assert.Equal(t, bucket.fillInterval, time.Millisecond)
	assert.Equal(t, bucket.batch, int64(10))
	assert.Equal(t, bucket.Available(), int64(10000))

	bucket = New(10061)
	assert.Equal(t, bucket.Capacity(), int64(10061))
	assert.Equal(t, bucket.Rate(), float64(10000))
	assert.Equal(t, bucket.fillInterval, time.Millisecond)
	assert.Equal(t, bucket.batch, int64(10))
	assert.Equal(t, bucket.Available(), int64(10061))
}
