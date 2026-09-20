// Package dramaquery turns the raw drama records returned by the weixin
// client into the bounded, domain-labelled answers that both the weappctl CLI
// and the read-only MCP server hand out.
//
// The point of this package is that the meaning of WeChat's status fields is
// encoded once, in one place: a caller asking for "in-review" cannot
// accidentally read weixin.DramaInfo.Status, which is NOT the review state
// (see the DramaInfo.Status comment for the measured discrepancy).
package dramaquery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xsh/weappctl/internal/weixin"
)

// PageSize is the page size used when paging through listDramas; WeChat caps
// limit at 100.
const PageSize = 100

// Derived output keys that do not come from the API response verbatim.
const (
	// FieldAuditStatus is the authoritative review state as a domain word.
	FieldAuditStatus = "audit_status"
	// FieldTakenDown reports whether the platform has taken the drama down.
	FieldTakenDown = "taken_down"
)

// Audit status words, mirroring weixin.AuditDetail.Status.
const (
	AuditInvalid  = "invalid"   // 0: not a valid audit record
	AuditInReview = "in-review" // 1: in review (authoritative review signal)
	AuditRejected = "rejected"  // 2: finally rejected
	AuditApproved = "approved"  // 3: approved
	AuditReturned = "returned"  // 4: returned for revision
	AuditUnknown  = "unknown"   // anything WeChat adds later
)

// State is one filterable domain state. Every state maps to exactly one
// predicate: the five audit states read the authoritative audit field, and
// taken-down reads the coarse playability field. A drama can match both an
// audit state and taken-down (23 of them do in the account this was built
// against), which is why the two axes stay separate.
type State string

// The filterable states.
const (
	StateInvalid   State = AuditInvalid
	StateInReview  State = AuditInReview
	StateRejected  State = AuditRejected
	StateApproved  State = AuditApproved
	StateReturned  State = AuditReturned
	StateTakenDown State = "taken-down"
)

// States lists every accepted --state / state value, in audit order.
var States = []State{
	StateInvalid,
	StateInReview,
	StateRejected,
	StateApproved,
	StateReturned,
	StateTakenDown,
}

// rawFields are the top-level keys of weixin.DramaInfo, i.e. everything the API
// returns verbatim. They stay selectable so a caller can always reach ground
// truth even when the derived words are what we steer them to.
var rawFields = []string{
	"drama_id",
	"create_time",
	"name",
	"cover_url",
	"media_count",
	"producer",
	"playwright",
	"description",
	"production_license",
	"audit_detail",
	"media_list",
	"expedited",
	"recommendations",
	"promotion_poster",
	"actor_list",
	"status",
}

// DefaultFields is the projection used when the caller does not ask for
// specific fields: small enough that a full list stays readable, and labelled
// so the review state cannot be misread.
var DefaultFields = []string{"drama_id", "name", FieldAuditStatus, FieldTakenDown}

var validFields = func() map[string]bool {
	m := make(map[string]bool, len(rawFields)+2)
	for _, f := range rawFields {
		m[f] = true
	}
	m[FieldAuditStatus] = true
	m[FieldTakenDown] = true
	return m
}()

// FieldNames returns every accepted output field name, sorted.
func FieldNames() []string {
	names := make([]string, 0, len(validFields))
	for f := range validFields {
		names = append(names, f)
	}
	sort.Strings(names)
	return names
}

// AuditStatus returns the authoritative review state of d as a domain word.
func AuditStatus(d weixin.DramaInfo) string {
	switch d.AuditDetail.Status {
	case 0:
		return AuditInvalid
	case 1:
		return AuditInReview
	case 2:
		return AuditRejected
	case 3:
		return AuditApproved
	case 4:
		return AuditReturned
	default:
		return AuditUnknown
	}
}

// TakenDown reports whether the platform has taken d down.
func TakenDown(d weixin.DramaInfo) bool { return d.Status == 3 }

func (s State) matches(d weixin.DramaInfo) bool {
	if s == StateTakenDown {
		return TakenDown(d)
	}
	return AuditStatus(d) == string(s)
}

// ParseStates validates the state values given on the command line or over
// MCP. An empty list means "no filter".
func ParseStates(values []string) ([]State, error) {
	if len(values) == 0 {
		return nil, nil
	}
	states := make([]State, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		known := false
		for _, s := range States {
			if string(s) == v {
				states = append(states, s)
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("未知的状态 %q；可选值：%s", v, stateList())
		}
	}
	return states, nil
}

func stateList() string {
	names := make([]string, 0, len(States))
	for _, s := range States {
		names = append(names, string(s))
	}
	return strings.Join(names, ", ")
}

// ParseFields turns a comma-separated field list into a validated projection.
// An empty value selects DefaultFields.
func ParseFields(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return append([]string(nil), DefaultFields...), nil
	}

	seen := make(map[string]bool)
	fields := make([]string, 0, len(DefaultFields))
	for _, f := range strings.Split(value, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !validFields[f] {
			return nil, fmt.Errorf("未知的输出字段 %q；可选字段：%s", f, strings.Join(FieldNames(), ", "))
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("输出字段不能为空；可选字段：%s", strings.Join(FieldNames(), ", "))
	}
	return fields, nil
}

// ValidateMaxItems rejects a negative cap. Zero means unlimited.
func ValidateMaxItems(maxItems int) error {
	if maxItems < 0 {
		return fmt.Errorf("max_items 不能为负数，当前为 %d（0 表示不限）", maxItems)
	}
	return nil
}

// Filter returns the dramas matching any of states; no states means all.
func Filter(dramas []weixin.DramaInfo, states []State) []weixin.DramaInfo {
	if len(states) == 0 {
		return dramas
	}
	out := make([]weixin.DramaInfo, 0, len(dramas))
	for _, d := range dramas {
		for _, s := range states {
			if s.matches(d) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// Counts summarises a full drama list. Both axes are reported independently:
// audit_status is the authoritative review state, taken_down is the platform
// takedown flag, and a drama may be counted in both.
type Counts struct {
	AuditStatus map[string]int `json:"audit_status"`
	TakenDown   int            `json:"taken_down"`
}

// Summarize counts dramas by audit state and by takedown. Every known audit
// state is present even when its count is zero, so callers see a stable shape.
func Summarize(dramas []weixin.DramaInfo) Counts {
	c := Counts{
		AuditStatus: map[string]int{
			AuditInvalid:  0,
			AuditInReview: 0,
			AuditRejected: 0,
			AuditApproved: 0,
			AuditReturned: 0,
		},
	}
	for _, d := range dramas {
		c.AuditStatus[AuditStatus(d)]++
		if TakenDown(d) {
			c.TakenDown++
		}
	}
	return c
}

// Envelope is the answer shape for list queries: the full-population counts,
// how many records matched, how many were actually returned, and the records
// themselves. Counts cover the whole fetched list, not just the filtered
// subset, so one call answers "how many are approved" without pulling 614
// records back.
type Envelope struct {
	Matched   int              `json:"matched"`
	Returned  int              `json:"returned"`
	Truncated bool             `json:"truncated"`
	Counts    Counts           `json:"counts"`
	Dramas    []map[string]any `json:"dramas"`
}

// Project returns the output record for d, keeping only fields. Raw fields are
// copied verbatim from the API response; audit_status and taken_down are the
// derived domain words.
func Project(d weixin.DramaInfo, fields []string) (map[string]any, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("encode drama %d: %w", d.DramaID, err)
	}
	var all map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("decode drama %d: %w", d.DramaID, err)
	}

	out := make(map[string]any, len(fields))
	for _, f := range fields {
		switch f {
		case FieldAuditStatus:
			out[f] = AuditStatus(d)
		case FieldTakenDown:
			out[f] = TakenDown(d)
		default:
			out[f] = all[f]
		}
	}
	return out, nil
}

// ProjectWithDerived returns every raw field of d plus the two derived keys.
// Single-drama lookups are small enough to hand over verbatim, and the derived
// keys are what stop a caller misreading the raw status field.
func ProjectWithDerived(d weixin.DramaInfo) (map[string]any, error) {
	fields := make([]string, 0, len(rawFields)+2)
	fields = append(fields, rawFields...)
	fields = append(fields, FieldAuditStatus, FieldTakenDown)
	return Project(d, fields)
}

// Build filters dramas, caps the result, and projects it into an Envelope.
// maxItems is the cap on returned records; zero means unlimited.
func Build(dramas []weixin.DramaInfo, states []State, fields []string, maxItems int) (Envelope, error) {
	if err := ValidateMaxItems(maxItems); err != nil {
		return Envelope{}, err
	}

	filtered := Filter(dramas, states)
	returned := filtered
	if maxItems > 0 && len(filtered) > maxItems {
		returned = filtered[:maxItems]
	}

	items := make([]map[string]any, 0, len(returned))
	for _, d := range returned {
		item, err := Project(d, fields)
		if err != nil {
			return Envelope{}, err
		}
		items = append(items, item)
	}

	return Envelope{
		Matched:   len(filtered),
		Returned:  len(items),
		Truncated: len(filtered) > len(items),
		Counts:    Summarize(dramas),
		Dramas:    items,
	}, nil
}

// PublishedEnvelope is the answer shape for the published-drama list. Only the
// count and the records are meaningful here: this endpoint reports which dramas
// are currently published, which is a different question from whether they
// passed review.
type PublishedEnvelope struct {
	Matched   int                     `json:"matched"`
	Returned  int                     `json:"returned"`
	Truncated bool                    `json:"truncated"`
	Dramas    []weixin.PublishedDrama `json:"dramas"`
}

// BuildPublished caps the published-drama list. maxItems is the cap on
// returned records; zero means unlimited.
func BuildPublished(pubs []weixin.PublishedDrama, maxItems int) (PublishedEnvelope, error) {
	if err := ValidateMaxItems(maxItems); err != nil {
		return PublishedEnvelope{}, err
	}

	returned := pubs
	if maxItems > 0 && len(pubs) > maxItems {
		returned = pubs[:maxItems]
	}
	if returned == nil {
		returned = []weixin.PublishedDrama{}
	}

	return PublishedEnvelope{
		Matched:   len(pubs),
		Returned:  len(returned),
		Truncated: len(pubs) > len(returned),
		Dramas:    returned,
	}, nil
}

// FetchAllDramas returns every drama submitted under the account, paging
// through listDramas until a short page arrives.
func FetchAllDramas(ctx context.Context, c *weixin.Client, token string) ([]weixin.DramaInfo, error) {
	var all []weixin.DramaInfo
	for offset := 0; ; offset += PageSize {
		page, err := c.ListDramas(ctx, token, offset, PageSize)
		if err != nil {
			return nil, fmt.Errorf("list dramas: %w", err)
		}
		all = append(all, page...)
		if len(page) < PageSize {
			return all, nil
		}
	}
}
