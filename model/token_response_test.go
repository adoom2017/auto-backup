package model

import (
	"testing"

	"auto-backup/log"
)

func TestTokenResponse_UnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"token_type": "Bearer",
		"expires_in": 3600,
		"scope": "files.readwrite offline_access", 
		"access_token": "test_access_token",
		"refresh_token": "test_refresh_token",
		"user_id": "test_user_id"
	}`

	var tr TokenResponse
	err := tr.UnmarshalJSON([]byte(jsonStr))
	if err != nil {
		t.Errorf("TokenResponse.UnmarshalJSON() error = %v", err)
	}

	log.Info("TokenType: %s", tr.TokenType)
	log.Info("ExpiresIn: %d", tr.ExpiresIn)
	log.Info("Scope: %s", tr.Scope)
	log.Info("AccessToken: %s", tr.AccessToken)
	log.Info("RefreshToken: %s", tr.RefreshToken)
	log.Info("UserID: %s", tr.UserID)
}
