//go:build ignore

package templates

type LoginInfoModel struct {
	LoggedIn bool
	UserName string
	IsOwner bool
	IsSettingMember bool
	IsAdmin bool
	IsSuperAdmin bool
}

