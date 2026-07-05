package format

import "fmt"

// CountLineOptions configures count line formatting.
type CountLineOptions struct {
	Count        int
	Limit        *int
	TotalCount   *int
	APILimitHit  bool
	DisplayLimit *int
}

// CountLine formats a consistent count line.
func CountLine(opts CountLineOptions) string {
	if opts.APILimitHit {
		return fmt.Sprintf("count: %d+ (GitHub search API limit reached)", opts.Count)
	}
	if opts.TotalCount != nil {
		return fmt.Sprintf("count: %d of %d total", opts.Count, *opts.TotalCount)
	}
	if opts.DisplayLimit != nil && opts.Count > *opts.DisplayLimit {
		return fmt.Sprintf("count: %d (showing first %d)", opts.Count, *opts.DisplayLimit)
	}
	if opts.Limit != nil && opts.Count == *opts.Limit && opts.Count > 0 {
		return fmt.Sprintf("count: %d (showing first %d)", opts.Count, opts.Count)
	}
	return fmt.Sprintf("count: %d", opts.Count)
}
