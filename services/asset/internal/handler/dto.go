package handler

type folderReq struct {
	Name string `json:"name" binding:"required,min=1,max=200"`
}

type noteReq struct {
	Title string `json:"title" binding:"required,min=1,max=200"`
	Body  string `json:"body"  binding:"max=65536"`
}

type noteUpdateReq struct {
	Title string `json:"title" binding:"omitempty,max=200"`
	Body  string `json:"body"  binding:"max=65536"`
}

type shareReq struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Access string `json:"access"  binding:"required,oneof=read write"`
}
