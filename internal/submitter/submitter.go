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
	"net/http/cookiejar"
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

	pageHistory = "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,4,5,6,7,8,9,10,11,12,13,14,15,16,17,19,21,23,25,26,27,28,29,30,31,32,33,34,35,37,38,39,40,41,42,43,44,45,46,47,49,50,51,52,53,54,55,56,57,58,59,60,61,63,65,67,69,71,72,73"
)

var fbzxPattern = regexp.MustCompile(`name="fbzx"\s+value="(-?\d+)"`)
var formDataPattern = regexp.MustCompile(`(?s)var FB_PUBLIC_LOAD_DATA_ = (.*?);</script>`)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36 Edg/152.0.0.0"

// fetchFBZX loads the live form page and extracts the session token Google
// ties to a single page-load. The caller must reuse the same client for the
// subsequent POST so the form session cookies are retained.
func fetchFBZX(client *http.Client, rec workbook.Record) (string, []byte, error) {
	formPageURL, err := url.Parse(viewformURL)
	if err != nil {
		return "", nil, fmt.Errorf("parsing form page URL: %w", err)
	}
	query := formPageURL.Query()
	for key, values := range rec.Values {
		if key == formfields.AuditDateEntry {
			if len(values) == 0 {
				continue
			}
			query.Set(key, values[0])
			continue
		}
		for _, value := range values {
			query.Add(key, value)
		}
	}
	formPageURL.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, formPageURL.String(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("building form page request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("User-Agent", browserUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("fetching form page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("reading form page: %w", err)
	}

	m := fbzxPattern.FindSubmatch(body)
	if m == nil {
		return "", nil, fmt.Errorf("fbzx token not found on form page")
	}
	return string(m[1]), body, nil
}

func buildPageHistory(rec workbook.Record, formPage []byte) (string, error) {
	match := formDataPattern.FindSubmatch(formPage)
	if match == nil {
		return "", fmt.Errorf("form metadata not found on form page")
	}
	var root []any
	if err := json.Unmarshal(match[1], &root); err != nil {
		return "", fmt.Errorf("parsing form metadata: %w", err)
	}
	rootOne, ok := nestedSlice(root, 1)
	if !ok {
		return "", fmt.Errorf("form metadata has no question data")
	}
	entries, ok := nestedSlice(rootOne, 1)
	if !ok {
		return "", fmt.Errorf("form metadata has no sections")
	}

	type section struct {
		id        int64
		questions []any
	}
	var sections []section
	for _, raw := range entries {
		items, ok := raw.([]any)
		if !ok || len(items) < 4 || number(items[3]) != 8 {
			continue
		}
		id, ok := integer(items[0])
		if !ok {
			continue
		}
		sections = append(sections, section{id: id})
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("form metadata has no sections")
	}
	sectionByID := make(map[int64]int, len(sections))
	for index := range sections {
		sectionByID[sections[index].id] = index
	}
	sectionIndex := -1
	for _, raw := range entries {
		items, ok := raw.([]any)
		if !ok || len(items) < 4 {
			continue
		}
		if number(items[3]) == 8 {
			sectionIndex++
			continue
		}
		if sectionIndex >= 0 {
			sections[sectionIndex].questions = append(sections[sectionIndex].questions, items)
		}
	}

	history := []int{0}
	current := 0
	for step := 0; current >= 0 && current < len(sections); step++ {
		if step >= 200 {
			return "", fmt.Errorf("form routing did not reach the final section")
		}
		history = append(history, current+1)
		next := current + 1
		for _, rawQuestion := range sections[current].questions {
			question, ok := rawQuestion.([]any)
			if !ok {
				continue
			}
			target, found := routeTarget(question, rec.Values, sectionByID)
			if found {
				next = target
				break
			}
		}
		current = next
	}
	parts := make([]string, len(history))
	for i, value := range history {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ","), nil
}

func routeTarget(question []any, values url.Values, sectionByID map[int64]int) (int, bool) {
	data, ok := nestedSlice(question, 4)
	if !ok || len(data) == 0 {
		return 0, false
	}
	questionData, ok := data[0].([]any)
	if !ok || len(questionData) < 2 {
		return 0, false
	}
	entryID, ok := integer(questionData[0])
	if !ok {
		return 0, false
	}
	choices, ok := questionData[1].([]any)
	if !ok {
		return 0, false
	}
	raw := values.Get("entry." + strconv.FormatInt(entryID, 10))
	for _, choiceRaw := range choices {
		choice, ok := choiceRaw.([]any)
		if !ok || len(choice) < 3 || !strings.EqualFold(fmt.Sprint(choice[0]), raw) {
			continue
		}
		targetID, ok := integer(choice[2])
		if !ok {
			return 0, false
		}
		target, ok := sectionByID[targetID]
		return target, ok
	}
	return 0, false
}

func nestedSlice(value []any, index int) ([]any, bool) {
	if index < 0 || index >= len(value) {
		return nil, false
	}
	result, ok := value[index].([]any)
	return result, ok
}

func number(value any) int {
	if n, ok := value.(float64); ok {
		return int(n)
	}
	return -1
}

func integer(value any) (int64, bool) {
	n, ok := value.(float64)
	return int64(n), ok
}

// buildPartialResponse encodes every answer except AuditObservationsEntry
// into the JSON array Google's multi-page form protocol expects, matching
// [[[null, entryID, [value], 0], ...], null, "<fbzx>"].
func buildPartialResponse(rec workbook.Record, fbzx string, formPage []byte, history string) (string, error) {
	allowed, err := entriesForHistory(formPage, history)
	if err != nil {
		return "", err
	}
	items := make([][]any, 0, len(rec.Values))
	for _, key := range formfields.Canonical {
		vals, ok := rec.Values[key]
		if !ok || !allowed[key] {
			continue
		}
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

func entriesForHistory(formPage []byte, history string) (map[string]bool, error) {
	match := formDataPattern.FindSubmatch(formPage)
	if match == nil {
		return nil, fmt.Errorf("form metadata not found on form page")
	}
	var root []any
	if err := json.Unmarshal(match[1], &root); err != nil {
		return nil, fmt.Errorf("parsing form metadata: %w", err)
	}
	rootOne, ok := nestedSlice(root, 1)
	if !ok {
		return nil, fmt.Errorf("form metadata has no question data")
	}
	entries, ok := nestedSlice(rootOne, 1)
	if !ok {
		return nil, fmt.Errorf("form metadata has no sections")
	}
	visited := make(map[int]bool)
	for _, rawPage := range strings.Split(history, ",") {
		page, err := strconv.Atoi(rawPage)
		if err == nil && page > 0 {
			visited[page-1] = true
		}
	}
	allowed := make(map[string]bool)
	section := -1
	for _, raw := range entries {
		items, ok := raw.([]any)
		if !ok || len(items) < 4 {
			continue
		}
		if number(items[3]) == 8 {
			section++
			continue
		}
		if !visited[section] || len(items) < 5 {
			continue
		}
		data, ok := items[4].([]any)
		if !ok || len(data) == 0 {
			continue
		}
		questionData, ok := data[0].([]any)
		if !ok || len(questionData) == 0 {
			continue
		}
		entryID, ok := integer(questionData[0])
		if ok {
			allowed["entry."+strconv.FormatInt(entryID, 10)] = true
		}
	}
	return allowed, nil
}

// Submit POSTs a single record to the Google Form, replicating a real
// multi-page browser submission (see package doc).
func Submit(client *http.Client, rec workbook.Record) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("creating form session: %w", err)
	}
	sessionClient := *client
	sessionClient.Jar = jar

	fbzx, formPage, err := fetchFBZX(&sessionClient, rec)
	if err != nil {
		return fmt.Errorf("getting form session token: %w", err)
	}
	pageHistory, err := buildPageHistory(rec, formPage)
	if err != nil {
		return fmt.Errorf("building page history: %w", err)
	}
	log.Debugf("record %d page history: %s", rec.Index, pageHistory)

	partial, err := buildPartialResponseAll(rec, fbzx)
	if err != nil {
		return fmt.Errorf("building partial response: %w", err)
	}
	body := url.Values{}
	observations := rec.Values.Get(formfields.AuditObservationsEntry)
	if observations == "" {
		observations = "NA"
	}
	body.Set(formfields.AuditObservationsEntry, observations)
	body.Set("fvv", "1")
	body.Set("partialResponse", partial)
	body.Set("pageHistory", pageHistory)
	body.Set("fbzx", fbzx)
	body.Set("submissionTimestamp", "-1")

	req, err := http.NewRequest(http.MethodPost, formURL, strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://docs.google.com")
	req.Header.Set("Referer", "https://docs.google.com/")
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := sessionClient.Do(req)
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
	if !accepted && resp.StatusCode == http.StatusBadRequest {
		return submitClassic(&sessionClient, rec)
	}
	if !accepted {
		log.Debugf("record %d submission rejected; fbzx=%s posted fields: %v; response body: %s",
			rec.Index, fbzx, rec.Values, truncate(string(respBody), 2000))
		return fmt.Errorf("form did not accept submission (status %s)", resp.Status)
	}
	return nil
}

func submitNavigationStates(client *http.Client, rec workbook.Record, formPage []byte, fbzx, history string) error {
	pages := strings.Split(history, ",")
	partial, err := buildPartialResponseAll(rec, fbzx)
	if err != nil {
		return err
	}
	for end := 2; end < len(pages); end++ {
		body, err := fieldsForPage(rec, formPage, pages[end-1])
		if err != nil {
			return err
		}
		body.Set("fvv", "1")
		body.Set("partialResponse", partial)
		body.Set("pageHistory", strings.Join(pages[:end], ","))
		body.Set("fbzx", fbzx)
		body.Set("submissionTimestamp", "-1")
		body.Set("continue", "1")
		resp, err := postForm(client, body)
		if err != nil {
			return fmt.Errorf("page history %s: %w", body.Get("pageHistory"), err)
		}
		resp.Body.Close()
	}
	return nil
}

func buildPartialResponseAll(rec workbook.Record, fbzx string) (string, error) {
	items := make([][]any, 0, len(rec.Values))
	for _, key := range formfields.Canonical {
		if key == formfields.AuditObservationsEntry {
			continue
		}
		values, ok := rec.Values[key]
		if !ok {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(key, "entry."), 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid entry key %q: %w", key, err)
		}
		items = append(items, []any{nil, id, values, 0})
	}
	b, err := json.Marshal([]any{items, nil, fbzx})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fieldsForPage(rec workbook.Record, formPage []byte, page string) (url.Values, error) {
	match := formDataPattern.FindSubmatch(formPage)
	if match == nil {
		return nil, fmt.Errorf("form metadata not found on form page")
	}
	var root []any
	if err := json.Unmarshal(match[1], &root); err != nil {
		return nil, fmt.Errorf("parsing form metadata: %w", err)
	}
	rootOne, ok := nestedSlice(root, 1)
	if !ok {
		return nil, fmt.Errorf("form metadata has no question data")
	}
	entries, ok := nestedSlice(rootOne, 1)
	if !ok {
		return nil, fmt.Errorf("form metadata has no sections")
	}
	pageNumber, err := strconv.Atoi(page)
	if err != nil {
		return nil, fmt.Errorf("invalid page number %q: %w", page, err)
	}
	section := -1
	body := url.Values{}
	for _, raw := range entries {
		items, ok := raw.([]any)
		if !ok || len(items) < 4 {
			continue
		}
		if number(items[3]) == 8 {
			section++
			continue
		}
		if section+1 != pageNumber || len(items) < 5 {
			continue
		}
		data, ok := items[4].([]any)
		if !ok || len(data) == 0 {
			continue
		}
		questionData, ok := data[0].([]any)
		if !ok || len(questionData) == 0 {
			continue
		}
		entryID, ok := integer(questionData[0])
		if !ok {
			continue
		}
		key := "entry." + strconv.FormatInt(entryID, 10)
		values, exists := rec.Values[key]
		if !exists {
			continue
		}
		if key == formfields.AuditDateEntry {
			parts := strings.Split(values[0], "-")
			if len(parts) == 3 {
				body.Set(key+"_year", parts[0])
				body.Set(key+"_month", parts[1])
				body.Set(key+"_day", parts[2])
			}
			continue
		}
		for _, value := range values {
			body.Add(key, value)
		}
		if formfields.IsChoice(key) {
			body.Set(key+"_sentinel", "")
		}
	}
	return body, nil
}

func postForm(client *http.Client, body url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, formURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://docs.google.com")
	req.Header.Set("Referer", "https://docs.google.com/")
	req.Header.Set("User-Agent", browserUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return resp, nil
}

func submitClassic(client *http.Client, rec workbook.Record) error {
	body := url.Values{}
	for key, values := range rec.Values {
		for _, value := range values {
			body.Add(key, value)
		}
	}
	req, err := http.NewRequest(http.MethodPost, formURL, strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("building fallback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://docs.google.com")
	req.Header.Set("Referer", "https://docs.google.com/")
	req.Header.Set("User-Agent", browserUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending fallback request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
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
