package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

const mixedListConfig = `{"runtime":{"device_id_mode":"auto"},"accounts":[
 {"email":"old@example.com","password":"p","remark":"TEAM","device_id_type":"auto"},
 {"email":"disabled@example.com","password":"p","disabled":true,"remark":"team","device_id_type":"auto"},
 {"email":"banned@example.com","password":"p","banned":true,"device_id_type":"auto"},
 {"email":"muted@example.com","password":"p","muted_until":4102444800,"device_id_type":"auto"},
 {"email":"cool@example.com","password":"p","cooldown_until":4102444800,"device_id_type":"auto"},
 {"email":"auth@example.com","password":"p","auth_failed":true,"device_id_type":"auto"},
 {"email":"new@example.com","password":"p","name":"Team","device_id_type":"auto"},
 {"email":"manual@example.com","password":"p","disabled":true,"device_id_type":"manual"}
]}`

func TestAccountListOrderingAcrossPagesAndFilters(t *testing.T) {
	h := newAdminTestHandler(t, mixedListConfig)
	for _, tc := range []struct {
		query string
		want  []string
		total int
	}{
		{"?page_size=3", []string{"new", "old", "manual"}, 8},
		{"?page_size=3&page=2", []string{"auth", "cool", "muted"}, 8},
		{"?page_size=3&page=3", []string{"banned", "disabled"}, 8},
		{"?q=TeAm", []string{"new", "old", "disabled"}, 3},
		{"?q=team&status=disabled", []string{"disabled"}, 1},
		{"?status=cooling_down", []string{"cool"}, 1},
		{"?page=999", []string{}, 8},
	} {
		t.Run(tc.query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.listAccounts(rec, httptest.NewRequest(http.MethodGet, "/admin/accounts"+tc.query, nil))
			got, totals := listResult(t, rec)
			if !reflect.DeepEqual(got, tc.want) || totals["total"] != tc.total {
				t.Fatalf("got %v total %d; want %v total %d", got, totals["total"], tc.want, tc.total)
			}
			if totals["configured"] != 8 {
				t.Fatalf("filter changed global counts: %v", totals)
			}
		})
	}
}

func TestDeviceListKeepsItsOwnOrderingAndUnfilteredTotal(t *testing.T) {
	h := newAdminTestHandler(t, mixedListConfig)
	for _, tc := range []struct {
		query string
		want  []string
		total int
	}{
		{"?page_size=3", []string{"old", "banned", "cool"}, 7},
		{"?page_size=3&page=2", []string{"auth", "new", "disabled"}, 7},
		{"?page_size=3&page=3", []string{"muted"}, 7},
		{"?q=DISABLED", []string{"disabled"}, 1},
		{"?q=manual", []string{}, 0},
		{"?page=999", []string{}, 7},
	} {
		t.Run(tc.query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.listRealDeviceIDs(rec, httptest.NewRequest(http.MethodGet, "/admin/device-ids/real"+tc.query, nil))
			got, totals := listResult(t, rec)
			if !reflect.DeepEqual(got, tc.want) || totals["total"] != tc.total || totals["total_all"] != 7 {
				t.Fatalf("got %v totals %v; want %v total %d (7 before filtering)", got, totals, tc.want, tc.total)
			}
		})
	}
}

func listResult(t *testing.T, rec *httptest.ResponseRecorder) ([]string, map[string]int) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items    []struct{ Identifier string }
		Total    int
		TotalAll int `json:"total_all"`
		Counts   map[string]int
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(body.Items))
	for _, item := range body.Items {
		names = append(names, item.Identifier[:len(item.Identifier)-len("@example.com")])
	}
	return names, map[string]int{"total": body.Total, "total_all": body.TotalAll, "configured": body.Counts["total"]}
}
