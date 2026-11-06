package accounts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"ds2api/internal/config"
	"ds2api/internal/jsonvalue"
)

const accountBackupFormat = "ds2api.accounts"
const maxAccountImportBytes = 16 << 20

type accountBackup struct {
	Format     string           `json:"format"`
	Version    int              `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Accounts   []config.Account `json:"accounts"`
}

func decodeAccountBackup(reader io.Reader) ([]config.Account, error) {
	var raw json.RawMessage
	if err := jsonvalue.Decode(reader, &raw); err != nil {
		return nil, fmt.Errorf("账号文件 JSON 无效: %w", err)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("账号文件必须使用有效 UTF-8 编码")
	}
	data := bytes.TrimSpace(raw)
	if len(data) > 0 && data[0] == '{' {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, err
		}
		if value, exists := envelope["format"]; exists {
			var format string
			if json.Unmarshal(value, &format) != nil || format != accountBackupFormat {
				return nil, fmt.Errorf("不支持的账号备份格式")
			}
		}
		if value, exists := envelope["version"]; exists {
			var version int
			if json.Unmarshal(value, &version) != nil || version != 1 {
				return nil, fmt.Errorf("不支持的账号备份版本")
			}
		}
		data = envelope["accounts"]
	}
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || len(records) == 0 {
		return nil, fmt.Errorf("文件需要包含非空 accounts 数组或账号数组")
	}
	accounts := make([]config.Account, 0, len(records))
	for i, record := range records {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(record, &fields); err != nil || fields == nil {
			return nil, fmt.Errorf("第 %d 条账号必须是对象", i+1)
		}
		for name, value := range fields {
			if !accountTransferFields[name] {
				return nil, fmt.Errorf("第 %d 条账号包含不支持的字段 %q，请使用账号备份或配置文件", i+1, name)
			}
			if string(value) == "null" && name != "photo_compress_ratio" {
				return nil, fmt.Errorf("第 %d 条账号的 %s 不能为 null", i+1, name)
			}
		}
		var acc config.Account
		decoder := json.NewDecoder(bytes.NewReader(record))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&acc); err != nil {
			return nil, fmt.Errorf("第 %d 条账号字段无效: %w", i+1, err)
		}
		if err := validateImportedAccount(acc); err != nil {
			return nil, fmt.Errorf("第 %d 条账号无效: %w", i+1, err)
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}
