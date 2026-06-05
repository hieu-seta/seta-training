package handler

type createTeamReq struct {
	Name string `json:"name" binding:"required,min=1,max=64"`
}

type addMemberReq struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}

type addManagerReq = addMemberReq
