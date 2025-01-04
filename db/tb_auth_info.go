package db

type AuthInfo struct {
	AccessToken  string `db:"access_token"`
	RefreshToken string `db:"refresh_token"`
	ExpiresIn    int64  `db:"expires_in"`
	UserID       string `db:"user_id"`
}

// 保存认证信息到数据库
func SaveAuthInfo(a *AuthInfo) error {

	query := `INSERT INTO auth_info (access_token, refresh_token, expires_in, user_id) 
			  VALUES ($1, $2, $3, $4)
			  ON CONFLICT (user_id) DO UPDATE 
			  SET access_token = $1, refresh_token = $2, expires_in = $3`

	_, err := db.Exec(query, a.AccessToken, a.RefreshToken, a.ExpiresIn, a.UserID)
	return err
}

// 从数据库加载认证信息
func LoadAuthInfo() (*AuthInfo, error) {
	a := &AuthInfo{}

	query := `SELECT user_id, access_token, refresh_token, expires_in 
			  FROM auth_info LIMIT 1`

	err := db.QueryRow(query).Scan(&a.UserID, &a.AccessToken, &a.RefreshToken, &a.ExpiresIn)
	if err != nil {
		return nil, err
	}
	return a, nil
}
