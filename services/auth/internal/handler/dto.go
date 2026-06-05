package handler

type registerReq struct {
	Username string `json:"username" binding:"required,min=1,max=64"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=128"`
	Role     string `json:"role"     binding:"required,oneof=manager member"`
}

type loginReq struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=1"`
}

type refreshReq struct {
	Refresh string `json:"refresh" binding:"required,min=10"`
}

type logoutReq = refreshReq

type tokenResp struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}

type existsResp struct {
	Exists bool   `json:"exists"`
	ID     string `json:"id"`
}
