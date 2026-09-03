// Package workbook loads audit records from the source Excel workbook and
// maps each row's columns onto Google Form entry fields.
package workbook

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"audituploader/internal/formfields"
	"audituploader/internal/log"
)

// monthNames maps a 1-based month number to the sheet name used in the
// workbook. Only January-August are present in the source data.
var monthNames = []string{
	"", // 0 unused, months are 1-based
	"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY", "AUGUST",
}

// Record is a single audit row ready to be submitted to the Google Form.
type Record struct {
	Index  int // 1-based index within the final, filtered record set
	Sheet  string
	Row    int // 1-based row number within the sheet (excluding header)
	UHID   string
	Values url.Values
}

// buildColumnEntryMap maps each "real" (non-blank, non-"NA") header column
// index to the Google Form entry field it corresponds to, based on position.
func buildColumnEntryMap(headers []string) (map[int]string, error) {
	var realCols []int
	for i, h := range headers {
		trimmed := strings.TrimSpace(h)
		if trimmed == "" || strings.EqualFold(trimmed, "NA") {
			continue
		}
		realCols = append(realCols, i)
	}

	var canonical []string
	switch len(realCols) {
	case len(formfields.Canonical):
		canonical = formfields.Canonical
	case len(formfields.Canonical) - formfields.GenericNamesFieldCount():
		canonical = formfields.WithoutGenericNames()
	default:
		return nil, fmt.Errorf("unexpected header column count %d (expected %d or %d); headers: %v",
			len(realCols), len(formfields.Canonical), len(formfields.Canonical)-formfields.GenericNamesFieldCount(), headers)
	}

	colToEntry := make(map[int]string, len(realCols))
	for i, col := range realCols {
		colToEntry[col] = canonical[i]
	}
	return colToEntry, nil
}

// SheetsForMonths resolves the -month / -month-range flags (or the default
// "latest sheet" behavior) to a concrete, ordered list of sheet names.
func SheetsForMonths(f *excelize.File, month int, monthRange string) ([]string, error) {
	available := f.GetSheetList()
	availableSet := make(map[string]bool, len(available))
	for _, s := range available {
		availableSet[s] = true
	}

	nameFor := func(m int) (string, error) {
		if m < 1 || m >= len(monthNames) {
			return "", fmt.Errorf("month %d is out of range (1-%d)", m, len(monthNames)-1)
		}
		name := monthNames[m]
		if !availableSet[name] {
			return "", fmt.Errorf("sheet %q for month %d not found in workbook", name, m)
		}
		return name, nil
	}

	switch {
	case monthRange != "":
		var start, end int
		if _, err := fmt.Sscanf(monthRange, "%d-%d", &start, &end); err != nil {
			return nil, fmt.Errorf("invalid -month-range %q, expected e.g. \"1-3\": %w", monthRange, err)
		}
		if start > end {
			return nil, fmt.Errorf("invalid -month-range %q: start must be <= end", monthRange)
		}
		var sheets []string
		for m := start; m <= end; m++ {
			name, err := nameFor(m)
			if err != nil {
				return nil, err
			}
			sheets = append(sheets, name)
		}
		return sheets, nil

	case month != 0:
		name, err := nameFor(month)
		if err != nil {
			return nil, err
		}
		return []string{name}, nil

	default:
		// Default to the latest sheet with data.
		for i := len(available) - 1; i >= 0; i-- {
			rows, err := f.GetRows(available[i])
			if err == nil && len(rows) > 1 {
				return []string{available[i]}, nil
			}
		}
		if len(available) == 0 {
			return nil, fmt.Errorf("workbook has no sheets")
		}
		return []string{available[len(available)-1]}, nil
	}
}

// LoadRecords reads every data row from the given sheets and converts each
// one into a Record ready for submission, numbering them sequentially.
func LoadRecords(f *excelize.File, sheets []string) ([]Record, error) {
	var records []Record
	index := 0
	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("reading sheet %q: %w", sheet, err)
		}
		if len(rows) == 0 {
			continue
		}
		header := rows[0]
		colToEntry, err := buildColumnEntryMap(header)
		if err != nil {
			return nil, fmt.Errorf("sheet %q: %w", sheet, err)
		}

		for r, row := range rows[1:] {
			rowNum := r + 2 // 1-based, +1 for header row
			if isBlankRow(row) {
				continue
			}
			values := url.Values{}
			var uhid string
			for col, entry := range colToEntry {
				var cell string
				if col < len(row) {
					cell = strings.TrimSpace(row[col])
				}
				if cell == "" {
					continue
				}

				if entry == formfields.AuditDateEntry {
					cell = normalizeDate(cell, sheet, rowNum)
				} else if normalized, ok := formfields.NormalizeChoice(entry, cell); ok {
					cell = normalized
				} else {
					log.Warnf("%s row %d: value %q for %s doesn't match any known option; submitting as-is", sheet, rowNum, cell, entry)
				}

				values.Set(entry, cell)
				if col == 0 {
					uhid = cell
				}
			}
			index++
			records = append(records, Record{
				Index:  index,
				Sheet:  sheet,
				Row:    rowNum,
				UHID:   uhid,
				Values: values,
			})
		}
	}
	return records, nil
}

// auditDateLayouts are the date formats seen across monthly sheets: numeric
// "MM-DD-YY" (e.g. "03-01-26") and "D-Mon-YY" (e.g. "2-Apr-26").
var auditDateLayouts = []string{"01-02-06", "2-Jan-06"}

// normalizeDate reformats an Audit Date cell to "2006-01-02", the format
// Google Forms' native date picker requires. If the value doesn't match any
// known layout, it's returned unchanged and a warning is logged.
func normalizeDate(cell, sheet string, rowNum int) string {
	for _, layout := range auditDateLayouts {
		if t, err := time.Parse(layout, cell); err == nil {
			return t.Format("2006-01-02")
		}
	}
	log.Warnf("%s row %d: could not parse Audit Date %q; submitting as-is", sheet, rowNum, cell)
	return cell
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
