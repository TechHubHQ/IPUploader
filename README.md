# IP Uploader

A high-performance concurrent utility that reads IP prescription audit records from Excel workbooks and submits them to Google Forms. Features include intelligent checkpointing, resume support, configurable concurrency, and comprehensive logging.

## Overview

This tool automates bulk data submission for IP prescription audit records across multiple monthly sheets (JANUARY-AUGUST). It uses a concurrent worker pool to efficiently submit records while maintaining fault tolerance through checkpoint-based recovery.

**Key Features:**

- Read from Excel workbooks with monthly sheet organization
- Concurrent submission with configurable worker pools
- Checkpoint-based resume support for interrupted runs
- Comprehensive logging to console and file
- Flexible filtering by month, record range, or record count
- Dry-run mode for validation without submission
- Retry logic with configurable attempt limits

## Requirements

- Go 1.26+ (see [go.mod](go.mod))
- Excel workbook with one sheet per month (JANUARY-AUGUST)
- Network access to target Google Form

## Installation & Build

```powershell
go build -o ip_uploader.exe ./cmd/uploader
```

## Usage

```text
Usage: ip_uploader.exe -input <file> [options]

Upload audit data from an input workbook.

Required:
  -input <file>  Path to the input Excel file

Options:
  -checkpoint string      Checkpoint CSV file (default "checkpoints/checkpoint.csv")
  -delay duration         Delay between requests per worker (default 250ms)
  -dry-run                Load and filter records without submitting
  -limit int              Limit the number of records to process (all if omitted)
  -month int              Month for the report, e.g., '1' for January
  -month-range string     Month range, e.g., '1-3' for January-March
  -record-index int       Process one record by 1-based index
  -record-range string    Process records in range, e.g., '10-20' (1-based, inclusive)
  -resume                 Resume from latest successful record in checkpoint
  -retries int            Attempts per record before giving up (default 3)
  -workers int            Number of concurrent submission workers (default 5)
```

### Examples

**Validate parsing without submission:**

```powershell
.\ip_uploader.exe -input "IP Rx AUDIT.xlsx" -month 3 -dry-run
```

**Submit March data with 8 concurrent workers:**

```powershell
.\ip_uploader.exe -input "IP Rx AUDIT.xlsx" -month 3 -workers 8
```

**Submit only records 10-20 from March:**

```powershell
.\ip_uploader.exe -input "IP Rx AUDIT.xlsx" -month 3 -record-range 10-20
```

**Resume interrupted run:**

```powershell
.\ip_uploader.exe -input "IP Rx AUDIT.xlsx" -month-range 1-8 -resume
```

**Test with limited records (first 50):**

```powershell
.\ip_uploader.exe -input "IP Rx AUDIT.xlsx" -month-range 1-8 -limit 50
```

## Architecture

### Components

- **workbook** — Reads Excel files using `excelize`, resolves month selections to sheet names, and maps spreadsheet columns to Google Form field identifiers. See [internal/formfields](internal/formfields/formfields.go).

- **submitter** — Submits records using the same multi-page protocol as browsers, including `partialResponse`, `pageHistory`, and freshly fetched `fbzx` tokens. Plain `entry.<id>=value` submissions are silently dropped by this form.

- **workerpool** — Manages a fixed pool of concurrent goroutines that pull records from a channel, submit them, and write results to the checkpoint in real-time.

- **checkpoint** — Persists submission outcomes as CSV (`index,sheet,row,uhid,status,timestamp,error`), enabling safe resumption after interruptions. The `-resume` flag skips all records marked `success`.

- **log** — Dual-output logger: Info/Error to both console and file; Warn/Debug to file only for readability without verbosity.

- **cli** — Command-line interface with flag parsing and validation.

## Project Structure

```text
IPUploader/
├── cmd/uploader/
│   └── main.go                     # Entry point: initializes logging, CLI, and orchestration
├── internal/
│   ├── cli/                        # Command-line flag parsing and validation
│   ├── formfields/                 # Google Form field mapping and value normalization
│   ├── workbook/                   # Excel file reading and sheet/record processing
│   ├── submitter/                  # HTTP submission with form protocol handling
│   ├── workerpool/                 # Concurrent worker pool management
│   ├── checkpoint/                 # Checkpoint CSV persistence and recovery
│   └── log/                        # Structured logging (console + file)
├── scripts/
│   └── release.ps1                 # Build and release automation
├── test/
│   └── sample-form-request.ps1     # Example captured request
├── checkpoints/                    # Checkpoint CSV files (default storage location)
├── logs/                           # Log output organized by date
├── go.mod                          # Go module definition
└── README.md                       # This file
```

## Logging

Logs are written to `logs/<YYYY-MM-DD>/` with timestamped filenames. The logging system uses separate levels:

- **Console**: Info and Error only (keeps terminal clean)
- **File**: All levels (Info, Warn, Error, Debug) for complete audit trail

Check log files for detailed troubleshooting information if submissions fail.

## Checkpoint Format

The checkpoint CSV records the outcome of each submission attempt:

```csv
index,sheet,row,uhid,status,timestamp,error
1,JANUARY,2,IP001,success,2026-09-03T14:23:45Z,
2,JANUARY,3,IP002,failed,2026-09-03T14:23:47Z,"HTTP 400: Invalid field"
```

**Fields:**

- `index`: Submission sequence number
- `sheet`: Month/sheet name
- `row`: Row number in the Excel sheet
- `uhid`: Unique identifier from the record
- `status`: `success`, `failed`, or `pending`
- `timestamp`: ISO 8601 timestamp
- `error`: Error message (empty if successful)

## Troubleshooting

**Submission failures:** Check the log file and checkpoint CSV for detailed error messages. Common issues:

- Network connectivity or timeouts
- Invalid field values (use `-dry-run` to validate data format)
- Form field mapping mismatch (verify [formfields.go](internal/formfields/formfields.go))

**Resume not working:** Ensure the checkpoint file path matches and contains completed records marked `success`.

**High failure rate:** Try increasing `-delay` to reduce request rate, or decreasing `-workers` if the form is rejecting concurrent requests.

## Development

The project is built with Go 1.26+ and uses standard library plus `excelize` for Excel support. To build locally:

```powershell
# Build executable
go build -o ip_uploader.exe ./cmd/uploader

# Run tests (if any)
go test ./...

# Create a release
.\scripts\release.ps1 -version v1.0.0
```

## License

[Add license information here]
