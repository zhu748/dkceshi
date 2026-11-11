package accounts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/config"
)

func BenchmarkAccountLists(b *testing.B) {
	for _, n := range []int{500, 5000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			cfg := config.Config{Accounts: make([]config.Account, n)}
			cfg.Runtime.DeviceIDMode = config.DeviceIDTypeAuto
			for i := range cfg.Accounts {
				cfg.Accounts[i] = config.Account{
					Email: fmt.Sprintf("account-%d@example.com", i), Password: "password",
					Disabled: i%3 == 0, DeviceIDType: config.DeviceIDTypeAuto,
				}
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				b.Fatal(err)
			}
			b.Setenv("DS2API_CONFIG_JSON", string(raw))
			h := &Handler{Store: config.LoadStore()}
			for _, device := range []bool{false, true} {
				b.Run(fmt.Sprint(device), func(b *testing.B) {
					handler, path := h.listAccounts, "/admin/accounts?page=2&page_size=10"
					if device {
						handler, path = h.listRealDeviceIDs, "/admin/device-ids/real?page=2&page_size=10"
					}
					request := httptest.NewRequest(http.MethodGet, path, nil)
					b.ReportAllocs()
					for b.Loop() {
						response := httptest.NewRecorder()
						handler(response, request)
						if response.Code != http.StatusOK {
							b.Fatal(response.Code)
						}
					}
				})
			}
		})
	}
}
