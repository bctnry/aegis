//go:build ignore

package templates

type AdminRedirectWithMessageModel struct {
	Config *aegis.AegisConfig
	LoginInfo *LoginInfoModel
	SideBarName string
	ErrorMsg string
	Timeout int
	RedirectUrl string
	MessageTitle string
	MessageText string
}

