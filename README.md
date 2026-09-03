# Audit Uploader

Reads IP prescription audit records from an Excel workbook (one sheet per
month, JANUARY-AUGUST) and submits each row to the corresponding Google Form,
using a concurrent worker pool with checkpointing and resume support.

## Requirements

- Go 1.26+ (see `go.mod`)
- The source workbook (e.g. `IP Rx AUDIT.xlsx`) with one sheet per month

## Build

```powershell
go build -o audit_uploader.exe ./cmd/uploader
```

## Usage

```text
Usage: audit_uploader.exe -input <file> [options]

Upload audit data from an input file.

Required:
  -input <file>  Path to the input file.

Options:
  -checkpoint string
        Checkpoint CSV file (default "checkpoints/checkpoint.csv")
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
```

### Examples

Validate parsing without submitting anything:

```powershell
.\audit_uploader.exe -input "IP Rx AUDIT.xlsx" -month 3 -dry-run
```

Submit every record in March with 8 concurrent workers:

```powershell
.\audit_uploader.exe -input "IP Rx AUDIT.xlsx" -month 3 -workers 8
```

Submit only records 10-20 (1-based, within the selected month(s)):

```powershell
.\audit_uploader.exe -input "IP Rx AUDIT.xlsx" -month 3 -record-range 10-20
```

Resume a previously interrupted run using its checkpoint file:

```powershell
.\audit_uploader.exe -input "IP Rx AUDIT.xlsx" -month-range 1-8 -resume
```

## How it works

- **workbook** reads the workbook with `excelize`, resolves `-month` /
  `-month-range` to sheet names, and maps each sheet's header columns onto
  Google Form entry fields positionally (see `internal/formfields`).
- **submitter** posts each record using the same multi-page protocol a real
  browser uses (`partialResponse` + `pageHistory` + a freshly fetched
  `fbzx` token) — plain `entry.<id>=value` submissions return HTTP 200 on
  this form but are silently dropped.
- **workerpool** runs a fixed pool of goroutines pulling records off a
  channel, submitting concurrently, and writing each outcome to the
  checkpoint as it completes.
- **checkpoint** persists `index,sheet,row,uhid,status,timestamp,error` rows
  to CSV so a run can be resumed (`-resume` continues after the highest
  index marked `success`).
- **log** writes Info/Error to both the console and the log file, and
  Warn/Debug to the log file only, so the terminal stays readable while the
  file keeps the full trace.

## File structure

```text
audit_uploaderv2/
├── cmd/
│   └── uploader/
│       └── main.go        # Entry point: initializes logging, runs the CLI
├── internal/
│   ├── cli/               # Flag parsing, usage text, run orchestration
│   ├── formfields/        # Google Form entry-ID mapping + choice/date normalization
│   ├── workbook/          # Excel loading, Record type, sheet resolution
│   ├── submitter/         # HTTP submission (partialResponse/pageHistory/fbzx protocol)
│   ├── workerpool/        # Concurrent submission pool
│   ├── checkpoint/        # Checkpoint CSV read/write
│   └── log/               # Leveled, concurrency-safe logging
├── scripts/
│   └── release.ps1        # Build + tag + GitHub release automation
├── testdata/
│   └── sample-form-request.ps1  # Reference captured request (see file for context)
├── checkpoints/            # Default checkpoint CSV location
├── logs/                   # Per-run log files: logs/<date>/auditUploader_<time>.log
├── IP Rx AUDIT.xlsx        # Source workbook (one sheet per month)
├── go.mod / go.sum
└── README.md
```
