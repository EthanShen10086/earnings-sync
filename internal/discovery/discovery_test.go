package discovery_test

import (
	"context"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/discovery"
)

func TestMemoryRegisterDiscover(t *testing.T) {
	ctx := context.Background()
	r := discovery.NewMemory()
	if err := r.Register(ctx, discovery.Instance{
		Service: "earnings-api",
		Host:    "127.0.0.1",
		Port:    8090,
		Meta:    map[string]string{"env": "test"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Discover(ctx, "earnings-api")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Port != 8090 || !got[0].Healthy {
		t.Fatalf("got=%+v", got)
	}
	svcs, err := r.ListServices(ctx)
	if err != nil || len(svcs) != 1 || svcs[0] != "earnings-api" {
		t.Fatalf("services=%v err=%v", svcs, err)
	}
	if err := r.Deregister(ctx, "earnings-api", got[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Discover(ctx, "earnings-api")
	if len(got) != 0 {
		t.Fatalf("expected empty after deregister: %+v", got)
	}
}

func TestOpenNacosWithoutURLUsesMemory(t *testing.T) {
	ctx := context.Background()
	reg, st, err := discovery.Open(ctx, "nacos", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	if !st.Degraded || st.Backend != "nacos-memory" {
		t.Fatalf("status=%+v", st)
	}
	if err := reg.Register(ctx, discovery.Instance{Service: "svc", Host: "h", Port: 1}); err != nil {
		t.Fatal(err)
	}
	list, err := reg.Discover(ctx, "svc")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
}

func TestOpenConsulUnreachableDegrades(t *testing.T) {
	ctx := context.Background()
	reg, st, err := discovery.Open(ctx, "consul", "127.0.0.1:1", false)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	if !st.Degraded {
		t.Fatalf("expected degraded: %+v", st)
	}
	_ = reg.Register(ctx, discovery.Instance{Service: "a", ID: "1", Host: "x", Port: 9})
	list, _ := reg.Discover(ctx, "a")
	if len(list) != 1 {
		t.Fatalf("memory path broken: %v", list)
	}
}

func TestOpenEurekaStrictFailsUnreachable(t *testing.T) {
	ctx := context.Background()
	_, _, err := discovery.Open(ctx, "eureka", "127.0.0.1:1", true)
	if err == nil {
		t.Fatal("expected strict error")
	}
}

func TestNormalizeBackend(t *testing.T) {
	if discovery.NormalizeBackend("Nacos") != discovery.BackendNacos {
		t.Fatal("nacos")
	}
	if discovery.NormalizeBackend("") != discovery.BackendMemory {
		t.Fatal("memory default")
	}
}
