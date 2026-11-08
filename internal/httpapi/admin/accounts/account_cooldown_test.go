package accounts

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAccountListSeparatesCooldownFromEnableSwitch(t *testing.T) {
	h := newAdminTestHandler(t, fmt.Sprintf(`{"accounts":[{"email":"cool@example.com","token":"token","cooldown_until":%d},{"email":"ready@example.com","token":"token"}]}`, time.Now().Add(time.Minute).Unix()))
	for _, query := range []string{"", "?status=cooling_down"} {
		rec := httptest.NewRecorder()
		h.listAccounts(rec, httptest.NewRequest("GET", "/admin/accounts"+query, nil))
		var body struct {
			Items []struct {
				Identifier  string
				Enabled     bool
				Cooling     bool `json:"cooling_down"`
				Schedulable bool
			}
			Counts map[string]int
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Counts["cooling_down"] != 1 || body.Counts["enabled"] != 2 {
			t.Fatalf("mixed manual and runtime status: %+v", body)
		}
		if query == "" && body.Items[0].Identifier != "ready@example.com" {
			t.Fatal("cooling account sorted ahead of ready account")
		}
		for _, acc := range body.Items {
			if acc.Identifier == "cool@example.com" && (!acc.Enabled || !acc.Cooling || acc.Schedulable) {
				t.Fatalf("cooldown presented as ready or manually disabled: %+v", acc)
			}
		}
		if query != "" && len(body.Items) != 1 {
			t.Fatal("cooldown filter ignored")
		}
	}
}
