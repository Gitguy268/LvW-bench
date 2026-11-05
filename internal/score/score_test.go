package score

import "testing"

func TestGolden(t *testing.T) {
	fails, n := SelfTest()
	for _, f := range fails {
		t.Error(f)
	}
	t.Logf("%d checks", n)
}
