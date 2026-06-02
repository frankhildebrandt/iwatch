package buffer

import (
	"fmt"
	"testing"

	"github.com/stackriot/iwatch/internal/config"
)

func fillBuffer(b *testing.B, buf *LogBuffer, count int) {
	b.Helper()
	for i := 0; i < count; i++ {
		buf.Append("stdout", fmt.Sprintf(`level=INFO component=api msg="line-%d"`, i))
	}
}

func BenchmarkSnapshotWarmCache(b *testing.B) {
	buf, err := New(100_000, nil)
	if err != nil {
		b.Fatal(err)
	}
	fillBuffer(b, buf, 100_000)
	opts := SnapshotOptions{Query: "level=info"}
	_ = buf.Snapshot(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.Snapshot(opts)
	}
}

func BenchmarkAppendLineWithWarmSnapshotCache(b *testing.B) {
	buf, err := New(100_000, nil)
	if err != nil {
		b.Fatal(err)
	}
	fillBuffer(b, buf, 100_000)
	opts := SnapshotOptions{Query: "level=info"}
	_ = buf.Snapshot(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Append("stdout", fmt.Sprintf(`level=INFO component=api msg="bench-%d"`, i))
	}
}

func BenchmarkDistinctFieldValuesIndexed(b *testing.B) {
	buf, err := New(100_000, nil)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 100_000; i++ {
		component := "api"
		if i%2 == 0 {
			component = "worker"
		}
		buf.Append("stdout", fmt.Sprintf(`level=INFO component=%s msg="line-%d"`, component, i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.DistinctFieldValues("component")
	}
}

func BenchmarkFullSnapshotColdCache(b *testing.B) {
	opts := SnapshotOptions{
		Query: "level=info",
		Preset: config.FilterPreset{
			Clauses: []config.FilterClause{
				{Conditions: []config.FilterCondition{{Field: "level", Value: "info"}}},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		buf, err := New(10_000, nil)
		if err != nil {
			b.Fatal(err)
		}
		fillBuffer(b, buf, 10_000)
		b.StartTimer()
		_ = buf.Snapshot(opts)
	}
}
