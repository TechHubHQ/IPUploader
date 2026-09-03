// Package cli implements the audit_uploader command-line interface: flag
// parsing, usage text, and orchestration of the loading/checkpoint/worker
// pool packages.
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xuri/excelize/v2"

	"audituploader/internal/checkpoint"
	"audituploader/internal/log"
	"audituploader/internal/workbook"
	"audituploader/internal/workerpool"
)

func usage() {
	exe := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `Usage: %s -input <file> [options]

Upload audit data from an input file.

Required:
  -input <file>  Path to the input file.

Options:
  -checkpoint string
        Checkpoint CSV file (default "checkpoints/checkpoint.csv")
  -input string
        Path to the input file (required)
  -limit int
        Limit the number of records to process (optional, defaults to no limit)
  -month int
        Month for the report, e.g., '1' for January (optional, defaults to the latest sheet)
  -month-range string
        Month range for the report, e.g., '1-3' for January-March (optional, defaults to the latest sheet)
  -record-index int
        Process one record by its 1-based index
  -record-range string
        Process an inclusive 1-based record range, for example 10-20
  -resume
        Resume from the latest successful record in the checkpoint
  -workers int
        Number of concurrent workers submitting records (default 5)
  -delay duration
        Delay between requests issued by each worker, e.g. "250ms" (default 250ms)
  -retries int
        Number of attempts per record before giving up (default 3)
  -dry-run
        Load and filter records but do not submit anything
`, exe)
}

// Run parses command-line flags and executes the uploader, returning a
// process exit code.
func Run() int {
	flag.Usage = usage

	inputPath := flag.String("input", "", "Path to the input file (required)")
	checkpointPath := flag.String("checkpoint", "checkpoints/checkpoint.csv", "Checkpoint CSV file")
	limit := flag.Int("limit", 0, "Limit the number of records to process (optional, defaults to no limit)")
	month := flag.Int("month", 0, "Month for the report, e.g., '1' for January (optional, defaults to the latest sheet)")
	monthRange := flag.String("month-range", "", "Month range for the report, e.g., '1-3' for January-March (optional, defaults to the latest sheet)")
	recordIndex := flag.Int("record-index", 0, "Process one record by its 1-based index")
	recordRange := flag.String("record-range", "", "Process an inclusive 1-based record range, for example 10-20")
	resume := flag.Bool("resume", false, "Resume from the latest successful record in the checkpoint")
	workers := flag.Int("workers", 5, "Number of concurrent workers submitting records")
	delay := flag.Duration("delay", 250*time.Millisecond, "Delay between requests issued by each worker")
	retries := flag.Int("retries", 3, "Number of attempts per record before giving up")
	dryRun := flag.Bool("dry-run", false, "Load and filter records but do not submit anything")

	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -input is required")
		usage()
		return 2
	}

	if err := execute(config{
		inputPath:      *inputPath,
		checkpointPath: *checkpointPath,
		limit:          *limit,
		month:          *month,
		monthRange:     *monthRange,
		recordIndex:    *recordIndex,
		recordRange:    *recordRange,
		resume:         *resume,
		workers:        *workers,
		delay:          *delay,
		retries:        *retries,
		dryRun:         *dryRun,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

type config struct {
	inputPath      string
	checkpointPath string
	limit          int
	month          int
	monthRange     string
	recordIndex    int
	recordRange    string
	resume         bool
	workers        int
	delay          time.Duration
	retries        int
	dryRun         bool
}

func execute(cfg config) error {
	log.Banner("Starting Audit Uploader")
	defer log.Banner("Completed Audit Uploader")

	log.Infof("Input file: %s", cfg.inputPath)
	log.Infof("Checkpoint file: %s", cfg.checkpointPath)
	log.Infof("Workers: %d, delay: %s, retries: %d", cfg.workers, cfg.delay, cfg.retries)

	f, err := excelize.OpenFile(cfg.inputPath)
	if err != nil {
		return fmt.Errorf("opening input file: %w", err)
	}
	defer f.Close()

	sheets, err := workbook.SheetsForMonths(f, cfg.month, cfg.monthRange)
	if err != nil {
		return err
	}
	log.Infof("Selected sheets: %v", sheets)

	records, err := workbook.LoadRecords(f, sheets)
	if err != nil {
		return err
	}
	log.Infof("Loaded %d records", len(records))

	records, err = filterRecords(records, cfg)
	if err != nil {
		return err
	}
	log.Infof("Processing %d records after filters", len(records))

	if len(records) == 0 {
		log.Info("Nothing to do.")
		return nil
	}

	if cfg.dryRun {
		for _, r := range records {
			log.Infof("[%d] %s row %d UHID=%s fields=%d", r.Index, r.Sheet, r.Row, r.UHID, len(r.Values))
		}
		log.Info("Dry run complete; nothing submitted.")
		return nil
	}

	cp, err := checkpoint.NewWriter(cfg.checkpointPath)
	if err != nil {
		return err
	}
	defer cp.Close()

	result := workerpool.Run(records, workerpool.Config{
		Workers:       cfg.workers,
		Delay:         cfg.delay,
		RetryAttempts: cfg.retries,
		RetryBackoff:  time.Second,
	}, cp)

	log.Infof("Done. total=%d success=%d failed=%d", result.Total, result.Success, result.Failed)
	if result.Failed > 0 {
		return fmt.Errorf("%d record(s) failed to submit", result.Failed)
	}
	return nil
}

// filterRecords applies -resume, -record-index, -record-range and -limit, in
// that order, to the full loaded record set.
func filterRecords(records []workbook.Record, cfg config) ([]workbook.Record, error) {
	if cfg.recordIndex != 0 && cfg.recordRange != "" {
		return nil, fmt.Errorf("-record-index and -record-range are mutually exclusive")
	}

	if cfg.recordIndex != 0 {
		for _, r := range records {
			if r.Index == cfg.recordIndex {
				return []workbook.Record{r}, nil
			}
		}
		return nil, fmt.Errorf("record index %d not found (loaded %d records)", cfg.recordIndex, len(records))
	}

	if cfg.recordRange != "" {
		var start, end int
		if _, err := fmt.Sscanf(cfg.recordRange, "%d-%d", &start, &end); err != nil {
			return nil, fmt.Errorf("invalid -record-range %q, expected e.g. \"10-20\": %w", cfg.recordRange, err)
		}
		if start > end {
			return nil, fmt.Errorf("invalid -record-range %q: start must be <= end", cfg.recordRange)
		}
		var out []workbook.Record
		for _, r := range records {
			if r.Index >= start && r.Index <= end {
				out = append(out, r)
			}
		}
		return out, nil
	}

	if cfg.resume {
		existing, err := checkpoint.Load(cfg.checkpointPath)
		if err != nil {
			return nil, err
		}
		latest := checkpoint.LatestSuccessfulIndex(existing)
		if latest > 0 {
			log.Infof("Resuming after record %d (latest successful in checkpoint)", latest)
			var out []workbook.Record
			for _, r := range records {
				if r.Index > latest {
					out = append(out, r)
				}
			}
			records = out
		}
	}

	if cfg.limit > 0 && cfg.limit < len(records) {
		records = records[:cfg.limit]
	}

	return records, nil
}
