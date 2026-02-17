package reports

import (
	"fmt"
	"strings"
	"time"
)

// ReportData contains the data needed to generate a report.
type ReportData struct {
	Title      string
	Author     string
	Date       time.Time
	Sections   []Section
	TotalItems int
	Summary    string
}

// Section is a named part of the report.
type Section struct {
	Name    string
	Content string
	Items   []string
	Count   int
}

// FormatHeader produces the report header line.
func FormatHeader(title string, width int) string {
	padding := width - len(title)
	if padding < 0 {
		padding = 0
	}
	left := padding / 2
	right := padding - left
	return strings.Repeat("=", left) + " " + title + " " + strings.Repeat("=", right)
}

// FormatFooter produces the report footer line.
func FormatFooter(width int) string {
	return strings.Repeat("=", width)
}

// GenerateReport produces a complete text report from the given data.
func GenerateReport(data *ReportData) string {
	var sb strings.Builder

	sb.WriteString(FormatHeader(data.Title, 60))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Author: %s\n", data.Author))
	sb.WriteString(fmt.Sprintf("Date: %s\n", data.Date.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("Total Items: %d\n", data.TotalItems))
	sb.WriteString("\n")

	for i, section := range data.Sections {
		sb.WriteString(fmt.Sprintf("--- Section %d: %s ---\n", i+1, section.Name))
		sb.WriteString(section.Content)
		sb.WriteString("\n")
		if len(section.Items) > 0 {
			sb.WriteString("Items:\n")
			for j, item := range section.Items {
				sb.WriteString(fmt.Sprintf("  %d. %s\n", j+1, item))
			}
		}
		sb.WriteString(fmt.Sprintf("Count: %d\n", section.Count))
		sb.WriteString("\n")
	}

	if data.Summary != "" {
		sb.WriteString("SUMMARY: ")
		sb.WriteString(data.Summary)
		sb.WriteString("\n")
	}

	sb.WriteString(FormatFooter(60))
	sb.WriteString("\n")

	return sb.String()
}

// GenerateCSV produces a CSV export of the report sections.
func GenerateCSV(data *ReportData) string {
	var sb strings.Builder

	sb.WriteString("section,content,count\n")
	for _, section := range data.Sections {
		// Escape quotes in content
		escaped := strings.ReplaceAll(section.Content, "\"", "\"\"")
		sb.WriteString(fmt.Sprintf("%q,%q,%d\n", section.Name, escaped, section.Count))
	}

	return sb.String()
}

// MergeReports combines multiple report data into one.
func MergeReports(reports []*ReportData) *ReportData {
	if len(reports) == 0 {
		return &ReportData{}
	}

	merged := &ReportData{
		Title:  "Merged Report",
		Author: reports[0].Author,
		Date:   time.Now(),
	}

	for _, r := range reports {
		merged.Sections = append(merged.Sections, r.Sections...)
		merged.TotalItems += r.TotalItems
	}

	summaries := make([]string, 0, len(reports))
	for _, r := range reports {
		if r.Summary != "" {
			summaries = append(summaries, r.Summary)
		}
	}
	merged.Summary = strings.Join(summaries, "; ")

	return merged
}

// ValidateReportData checks that the report data is complete.
func ValidateReportData(data *ReportData) error {
	if data.Title == "" {
		return fmt.Errorf("report title is required")
	}
	if data.Author == "" {
		return fmt.Errorf("report author is required")
	}
	if data.Date.IsZero() {
		return fmt.Errorf("report date is required")
	}
	if len(data.Sections) == 0 {
		return fmt.Errorf("report must have at least one section")
	}
	for i, s := range data.Sections {
		if s.Name == "" {
			return fmt.Errorf("section %d: name is required", i)
		}
	}
	return nil
}

// CompactReport produces a one-line summary of the report.
func CompactReport(data *ReportData) string {
	return fmt.Sprintf("[%s] %s by %s (%d sections, %d items)",
		data.Date.Format("2006-01-02"), data.Title, data.Author,
		len(data.Sections), data.TotalItems)
}
