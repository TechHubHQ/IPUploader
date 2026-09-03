// Package submitter posts audit records to the Google Form endpoint.
//
// Plain "entry.<id>=value" submissions return an HTTP 200 confirmation page
// on this form but are silently discarded and never appear in the response
// sheet: the form has a multi-page/branching layout, and Google Forms only
// records the response when the request also carries the same
// partialResponse + pageHistory + fbzx fields a real browser sends. So
// Submit replicates that exact protocol instead of the simpler classic
// method.
package submitter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"audituploader/internal/formfields"
	"audituploader/internal/log"
	"audituploader/internal/workbook"
)

const (
	formURL     = "https://docs.google.com/forms/d/e/1FAIpQLScuaiAFmILMbBua-wxXuQqh-3_uJAg_bxmSbFNix8kiw5LiGQ/formResponse"
	viewformURL = "https://docs.google.com/forms/d/e/1FAIpQLScuaiAFmILMbBua-wxXuQqh-3_uJAg_bxmSbFNix8kiw5LiGQ/viewform"

	// pageHistory mirrors the page-navigation trace a real browser produced
	// while submitting this form. Google validates it alongside fbzx as
	// proof the request came from an actual multi-page form session.
	pageHistory = "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,4,5,6,7,8,9,10,11,12,13,14,15,16,17,19,21,23,25,26,27,28,29,30,31,32,33,34,35,37,38,39,40,41,42,43,44,45,46,47,49,50,51,52,53,54,55,56,57,58,59,60,61,63,65,67,69,71,72,73"
)

var fbzxPattern = regexp.MustCompile(`name="fbzx"\s+value="(-?\d+)"`)

// fetchFBZX loads the live form page and extracts the session token Google
// ties to a single page-load. A fresh one is fetched per submission (rather
// than reused) so concurrent workers don't collide on the same token.
func fetchFBZX(client *http.Client) (string, error) {
	resp, err := client.Get(viewformURL)
	if err != nil {
		return "", fmt.Errorf("fetching form page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading form page: %w", err)
	}

	m := fbzxPattern.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("fbzx token not found on form page")
	}
	return string(m[1]), nil
}

// buildPartialResponse encodes every answer except AuditObservationsEntry
// into the JSON array Google's multi-page form protocol expects, matching
// [[[null, entryID, [value], 0], ...], null, "<fbzx>"].
func buildPartialResponse(rec workbook.Record, fbzx string) (string, error) {
	items := make([][]any, 0, len(rec.Values))
	for key, vals := range rec.Values {
		if key == formfields.AuditObservationsEntry {
			continue
		}
		idStr := strings.TrimPrefix(key, "entry.")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid entry key %q: %w", key, err)
		}
		items = append(items, []any{nil, id, vals, 0})
	}

	payload := []any{items, nil, fbzx}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Submit POSTs a single record to the Google Form, replicating a real
// multi-page browser submission (see package doc).
func Submit(client *http.Client, rec workbook.Record) error {
	fbzx, err := fetchFBZX(client)
	if err != nil {
		return fmt.Errorf("getting form session token: %w", err)
	}

	partial, err := buildPartialResponse(rec, fbzx)
	if err != nil {
		return fmt.Errorf("building partial response: %w", err)
	}

	body := url.Values{}
	if v := rec.Values.Get(formfields.AuditObservationsEntry); v != "" {
		body.Set(formfields.AuditObservationsEntry, v)
	}
	body.Set("fvv", "1")
	body.Set("partialResponse", partial)
	body.Set("pageHistory", pageHistory)
	body.Set("fbzx", fbzx)
	body.Set("submissionTimestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	req, err := http.NewRequest(http.MethodPost, formURL, strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://docs.google.com")
	req.Header.Set("Referer", "https://docs.google.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	// Google always answers with HTTP 200 for this endpoint; a rejected or
	// silently dropped submission still needs to be detected from the body.
	accepted := resp.StatusCode == http.StatusOK && !strings.Contains(string(respBody), "Something went wrong")
	if !accepted {
		log.Debugf("record %d submission rejected; fbzx=%s posted fields: %v; response body: %s",
			rec.Index, fbzx, rec.Values, truncate(string(respBody), 2000))
		return fmt.Errorf("form did not accept submission (status %s)", resp.Status)
	}
	return nil
}

// truncate shortens s to at most n runes for inclusion in log output.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// SubmitWithRetry retries transient failures a limited number of times with
// a short backoff before giving up.
func SubmitWithRetry(client *http.Client, rec workbook.Record, attempts int, backoff time.Duration) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(backoff * time.Duration(i))
		}
		if err := Submit(client, rec); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}
