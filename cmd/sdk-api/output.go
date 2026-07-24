package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// OutputFormat controls how command output is rendered.
type OutputFormat string

const (
	FormatText  OutputFormat = "text"
	FormatJSON  OutputFormat = "json"
	FormatTable OutputFormat = "table"
)

// OutputWriter formats command output based on the selected format.
type OutputWriter struct {
	Format OutputFormat
	Writer io.Writer
}

func NewOutput(w io.Writer) *OutputWriter {
	return &OutputWriter{Format: GetOutputFormat(), Writer: w}
}

func GetOutputFormat() OutputFormat {
	f, _ := rootCmd.Flags().GetString("output")
	switch OutputFormat(f) {
	case FormatJSON, FormatTable:
		return OutputFormat(f)
	default:
		return FormatText
	}
}

// Write renders data in the configured format.
func (o *OutputWriter) Write(data any) error {
	switch o.Format {
	case FormatJSON:
		return o.writeJSON(data)
	case FormatTable:
		return o.writeTable(data)
	default:
		return o.writeText(data)
	}
}

func (o *OutputWriter) writeJSON(data any) error {
	enc := json.NewEncoder(o.Writer)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func (o *OutputWriter) writeText(data any) error {
	switch v := data.(type) {
	case string:
		_, err := fmt.Fprintln(o.Writer, v)
		return err
	case fmt.Stringer:
		_, err := fmt.Fprintln(o.Writer, v.String())
		return err
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(o.Writer, string(b))
		return err
	}
}

func (o *OutputWriter) writeTable(data any) error {
	w := tabwriter.NewWriter(o.Writer, 0, 0, 3, ' ', 0)

	switch v := data.(type) {
	case map[string]string:
		for key, val := range v {
			if _, err := fmt.Fprintf(w, "%s\t%s\n", key, val); err != nil {
				return err
			}
		}
	case []map[string]string:
		if len(v) == 0 {
			return nil
		}
		headers := make([]string, 0, len(v[0]))
		for k := range v[0] {
			headers = append(headers, k)
		}
		if _, err := fmt.Fprintln(w, strings.Join(headers, "\t")); err != nil {
			return err
		}
		for _, row := range v {
			vals := make([]string, 0, len(headers))
			for _, h := range headers {
				vals = append(vals, row[h])
			}
			if _, err := fmt.Fprintln(w, strings.Join(vals, "\t")); err != nil {
				return err
			}
		}
	case []string:
		for _, line := range v {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
	default:
		return o.writeText(data)
	}
	return w.Flush()
}
