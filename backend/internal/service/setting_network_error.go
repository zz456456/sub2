package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const SettingKeyNetworkErrorCooldownSettings = "network_error_cooldown_settings"

type NetworkErrorCooldownSettings struct {
	Enabled bool `json:"enabled"`
}

func (s *SettingService) GetNetworkErrorCooldownSettings(ctx context.Context) (*NetworkErrorCooldownSettings, error) {
	settings := &NetworkErrorCooldownSettings{Enabled: true}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyNetworkErrorCooldownSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return settings, nil
		}
		return nil, fmt.Errorf("get network error cooldown settings: %w", err)
	}
	if value != "" {
		if err := json.Unmarshal([]byte(value), settings); err != nil {
			return &NetworkErrorCooldownSettings{Enabled: true}, nil
		}
	}
	return settings, nil
}

func (s *SettingService) SetNetworkErrorCooldownSettings(ctx context.Context, settings *NetworkErrorCooldownSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, SettingKeyNetworkErrorCooldownSettings, string(data))
}
