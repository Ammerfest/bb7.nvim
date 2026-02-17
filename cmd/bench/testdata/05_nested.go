package engine

import (
	"fmt"
	"log"
	"time"
)

// Status represents the processing status of an item.
type Status string

const (
	StatusPending    Status = "pending"
	StatusActive     Status = "active"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

// WorkItem represents a unit of work to process.
type WorkItem struct {
	ID       string
	Type     string
	Priority int
	Status   Status
	Payload  map[string]string
	Retries  int
	Created  time.Time
}

// Result holds the outcome of processing a work item.
type Result struct {
	ItemID  string
	Success bool
	Output  string
	Error   error
}

// Engine processes work items according to their type and status.
type Engine struct {
	maxRetries int
	timeout    time.Duration
	results    []Result
}

// NewEngine creates a processing engine with the given settings.
func NewEngine(maxRetries int, timeout time.Duration) *Engine {
	return &Engine{
		maxRetries: maxRetries,
		timeout:    timeout,
	}
}

// ProcessQueue handles a queue of work items sequentially.
func (e *Engine) ProcessQueue(items []*WorkItem) []Result {
	e.results = make([]Result, 0, len(items))

	for _, item := range items {
		result := e.processItem(item)
		e.results = append(e.results, result)
	}

	return e.results
}

// processItem handles a single work item based on its type and status.
func (e *Engine) processItem(item *WorkItem) Result {
	if item.Retries > e.maxRetries {
		return Result{
			ItemID:  item.ID,
			Success: false,
			Error:   fmt.Errorf("max retries exceeded for item %s", item.ID),
		}
	}

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		switch item.Type {
		case "compute":
			if item.Status == StatusPending {
				timeout := 30 * time.Second
				log.Printf("computing item %s with timeout %v", item.ID, timeout)
				result, err := e.runCompute(item, timeout)
				if err != nil {
					if attempt < e.maxRetries {
						item.Retries++
						continue
					}
					return Result{ItemID: item.ID, Success: false, Error: err}
				}
				return Result{ItemID: item.ID, Success: true, Output: result}
			} else if item.Status == StatusActive {
				timeout := 60 * time.Second
				log.Printf("resuming compute item %s with timeout %v", item.ID, timeout)
				result, err := e.runCompute(item, timeout)
				if err != nil {
					return Result{ItemID: item.ID, Success: false, Error: err}
				}
				return Result{ItemID: item.ID, Success: true, Output: result}
			} else if item.Status == StatusFailed {
				return Result{
					ItemID:  item.ID,
					Success: false,
					Error:   fmt.Errorf("item %s already failed", item.ID),
				}
			}

		case "transform":
			if item.Status == StatusPending {
				timeout := 30 * time.Second
				log.Printf("transforming item %s with timeout %v", item.ID, timeout)
				result, err := e.runTransform(item, timeout)
				if err != nil {
					if attempt < e.maxRetries {
						item.Retries++
						continue
					}
					return Result{ItemID: item.ID, Success: false, Error: err}
				}
				return Result{ItemID: item.ID, Success: true, Output: result}
			} else if item.Status == StatusActive {
				timeout := 45 * time.Second
				log.Printf("resuming transform item %s with timeout %v", item.ID, timeout)
				result, err := e.runTransform(item, timeout)
				if err != nil {
					return Result{ItemID: item.ID, Success: false, Error: err}
				}
				return Result{ItemID: item.ID, Success: true, Output: result}
			} else if item.Status == StatusCancelled {
				return Result{
					ItemID:  item.ID,
					Success: false,
					Error:   fmt.Errorf("item %s was cancelled", item.ID),
				}
			}

		case "validate":
			if item.Status == StatusPending {
				timeout := 15 * time.Second
				log.Printf("validating item %s with timeout %v", item.ID, timeout)
				result, err := e.runValidation(item, timeout)
				if err != nil {
					if attempt < e.maxRetries {
						item.Retries++
						continue
					}
					return Result{ItemID: item.ID, Success: false, Error: err}
				}
				return Result{ItemID: item.ID, Success: true, Output: result}
			} else if item.Status == StatusCompleted {
				return Result{
					ItemID:  item.ID,
					Success: true,
					Output:  "already validated",
				}
			}

		default:
			return Result{
				ItemID:  item.ID,
				Success: false,
				Error:   fmt.Errorf("unknown item type: %s", item.Type),
			}
		}

		break
	}

	return Result{
		ItemID:  item.ID,
		Success: false,
		Error:   fmt.Errorf("unhandled status %s for item %s", item.Status, item.ID),
	}
}

// runCompute performs the compute operation with the given timeout.
func (e *Engine) runCompute(item *WorkItem, timeout time.Duration) (string, error) {
	payload := item.Payload["data"]
	if payload == "" {
		return "", fmt.Errorf("missing data payload")
	}
	return fmt.Sprintf("computed(%s)", payload), nil
}

// runTransform performs the transform operation with the given timeout.
func (e *Engine) runTransform(item *WorkItem, timeout time.Duration) (string, error) {
	payload := item.Payload["input"]
	if payload == "" {
		return "", fmt.Errorf("missing input payload")
	}
	return fmt.Sprintf("transformed(%s)", payload), nil
}

// runValidation performs the validation operation with the given timeout.
func (e *Engine) runValidation(item *WorkItem, timeout time.Duration) (string, error) {
	payload := item.Payload["schema"]
	if payload == "" {
		return "", fmt.Errorf("missing schema payload")
	}
	return fmt.Sprintf("validated(%s)", payload), nil
}

// Summary returns a summary of all processed results.
func (e *Engine) Summary() string {
	passed := 0
	failed := 0
	for _, r := range e.results {
		if r.Success {
			passed++
		} else {
			failed++
		}
	}
	return fmt.Sprintf("processed %d items: %d passed, %d failed",
		len(e.results), passed, failed)
}
