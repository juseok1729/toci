package app

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"time"

	"toci/internal/registry"
)

// utf8BOM makes the CSV open correctly in Excel on Windows instead of
// mangling any non-ASCII text (Korean names/descriptions are common in
// this data) — Excel only reliably auto-detects UTF-8 CSV when the file
// starts with this marker.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// detailExportData is what "e" exports while in modeDetail — see
// Model.detailExport.
type detailExportData struct {
	filenameSuffix string
	header         []string
	records        [][]string
}

// writeCSVFile is the actual export mechanics (BOM + header + records),
// shared by every export path in the app — the main table's own columns
// ("e" on a resource list) and derived views like the security rules table
// ("e" from "v") alike.
func writeCSVFile(path string, header []string, records [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(utf8BOM); err != nil {
		return err
	}

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

// exportCSV writes the given rows (already filtered/displayed) to a CSV
// file using the resource's own column definitions, so the export always
// matches exactly what's on screen.
func exportCSV(path string, cols []registry.Column, rows []registry.Row) error {
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Header
	}
	records := make([][]string, len(rows))
	for i, row := range rows {
		record := make([]string, len(cols))
		for j, c := range cols {
			record[j] = c.Get(row)
		}
		records[i] = record
	}
	return writeCSVFile(path, header, records)
}

var filenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// exportFilename builds a path in the current directory, distinct per
// resource/view and export so repeated exports don't clobber each other.
// suffix is sanitized since it can come from user-controlled resource
// names (e.g. a security list's display name).
func exportFilename(suffix string, now time.Time) string {
	suffix = filenameUnsafe.ReplaceAllString(suffix, "-")
	return fmt.Sprintf("toci-%s-%s.csv", suffix, now.Format("20060102-150405"))
}
