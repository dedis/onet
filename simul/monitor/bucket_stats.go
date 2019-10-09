package monitor

import (
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// bucketRule represents a filter that will tell if a measure must
// be added to a bucket. It follows the logic of a slice indexing
// so that the lower index is inclusive and the upper one is exclusive.
type bucketRule struct {
	low  int
	high int
}

// newBucketRule takes a string and parses it to instantiate
// a bucket rule.
func newBucketRule(r string) (rule bucketRule, err error) {
	parts := strings.Split(r, ":")
	if len(parts) != 2 {
		err = xerrors.New("malformed rule")
		return
	}

	rule.low, err = strconv.Atoi(parts[0])
	if err != nil {
		err = xerrors.Errorf("atoi: %v", err)
		return
	}

	rule.high, err = strconv.Atoi(parts[1])
	if err != nil {
		err = xerrors.Errorf("atoi: %v", err)
		return
	}

	return
}

// Match returns true when the index is accepted by the rule, false
// otherwise.
func (r bucketRule) Match(index int) bool {
	return index >= r.low && index < r.high
}

type bucketRules []bucketRule

// Match returns true when at least one of the rule accepts the host
// index.
func (rr bucketRules) Match(host int) bool {
	if host < 0 {
		// a value below is considered as an invalid host index
		return false
	}

	for _, rule := range rr {
		if rule.Match(host) {
			return true
		}
	}

	return false
}

// BucketStats splits the statistics into buckets according to network addresses
// and associated rules
type BucketStats struct {
	rules   map[int]bucketRules
	buckets map[int]*Stats
}

func newBucketStats() *BucketStats {
	return &BucketStats{
		rules:   make(map[int]bucketRules),
		buckets: make(map[int]*Stats),
	}
}

// Set creates a new bucket at the given index that uses the rules to filter
// incoming measures
func (bs *BucketStats) Set(index int, rules []string, stats *Stats) error {
	bs.rules[index] = make(bucketRules, len(rules))
	for i, str := range rules {
		rule, err := newBucketRule(str)
		if err != nil {
			return xerrors.Errorf("bucket rule: %v", err)
		}

		bs.rules[index][i] = rule
	}

	bs.buckets[index] = stats
	return nil
}

// Get returns the bucket at the given index if it exists, nil otherwise
func (bs *BucketStats) Get(index int) *Stats {
	s := bs.buckets[index]
	if s != nil {
		s.Collect()
	}

	return s
}

// Update takes a single measure and fill the buckets that will match
// the host index if defined in the measure
func (bs *BucketStats) Update(m *singleMeasure) {
	for i, rr := range bs.rules {
		if rr.Match(m.Host) {
			bs.buckets[i].Update(m)
		}
	}
}
