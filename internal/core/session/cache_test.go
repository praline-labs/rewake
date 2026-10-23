package session

import "testing"

func TestNewCacheIsTheShareOfAllInputTheCacheServed(t *testing.T) {
	cases := []struct {
		read, write, input int64
		want               int
	}{
		{9000, 500, 500, 90},
		{0, 10000, 3, 0},
		{200, 0, 0, 100},
		{2, 1, 0, 67},
	}
	for _, one := range cases {
		got := NewCache(one.read, one.write, one.input)
		if got == nil || got.HitPercent != one.want {
			t.Fatalf("read %d write %d input %d: %+v, want %d%%", one.read, one.write, one.input, got, one.want)
		}
	}
	if NewCache(0, 0, 0) != nil || NewCache(-1, 5, 5) != nil {
		t.Fatal("a request with no input or a negative count has no share")
	}
}
