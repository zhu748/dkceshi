package config

import (
	"errors"
	"strings"
)

// UpdateAccountDeviceID 把新生成的真实设备指纹写回账号配置，并同步标注
// device_id_type。auto 模式下由数美 fp SDK 刷新入口调用；标识不存在时返回
// 错误，与 UpdateAccountToken 的行为保持一致。
func (s *Store) UpdateAccountDeviceID(identifier, deviceID, deviceIDType string) error {
	identifier = strings.TrimSpace(identifier)
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.findAccountIndexLocked(identifier)
	if !ok {
		return errors.New("account not found")
	}
	acc := s.cfg.Accounts[idx]
	acc.DeviceID = strings.TrimSpace(deviceID)
	if deviceIDType != "" {
		acc.DeviceIDType = NormalizeDeviceIDType(deviceIDType)
	}
	s.cfg.Accounts[idx] = acc
	// 指纹属于设备身份材料，写回后立即落盘，避免进程退出丢失。
	s.cfgDirty = true
	return s.saveLocked()
}
