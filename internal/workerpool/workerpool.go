// Package workerpool concurrently submits audit records using a fixed pool
// of worker goroutines, recording each outcome to a checkpoint.
package workerpool

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"audituploader/internal/checkpoint"
	"audituploader/internal/log"
	"audituploader/internal/submitter"
	"audituploader/internal/workbook"
)

// Config controls concurrency and pacing of the submission pool.
type Config struct {
	Workers       int
	Delay         time.Duration // minimum delay between requests, per worker
	RetryAttempts int
	RetryBackoff  time.Duration
}

// Result summarizes how many records succeeded/failed.
type Result struct {
	Total   int
	Success int
	Failed  int
}

// Run submits records concurrently across a fixed pool of worker goroutines,
// writing each outcome to the checkpoint as it completes.
func Run(records []workbook.Record, cfg Config, cp *checkpoint.Writer) Result {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}

	jobs := make(chan workbook.Record)
	var success, failed int64
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 30 * time.Second}

	worker := func() {
		defer wg.Done()
		for rec := range jobs {
			err := submitter.SubmitWithRetry(client, rec, cfg.RetryAttempts, cfg.RetryBackoff)

			entry := checkpoint.Entry{
				Index: rec.Index,
				Sheet: rec.Sheet,
				Row:   rec.Row,
				UHID:  rec.UHID,
			}
			if err != nil {
				entry.Status = "failed"
				entry.Error = err.Error()
				atomic.AddInt64(&failed, 1)
			} else {
				entry.Status = "success"
				atomic.AddInt64(&success, 1)
			}
			if cpErr := cp.Write(entry); cpErr != nil {
				log.Warnf("failed to write checkpoint for record %d: %v", rec.Index, cpErr)
			}

			if err != nil {
				log.Errorf("[%d] %s row %d (UHID %s): FAILED: %v", rec.Index, rec.Sheet, rec.Row, rec.UHID, err)
			} else {
				log.Infof("[%d] %s row %d (UHID %s): OK", rec.Index, rec.Sheet, rec.Row, rec.UHID)
			}

			if cfg.Delay > 0 {
				time.Sleep(cfg.Delay)
			}
		}
	}

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go worker()
	}

	for _, rec := range records {
		jobs <- rec
	}
	close(jobs)

	wg.Wait()

	return Result{
		Total:   len(records),
		Success: int(success),
		Failed:  int(failed),
	}
}
