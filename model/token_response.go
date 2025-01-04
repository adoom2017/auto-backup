package model

import (
	"encoding/json"
)

type TokenResponse struct {
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserID       string `json:"user_id,omitempty"`
}

func (t *TokenResponse) UnmarshalJSON(data []byte) error {
	// 创建一个临时结构体来避免递归
	type Alias TokenResponse
	aux := (*Alias)(t)

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	return nil
}
