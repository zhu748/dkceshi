package client

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/devid"
)

// isRiskDeviceDetected reports whether a login failure carries the upstream
// risk-control verdict for the submitted device id.
func isRiskDeviceDetected(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "RISK_DEVICE_DETECTED")
}

// shouldRegenerateLoginDeviceID reports whether a login rejection should
// trigger regenerating the SDK device id. Only ids this deployment actually
// generated (device_id_type=auto under the auto mode) can be rotated: manual
// pool ids have no archive to drop, and a stored id this deployment never
// generated has nothing to rebuild either.
func (c *Client) shouldRegenerateLoginDeviceID(acc config.Account, err error) bool {
	if !isRiskDeviceDetected(err) {
		return false
	}
	if c.Store == nil {
		return false
	}
	if c.Store.RuntimeDeviceIDMode() != config.DeviceIDTypeAuto {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(acc.DeviceIDType), config.DeviceIDTypeAuto)
}

// Keep the SDK-generated login ID independent from the Android installation ID.
// An old untyped 44-character local profile is not an SDK result.
func (c *Client) ensureRiskDeviceID(ctx context.Context, acc config.Account) (config.Account, error) {
	legacy := false
	if raw, err := base64.StdEncoding.DecodeString(acc.DeviceID); err == nil {
		legacy = len(raw) == 32 && acc.DeviceIDType == ""
	}
	modeMatches := c.Store == nil || acc.DeviceIDType == "" || acc.DeviceIDType == c.Store.RuntimeDeviceIDMode()
	if loginDeviceID(acc) != "" && !legacy && modeMatches {
		return acc, nil
	}
	if c.Store == nil {
		return acc, errors.New("missing risk device_id: configure a generated SDK ID")
	}
	if c.Store.RuntimeDeviceIDMode() == config.DeviceIDTypeAuto {
		unlock, err := devid.AcquireAccount(ctx, "login:"+acc.Identifier())
		if err != nil {
			return acc, err
		}
		defer unlock()
		if stored, ok := c.Store.FindAccount(acc.Identifier()); ok && stored.DeviceID != "" && stored.DeviceIDType == config.DeviceIDTypeAuto {
			acc.DeviceID, acc.DeviceIDType = stored.DeviceID, stored.DeviceIDType
			return acc, nil
		}
		id, err := c.RefreshRealDeviceID(ctx, acc.Identifier(), false)
		if err != nil {
			return acc, err
		}
		acc.DeviceID, acc.DeviceIDType = id, config.DeviceIDTypeAuto
		return acc, nil
	}
	err := c.Store.Update(func(cfg *config.Config) error {
		id, ok := cfg.LeastBoundDeviceID()
		if !ok {
			return config.ErrDeviceIDPoolEmpty
		}
		cfg.BindDeviceIDToAccount(acc.Identifier(), id)
		acc.DeviceID, acc.DeviceIDType = strings.TrimSpace(id), config.DeviceIDTypeManual
		return nil
	})
	return acc, err
}
