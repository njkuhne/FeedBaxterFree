package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/service"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

const (
	exportPage          = 500 // rows between flushes; matches the service's page size
	exportWriteDeadline = 30 * time.Second
)

var csvHeader = []string{"id", "received_at", "status", "channel", "user_id", "platform",
	"app_version", "tags", "message", "metadata"}

// guardCell stops a spreadsheet from running a cell as a formula.
func guardCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func csvRecord(f store.Feedback) []string {
	return []string{
		guardCell(f.ID), fmtMs(f.ReceivedAt), f.Status, guardCell(deref(f.ChannelName)),
		guardCell(deref(f.UserID)), guardCell(deref(f.Platform)), guardCell(deref(f.AppVersion)),
		guardCell(strings.Join(f.Tags, "; ")), guardCell(f.Message), string(f.Metadata),
	}
}

// exportFilename names the download: feedbaxter-<status>[-<tag>]-<date>.<format>.
// Channel, search and user filters stay out of the name (they can hold
// personal or long text).
func exportFilename(f service.Filter, format string, now time.Time) string {
	name := "feedbaxter-"
	var sts []string
	for _, st := range f.Statuses {
		if s := slugTag(st); s != "" { // statuses are checked later; keep the header safe
			sts = append(sts, s)
		}
	}
	if len(sts) == 0 {
		name += "all"
	} else {
		name += strings.Join(sts, "-")
	}
	if t := slugTag(f.Tag); t != "" {
		name += "-" + t
	}
	return name + "-" + now.UTC().Format("2006-01-02") + "." + format
}

// slugTag lowercases tag, turns every run of characters outside [a-z0-9] into
// one "-", trims "-" from both ends and cuts the result to 40 characters.
func slugTag(tag string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(tag) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	s := b.String()
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.TrimRight(s, "-")
}

// exportFeedback streams every matching item. Format and filters are checked
// before any header is written; headers go out with the first row (or at the
// end when nothing matched), so an error in the first page is still a proper
// JSON error. After that the stream can only be cut.
func (a *api) exportFeedback(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	format := r.URL.Query().Get("format")
	if format != "csv" && format != "json" {
		return invalidRequest("Format must be csv or json.")
	}
	f, err := filterFrom(r)
	if err != nil {
		return err
	}
	rc := http.NewResponseController(w)
	extend := func() { _ = rc.SetWriteDeadline(time.Now().Add(exportWriteDeadline)) }

	var cw *csv.Writer
	started, n := false, 0
	start := func() error {
		extend()
		h := w.Header()
		if format == "csv" {
			h.Set("Content-Type", "text/csv; charset=utf-8")
		} else {
			h.Set("Content-Type", "application/json")
		}
		h.Set("Content-Disposition", `attachment; filename="`+exportFilename(f, format, time.Now())+`"`)
		w.WriteHeader(http.StatusOK)
		started = true
		if format == "csv" {
			cw = csv.NewWriter(w)
			cw.UseCRLF = true
			return cw.Write(csvHeader)
		}
		_, err := w.Write([]byte("["))
		return err
	}
	flush := func() error {
		if cw != nil {
			cw.Flush()
			if err := cw.Error(); err != nil {
				return err
			}
		}
		return rc.Flush()
	}

	err = a.svc.Export(r.Context(), p, f, func(fb store.Feedback) error {
		if !started {
			if err := start(); err != nil {
				return err
			}
		}
		if format == "csv" {
			if err := cw.Write(csvRecord(fb)); err != nil {
				return err
			}
		} else {
			b, err := json.Marshal(newFeedbackJSON(fb))
			if err != nil {
				return err
			}
			if n > 0 {
				b = append([]byte{','}, b...)
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
		}
		n++
		if n%exportPage == 0 {
			extend()
			return flush()
		}
		return nil
	})
	if err != nil && !started {
		return err
	}
	if err == nil && !started {
		err = start()
	}
	if err == nil {
		if format == "json" {
			_, err = w.Write([]byte("]"))
		}
		if err == nil {
			err = flush()
		}
	}
	if err != nil {
		// Too late for a status. Cut the connection so the client sees a
		// failed download instead of a file that looks complete.
		a.log.Error("api: export stopped", "err", err, "rows", n)
		panic(http.ErrAbortHandler)
	}
	return nil
}
