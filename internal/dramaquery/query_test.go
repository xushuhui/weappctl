package dramaquery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/xsh/weappctl/internal/weixin"
)

// sample returns one drama per audit state, plus the overlap case the domain
// model warns about: two dramas that are approved and taken down at once.
func sample() []weixin.DramaInfo {
	return []weixin.DramaInfo{
		drama(1, "审核中", 1, 1),
		drama(2, "退回修改", 4, 1),
		drama(3, "终审拒绝", 2, 2),
		drama(4, "审核通过", 3, 0),
		drama(5, "通过但下架", 3, 3),
		drama(6, "通过也下架", 3, 3),
	}
}

func drama(id int64, name string, audit, status int) weixin.DramaInfo {
	return weixin.DramaInfo{
		DramaID:     id,
		Name:        name,
		Status:      status,
		AuditDetail: weixin.AuditDetail{Status: audit},
	}
}

func TestAuditStatusAndTakenDown(t *testing.T) {
	cases := []struct {
		audit int
		want  string
	}{
		{0, AuditInvalid},
		{1, AuditInReview},
		{2, AuditRejected},
		{3, AuditApproved},
		{4, AuditReturned},
		{9, AuditUnknown},
	}
	for _, tc := range cases {
		d := drama(1, "x", tc.audit, 0)
		if got := AuditStatus(d); got != tc.want {
			t.Errorf("AuditStatus(audit=%d) = %q, want %q", tc.audit, got, tc.want)
		}
	}

	if TakenDown(drama(1, "x", 3, 0)) {
		t.Error("TakenDown(status=0) = true, want false")
	}
	if !TakenDown(drama(1, "x", 3, 3)) {
		t.Error("TakenDown(status=3) = false, want true")
	}
}

func TestParseStates(t *testing.T) {
	t.Run("empty means no filter", func(t *testing.T) {
		states, err := ParseStates(nil)
		if err != nil || states != nil {
			t.Fatalf("ParseStates(nil) = %v, %v; want nil, nil", states, err)
		}
	})

	t.Run("valid values keep order", func(t *testing.T) {
		states, err := ParseStates([]string{"taken-down", "approved"})
		if err != nil {
			t.Fatalf("ParseStates() error = %v", err)
		}
		if len(states) != 2 || states[0] != StateTakenDown || states[1] != StateApproved {
			t.Fatalf("ParseStates() = %v, want [taken-down approved]", states)
		}
	})

	t.Run("unknown value errors with the valid list", func(t *testing.T) {
		_, err := ParseStates([]string{"审核中"})
		if err == nil {
			t.Fatal("ParseStates() error = nil, want error")
		}
		for _, want := range []string{"in-review", "taken-down"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})
}

func TestParseFields(t *testing.T) {
	t.Run("empty selects defaults", func(t *testing.T) {
		fields, err := ParseFields("  ")
		if err != nil {
			t.Fatalf("ParseFields() error = %v", err)
		}
		if strings.Join(fields, ",") != strings.Join(DefaultFields, ",") {
			t.Fatalf("ParseFields() = %v, want defaults %v", fields, DefaultFields)
		}
	})

	t.Run("trims, dedups, keeps raw fields selectable", func(t *testing.T) {
		fields, err := ParseFields(" drama_id , status ,drama_id")
		if err != nil {
			t.Fatalf("ParseFields() error = %v", err)
		}
		if strings.Join(fields, ",") != "drama_id,status" {
			t.Fatalf("ParseFields() = %v, want [drama_id status]", fields)
		}
	})

	t.Run("unknown field errors with the valid list", func(t *testing.T) {
		_, err := ParseFields("dramaId")
		if err == nil {
			t.Fatal("ParseFields() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "audit_status") {
			t.Errorf("error %q does not list the valid fields", err)
		}
	})
}

func TestValidateMaxItems(t *testing.T) {
	if err := ValidateMaxItems(0); err != nil {
		t.Errorf("ValidateMaxItems(0) error = %v, want nil", err)
	}
	if err := ValidateMaxItems(-1); err == nil {
		t.Error("ValidateMaxItems(-1) error = nil, want error")
	}
}

func TestSummarizeCountsBothAxes(t *testing.T) {
	counts := Summarize(sample())

	want := map[string]int{
		AuditInvalid:  0,
		AuditInReview: 1,
		AuditRejected: 1,
		AuditApproved: 3,
		AuditReturned: 1,
	}
	for state, n := range want {
		if counts.AuditStatus[state] != n {
			t.Errorf("AuditStatus[%s] = %d, want %d", state, counts.AuditStatus[state], n)
		}
	}
	if counts.TakenDown != 2 {
		t.Errorf("TakenDown = %d, want 2", counts.TakenDown)
	}
}

func TestBuildFiltersCapsAndCountsWholeList(t *testing.T) {
	states, err := ParseStates([]string{"approved"})
	if err != nil {
		t.Fatalf("ParseStates() error = %v", err)
	}
	fields, err := ParseFields("")
	if err != nil {
		t.Fatalf("ParseFields() error = %v", err)
	}

	env, err := Build(sample(), states, fields, 1)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if env.Matched != 3 || env.Returned != 1 || !env.Truncated {
		t.Errorf("envelope = matched %d, returned %d, truncated %v; want 3, 1, true",
			env.Matched, env.Returned, env.Truncated)
	}
	// Counts describe the whole fetched list, not the filtered subset.
	if env.Counts.AuditStatus[AuditReturned] != 1 || env.Counts.TakenDown != 2 {
		t.Errorf("counts = %+v, want whole-list counts", env.Counts)
	}
	item := env.Dramas[0]
	if item["drama_id"] != float64(4) || item["name"] != "审核通过" {
		t.Errorf("projected record = %+v, want drama_id 4 / name 审核通过", item)
	}
	if item[FieldAuditStatus] != AuditApproved || item[FieldTakenDown] != false {
		t.Errorf("derived keys = %v/%v, want approved/false",
			item[FieldAuditStatus], item[FieldTakenDown])
	}
	if len(item) != len(DefaultFields) {
		t.Errorf("record has %d keys, want %d", len(item), len(DefaultFields))
	}
}

func TestBuildTakenDownIsOrthogonalToApproved(t *testing.T) {
	states, err := ParseStates([]string{"taken-down"})
	if err != nil {
		t.Fatalf("ParseStates() error = %v", err)
	}
	fields, _ := ParseFields("")

	env, err := Build(sample(), states, fields, 0)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if env.Matched != 2 {
		t.Fatalf("matched = %d, want 2", env.Matched)
	}
	for _, item := range env.Dramas {
		if item[FieldAuditStatus] != AuditApproved || item[FieldTakenDown] != true {
			t.Errorf("record %+v should be approved AND taken down", item)
		}
	}
}

func TestBuildRejectsNegativeMaxItems(t *testing.T) {
	fields, _ := ParseFields("")
	if _, err := Build(sample(), nil, fields, -5); err == nil {
		t.Error("Build(maxItems=-5) error = nil, want error")
	}
}

func TestProjectWithDerivedKeepsEveryRawField(t *testing.T) {
	d := drama(7, "完整记录", 3, 3)
	d.Producer = "某制作方"
	d.MediaCount = 12

	record, err := ProjectWithDerived(d)
	if err != nil {
		t.Fatalf("ProjectWithDerived() error = %v", err)
	}
	if len(record) != len(rawFields)+2 {
		t.Errorf("record has %d keys, want %d", len(record), len(rawFields)+2)
	}
	if record["producer"] != "某制作方" || record["media_count"] != float64(12) {
		t.Errorf("raw fields lost in projection: %+v", record)
	}
	if record[FieldAuditStatus] != AuditApproved || record[FieldTakenDown] != true {
		t.Errorf("derived keys = %v/%v, want approved/true",
			record[FieldAuditStatus], record[FieldTakenDown])
	}
}

func TestBuildPublishedCapsAndReportsTruncation(t *testing.T) {
	pubs := []weixin.PublishedDrama{
		{DramaID: "1", SrcAppID: "wx1"},
		{DramaID: "2", SrcAppID: "wx2"},
		{DramaID: "3", SrcAppID: "wx3"},
	}

	env, err := BuildPublished(pubs, 2)
	if err != nil {
		t.Fatalf("BuildPublished() error = %v", err)
	}
	if env.Matched != 3 || env.Returned != 2 || !env.Truncated {
		t.Errorf("envelope = %d/%d/%v, want 3/2/true", env.Matched, env.Returned, env.Truncated)
	}

	empty, err := BuildPublished(nil, 0)
	if err != nil {
		t.Fatalf("BuildPublished(nil) error = %v", err)
	}
	if empty.Dramas == nil {
		t.Error("BuildPublished(nil).Dramas = nil, want empty slice for a stable JSON shape")
	}
	if _, err := BuildPublished(pubs, -1); err == nil {
		t.Error("BuildPublished(maxItems=-1) error = nil, want error")
	}
}

func TestFetchAllDramasPagesUntilShortPage(t *testing.T) {
	var offsets []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		offsets = append(offsets, body.Offset)

		// Two full pages, then a short one.
		n := PageSize
		if body.Offset >= 2*PageSize {
			n = 3
		}
		list := make([]weixin.DramaInfo, 0, n)
		for i := 0; i < n; i++ {
			list = append(list, drama(int64(body.Offset+i), "d", 3, 0))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"drama_info_list":%s}`, mustJSON(t, list))
	}))
	defer srv.Close()

	client := &weixin.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	all, err := FetchAllDramas(context.Background(), client, "token")
	if err != nil {
		t.Fatalf("FetchAllDramas() error = %v", err)
	}
	if len(all) != 2*PageSize+3 {
		t.Errorf("got %d dramas, want %d", len(all), 2*PageSize+3)
	}
	if strings.Join(intsToStrings(offsets), ",") != "0,100,200" {
		t.Errorf("offsets = %v, want [0 100 200]", offsets)
	}
}

func TestFetchAllDramasSurfacesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":40001,"errmsg":"invalid credential"}`)
	}))
	defer srv.Close()

	client := &weixin.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if _, err := FetchAllDramas(context.Background(), client, "token"); err == nil {
		t.Fatal("FetchAllDramas() error = nil, want the API error surfaced")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(data)
}

func intsToStrings(in []int) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, strconv.Itoa(v))
	}
	return out
}
