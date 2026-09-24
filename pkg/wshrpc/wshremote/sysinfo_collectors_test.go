package wshremote

import "testing"

func TestCpuCollectorProbeAndDescribe(t *testing.T) {
	c := MakeCpuCollector()
	if !c.Probe() {
		t.Fatal("cpu collector must always probe true")
	}
	meta := c.Describe()
	if _, ok := meta["cpu"]; !ok {
		t.Fatal("expected 'cpu' key in cpu collector metadata")
	}
	if meta["cpu"].Label == "" || meta["cpu"].Unit != "%" {
		t.Fatalf("unexpected cpu meta: %+v", meta["cpu"])
	}
}

func TestCpuCollectorCollectReturnsAggregateAndPerCore(t *testing.T) {
	c := MakeCpuCollector()
	c.Probe()
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := values["cpu"]; !ok {
		t.Fatal("expected aggregate 'cpu' value")
	}
	found := false
	for k := range values {
		if k != "cpu" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one per-core cpu:N value on a real host")
	}
}

func TestMemCollectorProbeAndCollect(t *testing.T) {
	c := MakeMemCollector()
	if !c.Probe() {
		t.Fatal("mem collector must always probe true")
	}
	values, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"mem:total", "mem:used", "mem:free", "mem:available"} {
		if _, ok := values[key]; !ok {
			t.Fatalf("expected %q in mem collector output", key)
		}
	}
	meta := c.Describe()
	if meta["mem:used"].MaxYKey != "mem:total" {
		t.Fatalf("expected mem:used to reference mem:total as its dynamic max, got %+v", meta["mem:used"])
	}
}
