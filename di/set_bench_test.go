package di

import (
	"testing"
)

// BenchmarkNewSet measures ProviderSet creation overhead.
func BenchmarkNewSet(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewSet("dev",
			func() (*mockRepo, error) { return &mockRepo{}, nil },
			func(c *Container) (*mockService, error) {
				repo, _ := Invoke[*mockRepo](c)
				return &mockService{repo: repo}, nil
			},
		)
	}
}

// BenchmarkRegisterSets measures registration of multiple sets.
func BenchmarkRegisterSets(b *testing.B) {
	devSet := NewSet("dev",
		func() (*mockRepo, error) { return &mockRepo{}, nil },
		func(c *Container) (*mockService, error) {
			repo, _ := Invoke[*mockRepo](c)
			return &mockService{repo: repo}, nil
		},
	)
	prodSet := NewSet("prod",
		func() (*mockRepo, error) { return &mockRepo{}, nil },
		func(c *Container) (*mockService, error) {
			repo, _ := Invoke[*mockRepo](c)
			return &mockService{repo: repo}, nil
		},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New()
		MustRegisterSets(c, "dev", devSet, prodSet)
	}
}

// BenchmarkInvoke measures dependency resolution cost.
func BenchmarkInvoke(b *testing.B) {
	c := New()
	devSet := NewSet("dev",
		func() (*mockRepo, error) { return &mockRepo{}, nil },
		func(c *Container) (*mockService, error) {
			repo, _ := Invoke[*mockRepo](c)
			return &mockService{repo: repo}, nil
		},
	)
	MustRegisterSets(c, "dev", devSet)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Invoke[*mockService](c)
	}
}

// BenchmarkInvokeParallel measures concurrent resolution.
func BenchmarkInvokeParallel(b *testing.B) {
	c := New()
	devSet := NewSet("dev",
		func() (*mockRepo, error) { return &mockRepo{}, nil },
		func(c *Container) (*mockService, error) {
			repo, _ := Invoke[*mockRepo](c)
			return &mockService{repo: repo}, nil
		},
	)
	MustRegisterSets(c, "dev", devSet)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = Invoke[*mockService](c)
		}
	})
}

// Mock types for benchmarking
type mockRepo struct{}

type mockService struct {
	repo *mockRepo
}
