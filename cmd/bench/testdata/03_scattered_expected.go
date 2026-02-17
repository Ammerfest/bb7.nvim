package processing

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
)

const (
	MaxRetries    = 5
	BatchSize     = 100
	DefaultWeight = 1.5
	Separator     = ","
)

// Record holds a single data record with metadata.
type Record struct {
	ID       int
	Name     string
	Value    float64
	Tags     []string
	Priority int
}

// Dataset is a collection of records with summary statistics.
type Dataset struct {
	Records  []*Record
	Sorted   bool
	Source   string
	MinValue float64
	MaxValue float64
}

// NewDataset creates a dataset from raw records.
func NewDataset(records []*Record, source string) *Dataset {
	ds := &Dataset{
		Records: records,
		Source:  source,
	}
	if len(records) > 0 {
		ds.MinValue = records[0].Value
		ds.MaxValue = records[0].Value
		for _, r := range records[1:] {
			if r.Value < ds.MinValue {
				ds.MinValue = r.Value
			}
			if r.Value > ds.MaxValue {
				ds.MaxValue = r.Value
			}
		}
	}
	return ds
}

// FilterByTag returns records that contain the specified tag.
func (ds *Dataset) FilterByTag(tag string) []*Record {
	var matched []*Record
	for _, r := range ds.Records {
		for _, t := range r.Tags {
			if t == tag {
				matched = append(matched, r)
				break
			}
		}
	}
	log.Printf("filtered %d records by tag %q, found %d matches", len(ds.Records), tag, len(matched))
	return matched
}

// SortByValue sorts the dataset records by value in ascending order.
func (ds *Dataset) SortByValue() {
	sort.Slice(ds.Records, func(i, j int) bool {
		return ds.Records[i].Value < ds.Records[j].Value
	})
	ds.Sorted = true
}

// ComputeStats calculates mean and standard deviation of record values.
func (ds *Dataset) ComputeStats() (mean, stddev float64) {
	if len(ds.Records) == 0 {
		return 0, 0
	}

	sum := 0.0
	for _, r := range ds.Records {
		sum += r.Value
	}
	mean = sum / float64(len(ds.Records))

	variance := 0.0
	for _, r := range ds.Records {
		diff := r.Value - mean
		variance += diff * diff
	}
	variance /= float64(len(ds.Records))
	stddev = math.Sqrt(variance)

	return mean, stddev
}

// NormalizeValues scales all record values to the range [0, 1].
func (ds *Dataset) NormalizeValues() {
	if len(ds.Records) == 0 {
		return
	}

	rangeVal := ds.MaxValue - ds.MinValue
	if rangeVal == 0 {
		for _, r := range ds.Records {
			r.Value = 0.5
		}
		return
	}

	for _, r := range ds.Records {
		r.Value = (r.Value - ds.MinValue) / rangeVal
	}

	ds.MinValue = 0
	ds.MaxValue = 1
}

// FormatRecord produces a human-readable string for a single record.
func FormatRecord(r *Record) string {
	if r == nil {
		return "<nil record>"
	}
	tagStr := strings.Join(r.Tags, Separator)
	return fmt.Sprintf("[%d] %s: %.2f (tags: %s, priority: %d)",
		r.ID, r.Name, r.Value, tagStr, r.Priority)
}

// TopN returns the top N records by value (highest first).
func (ds *Dataset) TopN(n int) []*Record {
	if n <= 0 {
		return nil
	}
	if n > len(ds.Records) {
		n = len(ds.Records)
	}

	sorted := make([]*Record, len(ds.Records))
	copy(sorted, ds.Records)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	return sorted[:n]
}

// MergeDatasets combines two datasets into one.
func MergeDatasets(a, b *Dataset) *Dataset {
	records := make([]*Record, 0, len(a.Records)+len(b.Records))
	records = append(records, a.Records...)
	records = append(records, b.Records...)

	merged := NewDataset(records, a.Source+"+"+b.Source)
	log.Printf("merged %d + %d = %d records from %q and %q", len(a.Records), len(b.Records), len(records), a.Source, b.Source)
	return merged
}

// ApplyWeights multiplies each record's value by the default weight.
func (ds *Dataset) ApplyWeights() {
	for _, r := range ds.Records {
		r.Value *= DefaultWeight
	}

	// Recalculate bounds
	if len(ds.Records) > 0 {
		ds.MinValue = ds.Records[0].Value
		ds.MaxValue = ds.Records[0].Value
		for _, r := range ds.Records[1:] {
			if r.Value < ds.MinValue {
				ds.MinValue = r.Value
			}
			if r.Value > ds.MaxValue {
				ds.MaxValue = r.Value
			}
		}
	}
}

// GroupByPriority returns records grouped by their priority level.
func (ds *Dataset) GroupByPriority() map[int][]*Record {
	groups := make(map[int][]*Record)
	for _, r := range ds.Records {
		groups[r.Priority] = append(groups[r.Priority], r)
	}
	return groups
}

// Summary produces a text summary of the dataset.
func (ds *Dataset) Summary() string {
	mean, stddev := ds.ComputeStats()
	return fmt.Sprintf("Dataset %q: %d records, mean=%.2f, stddev=%.2f, range=[%.2f, %.2f]",
		ds.Source, len(ds.Records), mean, stddev, ds.MinValue, ds.MaxValue)
}

// RemoveDuplicates removes records with duplicate IDs, keeping the first occurrence.
func (ds *Dataset) RemoveDuplicates() int {
	seen := make(map[int]bool)
	unique := make([]*Record, 0, len(ds.Records))

	for _, r := range ds.Records {
		if !seen[r.ID] {
			seen[r.ID] = true
			unique = append(unique, r)
		}
	}

	removed := len(ds.Records) - len(unique)
	ds.Records = unique
	return removed
}
