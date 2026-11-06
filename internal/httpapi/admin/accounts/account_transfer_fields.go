package accounts

import (
	"reflect"
	"strings"

	"ds2api/internal/config"
)

// Derive the account backup schema from the persistent record so new account
// fields are exported and accepted together, without a separate field whitelist.
var accountTransferFields = func() map[string]bool {
	fields := make(map[string]bool)
	typ := reflect.TypeFor[config.Account]()
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}()
