package provider

import (
	"reflect"
	"testing"
)

func TestRegistryGetAndOrder(t *testing.T) {
	registry := NewRegistry(
		&fakeProvider{IDValue: "zeta"},
		&fakeProvider{IDValue: Tencent},
		&fakeProvider{IDValue: "alpha"},
		&fakeProvider{IDValue: GCP},
		&fakeProvider{IDValue: Azure},
		&fakeProvider{IDValue: AWS},
	)

	var got []ID
	for _, p := range registry.All() {
		got = append(got, p.ID())
	}
	want := []ID{GCP, AWS, Azure, Tencent, "alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("All() order = %v, want %v", got, want)
	}

	if p, ok := registry.Get(AWS); !ok || p.ID() != AWS {
		t.Fatalf("Get(aws) = %v, %v", p, ok)
	}
	if _, ok := registry.Get("nope"); ok {
		t.Fatal("Get(nope) ok = true, want false")
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewRegistry did not panic on duplicate ID")
		}
	}()
	NewRegistry(&fakeProvider{IDValue: GCP}, &fakeProvider{IDValue: GCP})
}
